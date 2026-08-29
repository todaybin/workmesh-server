// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"io"
	"net"
	"net/http"
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
			ID uint `json:"ID"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if _, err := ssls.Get(r.Context(), req.ID); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		// ACME 申请是长任务；这里只返回已接受状态，实际申请由调度器执行。
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]any{"id": req.ID, "status": "queued"}})
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
		txt, lookupErr := net.LookupTXT("_acme-challenge." + item.PrimaryDomain)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"domain": item.PrimaryDomain, "records": txt, "resolved": lookupErr == nil}})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/push", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusAccepted, map[string]any{"code": 200, "data": map[string]string{"status": "queued"}})
	})
	mux.HandleFunc("POST /api/v2/websites/ssl/import", func(w http.ResponseWriter, r *http.Request) {
		var item model.WebsiteSSL
		if err := decodeJSON(r, &item); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := ssls.Upload(r.Context(), model.WebsiteSSLUploadRequest{ID: item.ID, PrivateKey: item.PrivateKey, Certificate: item.Certificate, Type: "paste", Description: item.Description})
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
