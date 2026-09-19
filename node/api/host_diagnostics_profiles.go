// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"compress/gzip"
	"errors"
	"fmt"
	"net/http"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
)

// handleRuntimeProfile captures a bounded runtime profile and returns it as a
// gzip-compressed download, matching the diagnostics panel's Blob contract.
func handleRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Type     string `json:"type"`
		Duration int    `json:"duration"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeHostError(w, http.StatusBadRequest, "INVALID_PROFILE_REQUEST", err)
		return
	}
	typ := strings.ToLower(strings.TrimSpace(request.Type))
	if typ == "" {
		typ = "goroutine"
	}
	if typ != "cpu" && typ != "heap" && typ != "goroutine" && typ != "mutex" && typ != "block" {
		writeHostError(w, http.StatusBadRequest, "INVALID_PROFILE_TYPE", errors.New("profile type must be cpu, heap, goroutine, mutex, or block"))
		return
	}
	duration := request.Duration
	if duration <= 0 {
		duration = 30
	}
	if duration > 60 {
		duration = 60
	}
	filename := "workmesh-" + typ + "-" + strconv.FormatInt(time.Now().Unix(), 10) + ".prof.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	zw := gzip.NewWriter(w)
	defer zw.Close()
	if typ == "cpu" {
		if err := pprof.StartCPUProfile(zw); err != nil {
			writeHostError(w, http.StatusServiceUnavailable, "PROFILE_UNAVAILABLE", err)
			return
		}
		timer := time.NewTimer(time.Duration(duration) * time.Second)
		select {
		case <-timer.C:
		case <-r.Context().Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		pprof.StopCPUProfile()
	} else {
		profile := pprof.Lookup(typ)
		if profile == nil {
			writeHostError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", fmt.Errorf("profile %s is unavailable", typ))
			return
		}
		debugLevel := 0
		if typ == "goroutine" {
			debugLevel = 2
		}
		if err := profile.WriteTo(zw, debugLevel); err != nil {
			writeHostError(w, http.StatusInternalServerError, "PROFILE_WRITE_FAILED", err)
			return
		}
	}
}
