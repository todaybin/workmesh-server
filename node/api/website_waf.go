// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var wafLogFilterKeys = []string{
	"host", "site", "websiteID", "websiteId",
	"ip", "clientIP",
	"ipRegion", "ip_region", "region", "country",
	"uri", "rule", "ruleID", "action", "status", "keyword",
}

func registerWAFRoutes(mux *http.ServeMux, svc *service.WebsiteService) {
	getLists := func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.GetLists()})
	}
	updateLists := func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFAccessLists
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := svc.UpdateLists(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
	}
	for _, path := range []string{"/api/v2/access-lists", "/api/v2/websites/waf/access-lists"} {
		mux.HandleFunc("GET "+path, getLists)
		mux.HandleFunc("POST "+path, updateLists)
	}
	mux.HandleFunc("GET /api/v2/websites/waf/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := svc.GetGlobal()
		resty := svc.GetOpenResty()
		data := svc.WAFRuntimeStatus()
		data["enabled"], data["mode"] = cfg.Enabled, cfg.Mode
		data["runtime"], data["crs"], data["standardRules"], data["version"] = "openresty", "4.14.0", cfg.StandardRules, resty.Version
		if _, ok := data["available"]; !ok {
			data["available"] = false
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/overview", func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.WAFOverview()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": data})
	})
	standard := func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.StandardRules()})
	}
	mux.HandleFunc("GET /api/v2/standard-rules", standard)
	mux.HandleFunc("GET /api/v2/websites/waf/standard-rules", standard)
	mux.HandleFunc("GET /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.ListWAFSites()})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/sites", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteID        uint   `json:"websiteID"`
			Enabled          bool   `json:"enabled"`
			Mode             string `json:"mode"`
			FrequencyEnabled *bool  `json:"frequencyEnabled"`
			DetectionLevel   *int   `json:"detectionLevel"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		item, err := svc.UpdateWAFSiteSettingsWithDetection(req.WebsiteID, req.Enabled, req.Mode, req.FrequencyEnabled, req.DetectionLevel)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	// Website-scoped REST form used by the v2 UI. Keep it backed by the same
	// file-authoritative service as the legacy /waf/sites endpoints.
	mux.HandleFunc("GET /api/v2/websites/waf/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		for _, item := range svc.ListWAFSites() {
			if item.WebsiteID == id {
				wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
				return
			}
		}
		writeError(w, http.StatusNotFound, os.ErrNotExist)
	})
	mux.HandleFunc("POST /api/v2/websites/waf/sites/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var req struct {
			Enabled          *bool  `json:"enabled"`
			Mode             string `json:"mode"`
			FrequencyEnabled *bool  `json:"frequencyEnabled"`
			DetectionLevel   *int   `json:"detectionLevel"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		current := model.WAFSite{Enabled: true, Mode: "observe"}
		for _, item := range svc.ListWAFSites() {
			if item.WebsiteID == id {
				current = item
				break
			}
		}
		if req.Enabled != nil {
			current.Enabled = *req.Enabled
		}
		if strings.TrimSpace(req.Mode) != "" {
			current.Mode = req.Mode
		}
		if req.FrequencyEnabled != nil {
			current.FrequencyEnabled = *req.FrequencyEnabled
		}
		item, err := svc.UpdateWAFSiteSettingsWithDetection(id, current.Enabled, current.Mode, req.FrequencyEnabled, req.DetectionLevel)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/sites/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		rules, err := svc.ListRules(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
	})
	// xpack/waf 兼容别名与网站 WAF 使用同一份规则存储。
	mux.HandleFunc("GET /api/v2/sites/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		rules, err := svc.ListRules(id)
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules", func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFRule
		var body struct {
			WebsiteID uint `json:"websiteID"`
			WebsiteId uint `json:"websiteId"`
			model.WAFRule
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		req = body.WAFRule
		id := body.WebsiteID
		if id == 0 {
			id = body.WebsiteId
		}
		item, err := svc.UpsertRule(id, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
	})
	mux.HandleFunc("POST /api/v2/websites/waf/rules/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteID uint   `json:"websiteID"`
			WebsiteId uint   `json:"websiteId"`
			ID        string `json:"id"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id := req.WebsiteID
		if id == 0 {
			id = req.WebsiteId
		}
		if err := svc.DeleteRule(id, req.ID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	mux.HandleFunc("POST /api/v2/rules", func(w http.ResponseWriter, r *http.Request) { wafRuleUpsert(svc, w, r) })
	mux.HandleFunc("POST /api/v2/rules/delete", func(w http.ResponseWriter, r *http.Request) { wafRuleDelete(svc, w, r) })
	mux.HandleFunc("POST /api/v2/websites/waf/global", func(w http.ResponseWriter, r *http.Request) {
		var req model.WAFGlobalConfig
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg, err := svc.UpdateGlobal(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": cfg})
	})
	mux.HandleFunc("GET /api/v2/websites/waf/global", func(w http.ResponseWriter, r *http.Request) {
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": svc.GetGlobal()})
	})
	// File-backed WAF audit queries. Both GET query parameters and the legacy
	// POST JSON form are accepted so the UI and xpack-compatible clients share
	// the same real data source.
	for _, kind := range []string{"access", "log", "attack", "intercept", "block"} {
		path := "/api/v2/websites/waf/logs/" + kind
		handler := func(w http.ResponseWriter, r *http.Request) {
			filters := map[string]string{}
			page, pageSize := 1, 20
			if r.Method == http.MethodPost && r.Body != nil {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
					for _, key := range wafLogFilterKeys {
						if value, ok := body[key]; ok {
							filters[key] = strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\""))
						}
					}
					for _, key := range []string{"startTime", "endTime"} {
						if value, ok := body[key]; ok {
							filters[key] = strings.TrimSpace(strings.Trim(fmt.Sprint(value), "\""))
						}
					}
					if value, ok := body["page"].(float64); ok {
						page = int(value)
					}
					if value, ok := body["pageSize"].(float64); ok {
						pageSize = int(value)
					}
				}
			}
			for _, key := range wafLogFilterKeys {
				if value := r.URL.Query().Get(key); value != "" {
					filters[key] = value
				}
			}
			for _, key := range []string{"startTime", "endTime"} {
				if value := r.URL.Query().Get(key); value != "" {
					filters[key] = value
				}
			}
			if value := r.URL.Query().Get("page"); value != "" {
				if n, err := strconv.Atoi(value); err == nil {
					page = n
				}
			}
			if value := r.URL.Query().Get("pageSize"); value != "" {
				if n, err := strconv.Atoi(value); err == nil {
					pageSize = n
				}
			}
			result, err := svc.QueryWAFLogs(kind, filters, page, pageSize)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
		}
		mux.HandleFunc("GET "+path, handler)
		mux.HandleFunc("POST "+path, handler)
		mux.HandleFunc("POST "+path+"/clear", func(w http.ResponseWriter, r *http.Request) {
			cleared, err := svc.ClearWAFLogs(kind)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cleared": true, "files": cleared, "kind": kind}})
		})
	}
	// Legacy xpack endpoints expose the same file-backed audit records. Keep
	// their response envelope compatible while avoiding the old in-memory log
	// snapshot used by the generic website extension handler.
	legacyLog := func(kind string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			query, err := requestMap(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			filters := map[string]string{}
			for _, key := range wafLogFilterKeys {
				if value := valueString(query, key); value != "" {
					filters[key] = value
				}
			}
			for _, key := range []string{"startTime", "endTime"} {
				if value := valueString(query, key); value != "" {
					filters[key] = value
				}
			}
			page, pageSize := intValue(query, "page"), intValue(query, "pageSize")
			if page < 1 {
				page = 1
			}
			if pageSize < 1 {
				pageSize = 20
			}
			result, err := svc.QueryWAFLogs(kind, filters, page, pageSize)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": result})
		}
	}
	mux.HandleFunc("POST /api/v2/websites/waf/log/search", legacyLog("log"))
	mux.HandleFunc("POST /api/v2/websites/waf/attack/stat", legacyLog("attack"))
	mux.HandleFunc("POST /api/v2/websites/waf/block/search", legacyLog("block"))
	mux.HandleFunc("POST /api/v2/websites/waf/relation/stat", legacyLog("attack"))
	ruleCollection := func(list func() ([]model.WAFRule, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			rules, err := list()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
		}
	}
	ruleUpdate := func(update func([]model.WAFRule) ([]model.WAFRule, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Rules []model.WAFRule `json:"rules"`
			}
			if err := decodeJSON(r, &body); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			rules, err := update(body.Rules)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": rules})
		}
	}
	mux.HandleFunc("GET /api/v2/websites/waf/global/default-rules", ruleCollection(svc.ListDefaultWAFRules))
	mux.HandleFunc("POST /api/v2/websites/waf/global/default-rules", ruleUpdate(svc.UpdateDefaultWAFRules))
	mux.HandleFunc("GET /api/v2/websites/waf/global/custom-rules", ruleCollection(svc.ListCustomWAFRules))
	mux.HandleFunc("POST /api/v2/websites/waf/global/custom-rules", ruleUpdate(svc.UpdateCustomWAFRules))
	mux.HandleFunc("POST /api/v2/websites/waf/global/apply", func(w http.ResponseWriter, r *http.Request) {
		if err := svc.ApplyDefaultWAFToWebsites(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200})
	})
	// WAF 测试只在内存中分析请求样本，不转发请求，也不执行 OpenResty 命令。
	mux.HandleFunc("POST /api/v2/websites/waf/test", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WebsiteID uint              `json:"websiteID"`
			URI       string            `json:"uri"`
			Query     string            `json:"query"`
			Body      string            `json:"body"`
			Method    string            `json:"method"`
			IP        string            `json:"ip"`
			Headers   map[string]string `json:"headers"`
			Cookies   map[string]string `json:"cookies"`
		}
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.WebsiteID != 0 {
			if _, err := svc.ListRules(req.WebsiteID); err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
		}
		parts := []string{req.URI, req.Query, req.Body, req.Method, req.IP}
		for key, value := range req.Headers {
			parts = append(parts, key, value)
		}
		for key, value := range req.Cookies {
			parts = append(parts, key, value)
		}
		sample := strings.Join(parts, "\n")
		patterns := []struct {
			id       string
			category string
			pattern  *regexp.Regexp
		}{
			{"CRS-942100", "sql_injection", regexp.MustCompile(`(?i)\bunion\s+select\b|\b(or|and)\s+['\"]?1['\"]?\s*=\s*['\"]?1`)},
			{"CRS-941100", "xss", regexp.MustCompile(`(?i)<script\b|javascript:`)},
			{"CRS-930110", "path_traversal", regexp.MustCompile(`(?:\.\./|/etc/passwd|boot\.ini)`)},
		}
		matched := make([]map[string]string, 0, len(patterns))
		for _, item := range patterns {
			if item.pattern.MatchString(sample) {
				matched = append(matched, map[string]string{"id": item.id, "category": item.category})
			}
		}
		action := "allow"
		if len(matched) > 0 && svc.GetGlobal().Mode == "block" {
			action = "block"
		}
		wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"matched": len(matched) > 0, "action": action, "rules": matched}})
	})
}
