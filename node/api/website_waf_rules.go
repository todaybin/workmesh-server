// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"errors"
	"github.com/todaybin/workmesh-server/node/model"
	"github.com/todaybin/workmesh-server/node/service"
	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func scopeParams(values map[string]string) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for name, value := range values {
		result = append(result, map[string]any{"name": name, "params": []string{value}})
	}
	sort.Slice(result, func(i, j int) bool { return result[i]["name"].(string) < result[j]["name"].(string) })
	return result
}

func wafRuleUpsert(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
	var body struct {
		WebsiteID uint `json:"websiteID"`
		WebsiteId uint `json:"websiteId"`
		model.WAFRule
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := body.WebsiteID
	if id == 0 {
		id = body.WebsiteId
	}
	item, err := svc.UpsertRule(id, body.WAFRule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

func wafRuleDelete(svc *service.WebsiteService, w http.ResponseWriter, r *http.Request) {
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
}

func parseID(value string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil || id == 0 {
		return 0, errors.New("ID 无效")
	}
	return uint(id), nil
}
