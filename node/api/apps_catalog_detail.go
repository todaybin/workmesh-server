// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"context"
	workmeshi18n "github.com/todaybin/workmesh-server/i18n"
	"io"
	"net/http"
	"strings"
	"time"
)

func phpRuntimeCatalogParams(catalog []appRecord, selected appVersionRecord) map[string]any {
	major := strings.SplitN(strings.TrimSpace(selected.Version), ".", 2)[0]
	for _, app := range catalog {
		if !strings.EqualFold(app.Key, "php") {
			continue
		}
		for _, candidate := range app.Versions {
			if strings.EqualFold(strings.TrimSpace(candidate.Version), major) && len(candidate.Params) > 0 {
				return candidate.Params
			}
		}
	}
	return selected.Params
}

// appRuntimeImage is the image repository portion expected by the runtime
// forms, which append the selected version themselves.  Runtime Compose files
// use environment placeholders, so the value is derived from the service
// type when no concrete image is present.
func appRuntimeImage(app appRecord, compose string) string {
	for _, line := range strings.Split(compose, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "image:") {
			continue
		}
		image := strings.TrimSpace(strings.TrimPrefix(trimmed, "image:"))
		image = strings.Trim(image, "\"'")
		if image != "" && !strings.Contains(image, "${") {
			if index := strings.LastIndex(image, ":"); index > strings.LastIndex(image, "/") {
				image = image[:index]
			}
			return image
		}
	}
	switch normalizeRuntimeTypeFilter(app.Type) {
	case "php":
		return "1panel-php-fpm"
	case "java":
		return "1panel/java"
	case "node":
		return "1panel/node"
	case "go":
		return "golang"
	case "python":
		return "python"
	case "dotnet":
		return "mcr.microsoft.com/dotnet/aspnet"
	default:
		return ""
	}
}

func fetchRemoteCompose(url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || len(body) == 0 {
		return ""
	}
	return string(body)
}

func appInstalledGet(w http.ResponseWriter, s *appStore, r *http.Request, id string) {
	s.mu.RLock()
	_, item := findApp(s.state.Apps, id)
	s.mu.RUnlock()
	if item.ID == "" {
		appOK(w, map[string]any{"id": id, "status": "not_installed", "env": map[string]any{}})
		return
	}
	data := appInstalledResponseData(item, workmeshi18n.LocaleFromRequest(r), s.state.CatalogTags)
	appOK(w, data)
}
