package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSPAHandlerReturnsNotFoundForMissingBuildAsset(t *testing.T) {
	handler := NewSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/_nuxt/does-not-exist.js", nil)
	req.Header.Set("Accept", "*/*")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html") {
		t.Fatalf("expected non-html 404 body for missing build asset")
	}
}

func TestSPAHandlerFallsBackToIndexForDocumentRoute(t *testing.T) {
	handler := NewSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/CaFeZn/hangrix-main-docker/issues/1", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html") {
		t.Fatalf("expected html document fallback")
	}
}

func TestShouldServeMobileSkipsStaticAssetsForMobileUA(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/CaFeZn/MarkdownBody.D0V51beu.css", nil)
	req.Header.Set("Accept", "text/css,*/*;q=0.1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; Mobile)")

	if shouldServeMobile("CaFeZn/MarkdownBody.D0V51beu.css", req) {
		t.Fatalf("expected static asset request to skip mobile shell")
	}
}

func TestShouldServeMobileHonorsDesktopPreferenceCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/CaFeZn/hangrix-main-docker/issues", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; Mobile)")
	req.AddCookie(&http.Cookie{Name: viewPreferenceCookie, Value: "desktop"})

	if shouldServeMobile("CaFeZn/hangrix-main-docker/issues", req) {
		t.Fatalf("expected desktop preference cookie to suppress mobile shell")
	}
}

func TestShouldServeMobileSkipsWindowsDesktopUA(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")

	if shouldServeMobile("", req) {
		t.Fatalf("expected Windows desktop UA to skip mobile shell")
	}
}

func TestShouldServeMobileAllowsMobileUAForRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1")

	if !shouldServeMobile("", req) {
		t.Fatalf("expected mobile UA to serve mobile shell")
	}
}

func TestShouldServeMobileHonorsMobileClientHint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")
	req.Header.Set("Sec-CH-UA-Mobile", "?1")

	if !shouldServeMobile("", req) {
		t.Fatalf("expected mobile client hint to serve mobile shell")
	}
}

func TestSPAHandlerDesktopUADoesNotServeMobileShell(t *testing.T) {
	handler := NewSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<title>Hangrix Mobile</title>") {
		t.Fatalf("expected desktop UA to receive Nuxt shell, got mobile shell")
	}
}

func TestSPAHandlerMobileUAServesMobileShell(t *testing.T) {
	handler := NewSPAHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; Mobile)")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Hangrix Mobile</title>") {
		t.Fatalf("expected mobile UA to receive mobile shell")
	}
}
