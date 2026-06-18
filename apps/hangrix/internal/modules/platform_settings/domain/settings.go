// Package domain declares the platform_settings module's external
// contract: a key-value store for operator-configurable lifecycle
// thresholds and other platform-wide settings. Values are stored as
// strings; the service layer parses durations / ints as needed.
package domain

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// ErrSettingNotFound is returned when a requested key has no row
// in platform_settings. Callers should use errors.Is to detect it.
var ErrSettingNotFound = errors.New("platform setting not found")

// Setting is one key-value pair persisted in platform_settings.
type Setting struct {
	Key         string
	Value       string
	Description string
	UpdatedAt   time.Time
}

// Definition describes one registered key — its name, default value,
// human-readable description, and optional validation regex.
type Definition struct {
	Key         string
	Default     string
	Description string
	Type        ValueType
}

// ValueType controls admin validation and typed getters for registered keys.
type ValueType string

const (
	ValueTypeDuration ValueType = "duration"
	ValueTypeBool     ValueType = "bool"
	ValueTypeInt      ValueType = "int"
	ValueTypeString   ValueType = "string"
)

const (
	SettingFirepowerEnabled        = "firepower.enabled"
	SettingRunnerMaxTasksPerPoll   = "runner.max_tasks_per_poll"
	SettingFirepowerRunnerMaxTasks = "firepower.runner_max_tasks_per_poll"
	SettingChatGPTFastMode         = "llm.chatgpt_fast_mode"
	FirepowerPreferredModel        = "gpt-5.5"
	DefaultRunnerMaxTasksPerPoll   = 64
	DefaultFirepowerRunnerMaxTasks = 256
)

// NormalizedType returns duration for legacy definitions that omit Type.
func (d Definition) NormalizedType() ValueType {
	if d.Type == "" {
		return ValueTypeDuration
	}
	return d.Type
}

// Registry is the set of known keys with their defaults. Keys not in
// the registry are still accepted (they are stored as-is) but
// GetDuration / Get fall back to the registered default when the row
// is missing.
type Registry struct {
	defs map[string]Definition
}

// NewRegistry creates a registry seeded with the given definitions.
func NewRegistry(defs []Definition) *Registry {
	r := &Registry{defs: make(map[string]Definition, len(defs))}
	for _, d := range defs {
		r.defs[d.Key] = d
	}
	return r
}

// Lookup returns the Definition and true when key is registered.
func (r *Registry) Lookup(key string) (Definition, bool) {
	d, ok := r.defs[key]
	return d, ok
}

// Definitions returns a copy of all registered definitions.
func (r *Registry) Definitions() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

// Store is the persistence + cache abstraction consumed by the reaper
// and admin handlers. Every read goes through a short TTL cache; writes
// invalidate the cache so the next read picks up the fresh value.
type Store interface {
	// Get returns the value for key. Returns ("", false) when the
	// key has no row — callers fall back to the registered default
	// via Registry.Lookup.
	Get(ctx context.Context, key string) (string, bool, error)

	// GetDuration parses the stored value as a time.Duration.
	// Returns (registeredDefault, false) when the key is not in the
	// DB _and_ not in the registry; returns (parsed, true) on
	// success; returns an error on parse failure.
	GetDuration(ctx context.Context, key string) (time.Duration, error)

	// GetBool parses the stored value as a bool, falling back to the
	// registered default when the row is missing.
	GetBool(ctx context.Context, key string) (bool, error)

	// GetInt parses the stored value as an int, falling back to the
	// registered default when the row is missing.
	GetInt(ctx context.Context, key string) (int, error)

	// Set upserts a key-value pair. description is optional metadata
	// (only persisted on INSERT, ignored on UPDATE).
	Set(ctx context.Context, key, value, description string) error

	// List returns every known setting. Useful for admin UI.
	List(ctx context.Context) ([]Setting, error)
}

func ParseBoolValue(v string) (bool, error) {
	return strconv.ParseBool(v)
}

func ParseIntValue(v string) (int, error) {
	return strconv.Atoi(v)
}
