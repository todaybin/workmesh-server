// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

type deploymentState struct {
	mu         sync.RWMutex
	Active     string    `json:"active"`
	Previous   string    `json:"previous"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updatedAt"`
	LastVerify string    `json:"lastVerify"`
}

var localDeployment = deploymentState{Status: "idle"}

func registerDeploymentAndProcessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/deployment/status", deploymentStatus)
	mux.HandleFunc("POST /api/v2/deployment/execute", deploymentExecute)
	mux.HandleFunc("POST /api/v2/deployment/rollback", deploymentRollback)
	mux.HandleFunc("POST /api/v2/deployment-manifest/verify", deploymentManifestVerify)
	mux.HandleFunc("POST /api/v2/deployment-artifact/activate", deploymentArtifactActivate)
	registerProcessRoutes(mux)
}

func isDeploymentProcessRoute(pattern string) bool {
	parts := strings.SplitN(pattern, " ", 2)
	path := pattern
	if len(parts) == 2 {
		path = parts[1]
	}
	return path == "/api/v2/deployment/status" || strings.HasPrefix(path, "/api/v2/deployment/") || strings.HasPrefix(path, "/api/v2/deployment-")
}

func deploymentStatus(w http.ResponseWriter, _ *http.Request) {
	localDeployment.mu.RLock()
	state := localDeployment
	localDeployment.mu.RUnlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": state})
}

func deploymentManifestVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		SHA256  string `json:"sha256"`
		Content string `json:"content"`
	}
	if err := decodeLimited(r, &req); err != nil {
		deploymentError(w, err)
		return
	}
	var digest string
	if req.Path != "" {
		f, err := os.Open(req.Path)
		if err != nil {
			deploymentError(w, err)
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err = io.Copy(h, f); err != nil {
			deploymentError(w, err)
			return
		}
		digest = hex.EncodeToString(h.Sum(nil))
	} else {
		sum := sha256.Sum256([]byte(req.Content))
		digest = hex.EncodeToString(sum[:])
	}
	if req.SHA256 != "" && !strings.EqualFold(req.SHA256, digest) {
		deploymentError(w, errors.New("制品 SHA256 校验失败"))
		return
	}
	localDeployment.mu.Lock()
	localDeployment.LastVerify = digest
	localDeployment.UpdatedAt = time.Now().UTC()
	localDeployment.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"sha256": digest, "verified": true}})
}

func deploymentArtifactActivate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Artifact string `json:"artifact"`
		Path     string `json:"path"`
		SHA256   string `json:"sha256"`
	}
	if err := decodeLimited(r, &req); err != nil {
		deploymentError(w, err)
		return
	}
	artifact := req.Artifact
	if artifact == "" {
		artifact = req.Path
	}
	if artifact == "" {
		deploymentError(w, errors.New("制品路径不能为空"))
		return
	}
	info, err := os.Stat(artifact)
	if err != nil || !info.Mode().IsRegular() {
		deploymentError(w, errors.New("制品不存在或不是普通文件"))
		return
	}
	verified, err := fileSHA256(artifact)
	if err != nil {
		deploymentError(w, err)
		return
	}
	if req.SHA256 != "" && !strings.EqualFold(req.SHA256, verified) {
		deploymentError(w, errors.New("制品 SHA256 校验失败"))
		return
	}
	localDeployment.mu.Lock()
	localDeployment.Previous = localDeployment.Active
	localDeployment.Active = artifact
	localDeployment.Status = "staged"
	localDeployment.LastVerify = verified
	localDeployment.UpdatedAt = time.Now().UTC()
	localDeployment.mu.Unlock()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"artifact": artifact, "sha256": verified, "status": "staged"}})
}

func deploymentExecute(w http.ResponseWriter, _ *http.Request) {
	localDeployment.mu.Lock()
	defer localDeployment.mu.Unlock()
	if localDeployment.Active == "" {
		deploymentError(w, errors.New("尚未激活部署制品"))
		return
	}
	localDeployment.Status = "active"
	localDeployment.UpdatedAt = time.Now().UTC()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": localDeployment})
}

func deploymentRollback(w http.ResponseWriter, _ *http.Request) {
	localDeployment.mu.Lock()
	defer localDeployment.mu.Unlock()
	if localDeployment.Previous == "" {
		deploymentError(w, errors.New("没有可回滚的版本"))
		return
	}
	localDeployment.Active, localDeployment.Previous = localDeployment.Previous, localDeployment.Active
	localDeployment.Status = "rolled_back"
	localDeployment.UpdatedAt = time.Now().UTC()
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": localDeployment})
}

func processInfo(w http.ResponseWriter, r *http.Request) {
	pidText := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/process/"), "/")
	if pidText == "ws" {
		wmhttp.JSON(w, http.StatusUpgradeRequired, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "WEBSOCKET_REQUIRED"}})
		return
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		deploymentError(w, errors.New("PID 无效"))
		return
	}
	_, findErr := os.FindProcess(pid)
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"pid": pid, "exists": findErr == nil}})
}

func processStop(w http.ResponseWriter, r *http.Request) {
	if token := os.Getenv("WORKMESH_PROCESS_TOKEN"); token == "" || r.Header.Get("X-WorkMesh-Token") != token {
		wmhttp.JSON(w, http.StatusUnauthorized, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "PROCESS_AUTH_REQUIRED"}})
		return
	}
	var req struct {
		PID int `json:"pid"`
	}
	if err := decodeLimited(r, &req); err != nil {
		deploymentError(w, err)
		return
	}
	if req.PID <= 1 || req.PID == os.Getpid() {
		deploymentError(w, errors.New("拒绝终止受保护进程"))
		return
	}
	process, err := os.FindProcess(req.PID)
	if err != nil {
		deploymentError(w, err)
		return
	}
	if err := process.Kill(); err != nil {
		deploymentError(w, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"pid": req.PID, "stopped": true}})
}

func processListening(w http.ResponseWriter, _ *http.Request) {
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"items": []any{}, "total": 0}})
}

func decodeLimited(r *http.Request, target any) error {
	if r.Body == nil {
		return io.EOF
	}
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(target)
}

func deploymentError(w http.ResponseWriter, err error) {
	wmhttp.JSON(w, http.StatusBadRequest, map[string]any{"code": "ERR", "details": map[string]string{"errCode": "DEPLOYMENT_INVALID"}, "message": fmt.Sprint(err)})
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
