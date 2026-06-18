package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	platformsettings "github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
)

type stubFastModeStore struct {
	data map[string]string
}

func newStubFastModeStore() *stubFastModeStore {
	return &stubFastModeStore{data: map[string]string{}}
}

func (s *stubFastModeStore) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := s.data[key]
	return v, ok, nil
}

func (s *stubFastModeStore) GetDuration(context.Context, string) (time.Duration, error) {
	return 0, nil
}

func (s *stubFastModeStore) GetBool(ctx context.Context, key string) (bool, error) {
	v, ok, err := s.Get(ctx, key)
	if err != nil || !ok || v == "" {
		return false, err
	}
	return platformsettings.ParseBoolValue(v)
}

func (s *stubFastModeStore) GetInt(context.Context, string) (int, error) {
	return 0, nil
}

func (s *stubFastModeStore) Set(_ context.Context, key, value, _ string) error {
	s.data[key] = value
	return nil
}

func (s *stubFastModeStore) List(context.Context) ([]platformsettings.Setting, error) {
	return nil, nil
}

type noopMiddleware struct{}

func (noopMiddleware) RequireAuth(next http.Handler) http.Handler  { return next }
func (noopMiddleware) RequireAdmin(next http.Handler) http.Handler { return next }

func newFastModeTestRouter(store platformsettings.Store) http.Handler {
	h := NewHandler(&HandlerDeps{
		Settings:   store,
		Middleware: noopMiddleware{},
	})
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func TestFastModeDefaultFalse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/admin/dashboard/fast-mode", nil)
	w := httptest.NewRecorder()
	newFastModeTestRouter(newStubFastModeStore()).ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body fastModeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Enabled {
		t.Fatal("enabled = true, want false")
	}
}

func TestPatchFastModePersistsEnabled(t *testing.T) {
	store := newStubFastModeStore()
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/dashboard/fast-mode", bytes.NewReader([]byte(`{"enabled":true}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newFastModeTestRouter(store).ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if store.data[platformsettings.SettingChatGPTFastMode] != "true" {
		t.Fatalf("stored value = %q, want true", store.data[platformsettings.SettingChatGPTFastMode])
	}
}

func TestPatchFastModeRequiresEnabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/dashboard/fast-mode", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newFastModeTestRouter(newStubFastModeStore()).ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
