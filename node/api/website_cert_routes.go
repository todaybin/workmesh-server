// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

// registerWebsiteCertificateRoutes 注册 ACME 账户和自签 CA 的完整生命周期接口。
// 证书私钥仅在服务端持久化，JSON 和下载接口均明确控制敏感字段的暴露范围。
// registerWebsiteCertificateRoutes 注册 ACME 账户和自签 CA 的完整生命周期接口。
// 证书私钥仅在服务端持久化，所有 JSON 和下载接口都明确控制暴露范围。
func registerWebsiteCertificateRoutes(mux *http.ServeMux, security *service.WebsiteSecurityService) {
	registerACMERoutes(mux, security)
	registerCARoutes(mux, security)
}

func registerACMERoutes(mux *http.ServeMux, security *service.WebsiteSecurityService) {
	mux.HandleFunc("POST /api/v2/websites/acme/search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
			Info     string `json:"info"`
			Keyword  string `json:"keyword"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.Keyword == "" {
			in.Keyword = in.Info
		}
		total, items := security.ListACME(in.Keyword, in.Page, in.PageSize)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": total, "items": items, "page": normalizedPage(in.Page), "pageSize": normalizedPageSize(in.PageSize)}})
	})
	mux.HandleFunc("POST /api/v2/websites/acme", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email      string `json:"email"`
			Type       string `json:"type"`
			KeyType    string `json:"keyType"`
			EabKid     string `json:"eabKid"`
			EabHmacKey string `json:"eabHmacKey"`
			UseProxy   bool   `json:"useProxy"`
			CaDirURL   string `json:"caDirURL"`
			UseEAB     bool   `json:"useEAB"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := security.CreateACME(in.Email, in.Type, in.KeyType, in.EabKid, in.EabHmacKey, in.CaDirURL, in.UseProxy, in.UseEAB)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/acme/update", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID       uint `json:"id"`
			UseProxy bool `json:"useProxy"`
		}
		if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
			if err == nil {
				err = errors.New("ACME 璐︽埛 ID 鏃犳晥")
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := security.UpdateACME(in.ID, in.UseProxy)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/acme/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID uint `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
			if err == nil {
				err = errors.New("ACME 璐︽埛 ID 鏃犳晥")
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := security.DeleteACME(in.ID); errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else if err != nil {
			writeError(w, http.StatusBadRequest, err)
		} else {
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
		}
	})
}

func registerCARoutes(mux *http.ServeMux, security *service.WebsiteSecurityService) {
	mux.HandleFunc("POST /api/v2/websites/ca/search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Page     int    `json:"page"`
			PageSize int    `json:"pageSize"`
			Name     string `json:"name"`
			Keyword  string `json:"keyword"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if in.Keyword == "" {
			in.Keyword = in.Name
		}
		total, items := security.ListCA(in.Keyword, in.Page, in.PageSize)
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"total": total, "items": items, "page": normalizedPage(in.Page), "pageSize": normalizedPageSize(in.PageSize)}})
	})
	mux.HandleFunc("POST /api/v2/websites/ca", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name             string `json:"name"`
			CommonName       string `json:"commonName"`
			Country          string `json:"country"`
			Organization     string `json:"organization"`
			OrganizationUnit string `json:"organizationUint"`
			Province         string `json:"province"`
			City             string `json:"city"`
			KeyType          string `json:"keyType"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := security.CreateCA(in.Name, in.CommonName, in.Country, in.Organization, in.OrganizationUnit, in.Province, in.City, in.KeyType)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("GET /api/v2/websites/ca/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
		if err != nil || id == 0 {
			if err == nil {
				err = errors.New("CA ID 鏃犳晥")
			}
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := security.GetCA(uint(id))
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/del", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID uint `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
			if err == nil {
				err = errors.New("CA ID 鏃犳晥")
			}
			writeError(w, 400, err)
			return
		}
		if err := security.DeleteCA(in.ID); errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
		} else if err != nil {
			writeError(w, 400, err)
		} else {
			wmhttp.JSON(w, 200, map[string]any{"code": 200})
		}
	})
	mux.HandleFunc("POST /api/v2/websites/ca/obtain", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID          uint   `json:"id"`
			Domains     string `json:"domains"`
			KeyType     string `json:"keyType"`
			Unit        string `json:"unit"`
			Time        int    `json:"time"`
			AutoRenew   bool   `json:"autoRenew"`
			Description string `json:"description"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		item, err := security.ObtainCA(in.ID, 0, in.Domains, in.KeyType, in.Unit, in.Time, in.AutoRenew, in.Description)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, 404, err)
			} else {
				writeError(w, 400, err)
			}
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/renew", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SSLID      uint `json:"SSLID"`
			SSLIDLower uint `json:"sslID"`
		}
		if err := decodeJSON(r, &in); err != nil {
			writeError(w, 400, err)
			return
		}
		if in.SSLID == 0 {
			in.SSLID = in.SSLIDLower
		}
		if in.SSLID == 0 {
			writeError(w, 400, errors.New("璇佷功 ID 鏃犳晥"))
			return
		}
		item, err := security.RenewCA(in.SSLID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeError(w, 404, err)
			} else {
				writeError(w, 400, err)
			}
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/ca/download", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID uint `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil || in.ID == 0 {
			if err == nil {
				err = errors.New("CA ID 鏃犳晥")
			}
			writeError(w, 400, err)
			return
		}
		data, name, err := security.DownloadCA(in.ID)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, 404, err)
			return
		}
		if err != nil {
			writeError(w, 500, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(strings.ReplaceAll(name, "\"", "")))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}

func normalizedPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}
func normalizedPageSize(size int) int {
	if size < 1 || size > 200 {
		return 50
	}
	return size
}
