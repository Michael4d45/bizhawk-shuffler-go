package serverhost

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedAdminIndex(t *testing.T) {
	s := New()
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}
