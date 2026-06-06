package arch_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michael4d45/bizshuffle/serverhost"
)

func TestServerServesAdmin(t *testing.T) {
	s := serverhost.New()
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Skip("admin static not built; run make build-admin")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
