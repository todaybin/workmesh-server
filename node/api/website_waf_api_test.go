package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
)

func TestWAFLogPOSTFiltersByIPRegionAliases(t *testing.T) {
	dataDir := t.TempDir()
	logDir := filepath.Join(dataDir, "waf-logs")
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKMESH_DATA_DIR", dataDir)
	t.Setenv("WORKMESH_WAF_LOG_DIR", logDir)

	logs := `{"id":"ip-region","ipRegion":"CN-Region","action":"block","status":403}` + "\n" +
		`{"id":"ip-region-underscore","ip_region":"US-Region","action":"block","status":403}` + "\n" +
		`{"id":"region","region":"EU-Region","action":"block","status":403}` + "\n" +
		`{"id":"country","country":"JP","action":"block","status":403}` + "\n"
	if err := os.WriteFile(filepath.Join(logDir, "attack.jsonl"), []byte(logs), 0o640); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	registerWAFRoutes(mux, service.NewWebsiteService(dataDir))

	tests := []struct {
		name  string
		field string
		value string
		id    string
	}{
		{name: "ipRegion", field: "ipRegion", value: "CN-Region", id: "ip-region"},
		{name: "ip_region", field: "ip_region", value: "US-Region", id: "ip-region-underscore"},
		{name: "region", field: "region", value: "EU-Region", id: "region"},
		{name: "country", field: "country", value: "JP", id: "country"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				tt.field:   tt.value,
				"page":     1,
				"pageSize": 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v2/websites/waf/logs/attack", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("POST WAF logs returned %d: %s", res.Code, res.Body.String())
			}

			var envelope struct {
				Data struct {
					Total int              `json:"total"`
					Items []map[string]any `json:"items"`
				} `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Total != 1 || len(envelope.Data.Items) != 1 {
				t.Fatalf("POST %s filter returned unexpected data: %s", tt.field, res.Body.String())
			}
			if got := envelope.Data.Items[0]["id"]; got != tt.id {
				t.Fatalf("POST %s filter returned wrong record %v: %s", tt.field, got, res.Body.String())
			}
		})
	}
}
