package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRestyModulesContractContainsModulesField(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	mux := http.NewServeMux()
	registerWebsiteFunctionalRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v2/openresty/modules", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"modules"`) || !strings.Contains(rec.Body.String(), `"dynamicSupported"`) {
		t.Fatalf("unexpected modules contract: %s", rec.Body.String())
	}
}
