package arch_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/michael4d45/bizshuffle/serverhost"
)

func TestAdminStaticIndexExists(t *testing.T) {
	index := filepath.Join("..", "..", "serverhost", "static", "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Fatal(err)
	}
}

func TestServerServesAdmin(t *testing.T) {
	s := serverhost.New()
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}
