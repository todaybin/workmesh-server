// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// RegisterSSLRoutes 注册网站 SSL 全量 API，采用与旧 Agent 一致的路径和 JSON 字段。
func RegisterSSLRoutes(mux *http.ServeMux) {
	ssls := service.NewSSLService()
	mux.HandleFunc("POST /api/v2/websites/ssl/search", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Domain string `json:"domain"`
		}
		_ = decodeJSON(r, &req)
		list := ssls.List(r.Context(), req.Domain)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": len(list), "items": list}})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": ssls.List(r.Context(), "")})
	})
	mux.HandleFunc("GET /api/v2/websites/ssl/list", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": ssls.List(r.Context(), r.URL.Query().Get("domain"))})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteSSLCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := ssls.Create(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
		provider := strings.ToLower(strings.TrimSpace(item.Provider))
		if item.AcmeAccountID != 0 && (provider == "http" || provider == "letsencrypt" || provider == "dnsaccount") {
			go func(id uint) { _ = ssls.Obtain(context.Background(), id, false) }(item.ID)
		}
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/upload", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteSSLUploadRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := ssls.Upload(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/upload/file", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		key, err := readMultipart(r, "privateKeyFile")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cert, err := readMultipart(r, "certificateFile")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id, _ := strconv.ParseUint(r.FormValue("sslID"), 10, 32)
		item, err := ssls.Upload(r.Context(), model.WebsiteSSLUploadRequest{ID: uint(id), Type: "paste", PrivateKey: string(key), Certificate: string(cert), Description: r.FormValue("description")})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/del", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []uint `json:"ids"`
		}
		if err := decodeJSON(r, &req); err != nil || len(req.IDs) == 0 {
			writeError(w, http.StatusBadRequest, errInvalidIDs)
			return
		}
		if err := ssls.Delete(r.Context(), req.IDs); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/update", func(w http.ResponseWriter, r *http.Request) {
		var req model.WebsiteSSLUpdateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := ssls.Update(r.Context(), req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/obtain", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID         uint              `json:"ID"`
			TXTRecords map[string]string `json:"TXTRecords"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if _, err := ssls.Get(r.Context(), req.ID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		item, err := ssls.Get(r.Context(), req.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if strings.EqualFold(strings.TrimSpace(item.Provider), "http") || strings.EqualFold(strings.TrimSpace(item.Provider), "letsencrypt") {
			certbot := strings.TrimSpace(os.Getenv("WORKMESH_CERTBOT_BIN"))
			if certbot == "" {
				certbot = "certbot"
			}
			if _, lookErr := exec.LookPath(certbot); lookErr != nil {
				writeError(w, http.StatusServiceUnavailable, errors.New("certbot 执行器未配置，未创建申请任务"))
				return
			}
		}
		if !strings.EqualFold(strings.TrimSpace(item.Provider), "http") && !strings.EqualFold(strings.TrimSpace(item.Provider), "dnsaccount") && !strings.EqualFold(strings.TrimSpace(item.Provider), "dnsmanual") {
			writeError(w, http.StatusServiceUnavailable, errors.New("证书提供商暂不支持本机自动签发"))
			return
		}
		// DNS 手动验证需要把用户确认的 TXT 记录交给 ACME order finalize。
		if strings.EqualFold(strings.TrimSpace(item.Provider), "dnsmanual") {
			go func() { _ = ssls.FinalizeDNSManual(context.Background(), req.ID, req.TXTRecords) }()
		} else {
			// 与 1Panel 一致：申请已进入执行状态，HTTP 请求不等待 ACME 完成。
			go func() { _ = ssls.Obtain(context.Background(), req.ID, false) }()
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"id": req.ID, "status": "applying"}})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/resolve", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteSSLID uint `json:"websiteSSLId"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := ssls.Get(r.Context(), req.WebsiteSSLID)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if strings.EqualFold(strings.TrimSpace(item.Provider), "dnsmanual") {
			account, accountErr := ssls.LoadACMEForSSL(item)
			if accountErr != nil {
				writeError(w, http.StatusBadRequest, accountErr)
				return
			}
			values, resolveErr := ssls.GetDNSManualResolve(r.Context(), item, account)
			if resolveErr != nil {
				writeError(w, http.StatusBadRequest, resolveErr)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": values})
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": []any{}})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/push", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID       uint   `json:"id"`
			SSLID    uint   `json:"sslID"`
			PushNode bool   `json:"pushNode"`
			Nodes    string `json:"nodes"`
			TaskID   string `json:"taskID"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.ID == 0 {
			req.ID = req.SSLID
		}
		if req.ID == 0 {
			writeError(w, http.StatusBadRequest, errors.New("证书 ID 无效"))
			return
		}
		item, err := ssls.Get(r.Context(), req.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		nodes := strings.TrimSpace(req.Nodes)
		if !req.PushNode || nodes == "" {
			writeError(w, http.StatusServiceUnavailable, errors.New("证书推送执行器未配置，未执行推送"))
			return
		}
		for _, node := range strings.Split(nodes, ",") {
			node = strings.TrimSpace(node)
			if node == "" || len(node) > 128 || strings.ContainsAny(node, "\\\"'\r\n") {
				writeError(w, http.StatusBadRequest, errors.New("推送节点标识无效"))
				return
			}
		}
		if !strings.EqualFold(strings.TrimSpace(item.Status), "ready") {
			writeError(w, http.StatusBadRequest, errors.New("只有 ready 状态的证书才能推送"))
			return
		}
		// 当前 WorkMesh 进程没有多节点推送执行器；不要把本地元数据更新伪装成已推送。
		writeError(w, http.StatusServiceUnavailable, errors.New("证书推送执行器未配置，未执行推送"))
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/import", func(w http.ResponseWriter, r *http.Request) {
		var item struct {
			ID          uint   `json:"id"`
			SSLID       uint   `json:"sslID"`
			PrivateKey  string `json:"privateKey"`
			Certificate string `json:"certificate"`
			PEM         string `json:"pem"`
			Description string `json:"description"`
		}
		if err := decodeJSON(r, &item); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id := item.ID
		if id == 0 {
			id = item.SSLID
		}
		cert := item.Certificate
		if cert == "" {
			cert = item.PEM
		}
		result, err := ssls.Upload(r.Context(), model.WebsiteSSLUploadRequest{ID: id, PrivateKey: item.PrivateKey, Certificate: cert, Type: "paste", Description: item.Description})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	})
	mux.HandleFunc("GET /api/v2/websites/ssl/website/{websiteId}", func(w http.ResponseWriter, r *http.Request) { getSSL(mux, ssls, w, r) })
	mux.HandleFunc("GET /api/v2/websites/ssl/{id}", func(w http.ResponseWriter, r *http.Request) { getSSL(mux, ssls, w, r) })
	mux.HandleFunc("POST /api/v2/websites/ssl/download", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID uint `json:"id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := ssls.Get(r.Context(), req.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename="+strings.ReplaceAll(item.PrimaryDomain, "/", "_")+".pem")
		_, _ = w.Write([]byte(item.Certificate))
	})
}

func getSSL(_ *http.ServeMux, ssls *service.SSLService, w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		id, err = strconv.ParseUint(r.PathValue("websiteId"), 10, 32)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	item, err := ssls.Get(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

func readMultipart(r *http.Request, name string) ([]byte, error) {
	file, _, err := r.FormFile(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, 2<<20))
}

var errInvalidIDs = errors.New("证书 ID 列表不能为空")
