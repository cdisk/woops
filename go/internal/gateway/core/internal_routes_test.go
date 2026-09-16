package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Internal routes are unauthenticated (control-api is the only caller), so the
// public listener must never serve them. Regression: /internal/portmap/* used to
// be registered on the shared mux and was reachable from the internet on :9200.
func TestRegisterInternalStaysOffPublicHandler(t *testing.T) {
	s := New(Config{})
	s.RegisterInternal("/internal/portmap/open", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/portmap/open", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public handler served internal route: status = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	s.InternalHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/portmap/open", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("internal handler did not serve internal route: status = %d, want 418", rec.Code)
	}
}

// nginx reverse-proxies /ws/, /i/ and /bin/ through the internal listener, so the
// internal handler must keep serving public routes too.
func TestInternalHandlerAlsoServesPublicRoutes(t *testing.T) {
	s := New(Config{})
	s.Register("/ws/shell", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	for name, h := range map[string]http.Handler{"public": s.Handler(), "internal": s.InternalHandler()} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws/shell", nil))
		if rec.Code != http.StatusTeapot {
			t.Fatalf("%s handler: status = %d, want 418", name, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	s.InternalHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("internal handler /health: status = %d, want 200", rec.Code)
	}
}
