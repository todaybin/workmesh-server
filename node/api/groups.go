package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var (
	groupServiceMu   sync.Mutex
	groupService     *service.GroupService
	groupServicePath string
)

func currentGroupService() *service.GroupService {
	root := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if root == "" {
		root = "./data"
	}
	path := filepath.Clean(root)
	groupServiceMu.Lock()
	defer groupServiceMu.Unlock()
	if groupService == nil || groupServicePath != path {
		groupService = service.NewGroupService(root)
		groupServicePath = path
	}
	return groupService
}

func registerGroupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/groups/search", handleGroupSearch)
	mux.HandleFunc("POST /api/v2/groups", handleGroupUpsert)
	mux.HandleFunc("POST /api/v2/groups/update", handleGroupUpsert)
	mux.HandleFunc("POST /api/v2/groups/del", handleGroupDelete)
}

func handleGroupSearch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type string `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": currentGroupService().List(strings.TrimSpace(in.Type))})
}

func handleGroupUpsert(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        uint   `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	g, err := currentGroupService().Upsert(in.ID, in.Name, in.Type, in.IsDefault)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": g})
}

func handleGroupDelete(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID uint `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID == 0 {
		writeError(w, 400, strconv.ErrSyntax)
		return
	}
	if err := currentGroupService().Delete(in.ID, service.NewWebsiteService("").GroupInUse); err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
}
