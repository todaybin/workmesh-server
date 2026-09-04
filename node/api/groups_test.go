package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/todaybin/workmesh-server/node/service"
	_ "modernc.org/sqlite"
)

func TestGroupSearchReturnsArrayAndCreatePersists(t *testing.T) {
	t.Setenv("WORKMESH_DATA_DIR", t.TempDir())
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	service.SetSharedDatabase(db)
	defer service.SetSharedDatabase(nil)
	mux := http.NewServeMux()
	registerGroupRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v2/groups", bytes.NewBufferString(`{"name":"业务站点","type":"website"}`)))
	if create.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	search := httptest.NewRecorder()
	mux.ServeHTTP(search, httptest.NewRequest(http.MethodPost, "/api/v2/groups/search", bytes.NewBufferString(`{"type":"website"}`)))
	var envelope struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(search.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data) < 2 {
		t.Fatalf("expected default and created groups, got %s", search.Body.String())
	}
}
