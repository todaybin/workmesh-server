// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

func websiteConfigWrite(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var in struct {
		WebsiteID uint            `json:"websiteID"`
		WebsiteId uint            `json:"websiteId"`
		ID        uint            `json:"id"`
		Type      string          `json:"type"`
		Operate   string          `json:"operate"`
		Scope     string          `json:"scope"`
		Params    json.RawMessage `json:"params"`
		Config    map[string]any  `json:"config"`
		Content   string          `json:"content"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.ID
	}
	if in.WebsiteID == 0 {
		in.WebsiteID = in.WebsiteId
	}
	if in.WebsiteID == 0 {
		writeError(w, 400, errors.New("网站 ID 无效"))
		return
	}
	if in.Scope == "index" {
		var paramMap map[string]any
		_ = json.Unmarshal(in.Params, &paramMap)
		params := map[string]string{}
		for k, v := range paramMap {
			switch value := v.(type) {
			case string:
				params[k] = value
			case []any:
				parts := make([]string, 0, len(value))
				for _, item := range value {
					if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
						parts = append(parts, strings.TrimSpace(text))
					}
				}
				params[k] = strings.Join(parts, " ")
			}
		}
		if in.Operate == "get" || len(params) == 0 {
			cfg, err := svc.WebsiteNginxScopeConfig(in.WebsiteID, in.Scope)
			if err != nil {
				writeError(w, 404, err)
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
			return
		}
		documentText := params["index"]
		documents := strings.Fields(documentText)
		if len(documents) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("默认文档不能为空"))
			return
		}
		if err := svc.UpdateWebsiteNginxIndex(in.WebsiteID, documents); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		cfg := map[string]any{"enable": in.Operate != "disable", "params": []map[string]any{{"name": "index", "params": documents}}}
		result, err := svc.UpdateConfig(in.WebsiteID, "index", cfg)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": result})
		return
	}
	if in.Scope == "limit-conn" {
		if in.Operate == "get" {
			cfg, err := svc.WebsiteNginxScopeConfig(in.WebsiteID, in.Scope)
			if err != nil {
				writeError(w, 404, err)
				return
			}
			wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
			return
		}
		var entries []map[string]string
		if len(in.Params) > 0 && string(in.Params) != "null" {
			if err := json.Unmarshal(in.Params, &entries); err != nil {
				writeError(w, 400, errors.New("limit-conn params 必须为数组"))
				return
			}
		}
		enabled := in.Operate != "delete" && in.Operate != "disable"
		if enabled && len(entries) == 0 {
			writeError(w, 400, errors.New("limit-conn params 必须为数组"))
			return
		}
		perserver, perip, rate := 300, 25, 512
		for _, entry := range entries {
			for name, raw := range entry {
				if name != "limit_conn" && name != "limit_rate" {
					writeError(w, 400, errors.New("不支持的限流指令"))
					return
				}
				value := strings.TrimSpace(raw)
				if name == "limit_conn" {
					parts := strings.Fields(value)
					if len(parts) != 2 {
						writeError(w, 400, errors.New("limit_conn 参数无效"))
						return
					}
					n, err := strconv.Atoi(parts[1])
					if err != nil || n < 1 || n > 65535 || (parts[0] != "perserver" && parts[0] != "perip") {
						writeError(w, 400, errors.New("limit_conn 参数无效"))
						return
					}
					if parts[0] == "perserver" {
						perserver = n
					} else {
						perip = n
					}
				} else {
					if !strings.HasSuffix(value, "k") {
						writeError(w, 400, errors.New("limit_rate 单位必须为 k"))
						return
					}
					n, err := strconv.Atoi(strings.TrimSuffix(value, "k"))
					if err != nil || n < 1 || n > 99999999 {
						writeError(w, 400, errors.New("limit_rate 参数无效"))
						return
					}
					rate = n
				}
			}
		}
		cfg, err := svc.UpdateLimitConn(in.WebsiteID, enabled, perserver, perip, rate)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
		return
	}
	if in.Type == "" {
		in.Type = "nginx"
	}
	value := in.Config
	if value == nil {
		value = map[string]any{"content": in.Content}
	}
	cfg, err := svc.UpdateConfig(in.WebsiteID, in.Type, value)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, 404, err)
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": cfg})
}
