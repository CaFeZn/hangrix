// Package service implements the platform_settings Store with a 30s
// TTL in-memory cache on top of the Postgres repo. Writes invalidate
// the cache immediately; reads fall back to the registered default
// when the key has no DB row.
package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
)

// cacheTTL is how long a cached value is considered fresh. 30s is short
// enough that a PATCH appears in the admin UI quickly, long enough that
// a reaper tick (hourly) never generates a DB round-trip per sweep.
const cacheTTL = 30 * time.Second

// Service implements domain.Store with a short-TTL cache.
type Service struct {
	repo     Repo
	registry *domain.Registry

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// Repo is the narrow persistence surface we need — just enough to
// decouple from the infra package.
type Repo interface {
	GetSetting(ctx context.Context, key string) (domain.Setting, error)
	UpsertSetting(ctx context.Context, key, value, description string) error
	ListSettings(ctx context.Context) ([]domain.Setting, error)
}

type ServiceDeps struct {
	Repo     Repo
	Registry *domain.Registry
}

func NewService(deps *ServiceDeps) *Service {
	return &Service{
		repo:     deps.Repo,
		registry: deps.Registry,
		cache:    make(map[string]cacheEntry),
	}
}

// Get satisfies domain.Store.
func (s *Service) Get(ctx context.Context, key string) (string, bool, error) {
	if v, ok := s.cacheGet(key); ok {
		return v, true, nil
	}
	setting, err := s.repo.GetSetting(ctx, key)
	if err != nil {
		if errors.Is(err, domain.ErrSettingNotFound) {
			s.cacheSet(key, "", false)
			return "", false, nil
		}
		return "", false, err
	}
	s.cacheSet(key, setting.Value, true)
	return setting.Value, true, nil
}

// GetDuration satisfies domain.Store.
func (s *Service) GetDuration(ctx context.Context, key string) (time.Duration, error) {
	v, found, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if !found || v == "" {
		if s.registry != nil {
			if def, ok := s.registry.Lookup(key); ok {
				return time.ParseDuration(def.Default)
			}
		}
		return 0, nil
	}
	return time.ParseDuration(v)
}

// GetBool satisfies domain.Store.
func (s *Service) GetBool(ctx context.Context, key string) (bool, error) {
	v, found, err := s.Get(ctx, key)
	if err != nil {
		return false, err
	}
	if !found || v == "" {
		if s.registry != nil {
			if def, ok := s.registry.Lookup(key); ok {
				return domain.ParseBoolValue(def.Default)
			}
		}
		return false, nil
	}
	return domain.ParseBoolValue(v)
}

// GetInt satisfies domain.Store.
func (s *Service) GetInt(ctx context.Context, key string) (int, error) {
	v, found, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if !found || v == "" {
		if s.registry != nil {
			if def, ok := s.registry.Lookup(key); ok {
				return domain.ParseIntValue(def.Default)
			}
		}
		return 0, nil
	}
	return domain.ParseIntValue(v)
}

// Set satisfies domain.Store. Writes through to Postgres then
// invalidates the cache entry so the next read picks up the fresh
// value.
func (s *Service) Set(ctx context.Context, key, value, description string) error {
	if err := s.repo.UpsertSetting(ctx, key, value, description); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
	return nil
}

// List satisfies domain.Store. Always hits Postgres — the admin UI
// calls this infrequently enough that a cache isn't worth the
// staleness risk.
func (s *Service) List(ctx context.Context) ([]domain.Setting, error) {
	rows, err := s.repo.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]domain.Setting, len(rows))
	for _, row := range rows {
		byKey[row.Key] = row
	}
	for _, def := range s.registry.Definitions() {
		if _, ok := byKey[def.Key]; ok {
			continue
		}
		byKey[def.Key] = domain.Setting{
			Key:         def.Key,
			Value:       def.Default,
			Description: def.Description,
		}
	}
	out := make([]domain.Setting, 0, len(byKey))
	for _, row := range byKey {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// cacheGet returns a cached value if it's still fresh.
func (s *Service) cacheGet(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.cache[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

// cacheSet stores a value in the cache. When `present` is false the
// entry is a negative cache (key not in DB).
func (s *Service) cacheSet(key, value string, present bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if present {
		s.cache[key] = cacheEntry{value: value, expiresAt: time.Now().Add(cacheTTL)}
	} else {
		// Negative cache: record absence so a tight loop doesn't
		// hammer the DB. Shorter TTL so a concurrent upsert shows
		// up quickly.
		s.cache[key] = cacheEntry{value: "", expiresAt: time.Now().Add(5 * time.Second)}
	}
}

var _ domain.Store = (*Service)(nil)
