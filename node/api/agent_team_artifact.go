// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	teamArtifactChunkSize = 8 << 20
	teamArtifactMaxBytes  = 1 << 30
	// 对账是只读诊断接口，限制返回条目数避免异常目录耗尽响应内存。
	teamArtifactReconcileMaxFiles = 512
	// 项目级对账同时限制任务目录和文件条目，避免残留文件拖垮管理请求。
	teamArtifactProjectReconcileMaxTasks = 512
	teamArtifactProjectReconcileMaxFiles = 2048
	// 对账引用来自任务快照，限制返回规模避免历史完成报告无限膨胀响应。
	teamArtifactProjectReconcileMaxReferences = 4096
	// Artifact 默认保留期只用于 dry-run 候选判断，真正回收仍需单独的人工批准流程。
	teamArtifactRetentionDuration = 30 * 24 * time.Hour
)

var teamArtifactUploadMu sync.Mutex

func handleAgentRuntimeArtifactRoute(w http.ResponseWriter, r *http.Request, s *executionState) {
	if r.Method == http.MethodPost && r.URL.Path == "/api/v2/agent-runtime/artifacts/init" {
		body, err := aiBody(r)
		if err != nil {
			teamError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		projectID := validTeamID(aiString(body, "projectId"))
		if !requireAgentRuntimeAuth(w, r, projectID) {
			return
		}
		handleAgentArtifactInit(w, s, body)
		return
	}
	if r.Method != http.MethodPut || !strings.HasSuffix(r.URL.Path, "/chunks") {
		teamError(w, http.StatusNotFound, "AGENT_ARTIFACT_ROUTE_NOT_FOUND", "Artifact 上传路由不存在")
		return
	}
	projectID := validTeamID(r.URL.Query().Get("projectId"))
	if !requireAgentRuntimeAuth(w, r, projectID) {
		return
	}
	handleAgentArtifactChunk(w, r, s, projectID)
}

func handleAgentArtifactInit(w http.ResponseWriter, s *executionState, body map[string]any) {
	projectID := validTeamID(aiString(body, "projectId"))
	runtimeID := validTeamID(aiString(body, "runtimeId"))
	fencing := strings.TrimSpace(aiString(body, "fencingToken"))
	taskID := validTeamID(aiString(body, "taskId"))
	artifactID := validTeamID(aiString(body, "artifactId"))
	if projectID == "" || runtimeID == "" || fencing == "" || taskID == "" || artifactID == "" {
		teamError(w, http.StatusBadRequest, "AGENT_ARTIFACT_FIELDS_REQUIRED", "projectId、runtimeId、fencingToken、taskId 和 artifactId 不能为空")
		return
	}
	teamArtifactUploadMu.Lock()
	defer teamArtifactUploadMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validTeamRuntimeLeaseLocked(s, projectID, runtimeID, fencing) {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	task := findTeamTaskLocked(s, taskID)
	artifact := findTeamArtifactLocked(task, artifactID)
	if task == nil || aiString(task, "projectId") != projectID || artifact == nil {
		teamError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "任务 Artifact 不存在")
		return
	}
	if aiString(artifact, "uploadStatus") == "complete" {
		finalPath, err := teamArtifactPath(projectID, taskID, artifactID, ".bin")
		if err == nil {
			if info, statErr := os.Stat(finalPath); statErr == nil && info.Size() == boundedInt64(artifact["size"], -1, teamArtifactMaxBytes) {
				aiOK(w, map[string]any{"artifactId": artifactID, "offset": info.Size(), "complete": true, "chunkSize": teamArtifactChunkSize})
				return
			}
		}
	}
	partPath, err := teamArtifactPath(projectID, taskID, artifactID, ".part")
	if err != nil {
		teamError(w, http.StatusBadRequest, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	finalPath, err := teamArtifactPath(projectID, taskID, artifactID, ".bin")
	if err != nil {
		teamError(w, http.StatusBadRequest, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	if _, statErr := os.Stat(finalPath); statErr == nil {
		teamError(w, http.StatusConflict, "ARTIFACT_UPLOAD_ALREADY_EXISTS", "Artifact 已存在但元数据未标记完成")
		return
	}
	file, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STORAGE_FAILED", "无法初始化 Artifact 存储")
		return
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(partPath)
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STORAGE_FAILED", "无法关闭 Artifact 临时文件")
		return
	}
	before := snapshotTeamStateLocked(s)
	updatedTask := updateTeamArtifactLocked(task, artifactID, map[string]any{"uploadStatus": "uploading", "uploadOffset": int64(0)})
	replaceTeamTaskLocked(s, updatedTask)
	if err := saveTeamStateLocked(s, before); err != nil {
		_ = os.Remove(partPath)
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STATE_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"artifactId": artifactID, "offset": 0, "complete": false, "chunkSize": teamArtifactChunkSize})
}

func handleAgentArtifactChunk(w http.ResponseWriter, r *http.Request, s *executionState, projectID string) {
	runtimeID := validTeamID(r.URL.Query().Get("runtimeId"))
	fencing := strings.TrimSpace(r.Header.Get("X-WorkMesh-Fencing-Token"))
	taskID := validTeamID(r.URL.Query().Get("taskId"))
	artifactID := validTeamID(r.PathValue("artifactId"))
	offsetText := strings.TrimSpace(r.URL.Query().Get("offset"))
	offset, err := strconv.ParseInt(offsetText, 10, 64)
	if projectID == "" || runtimeID == "" || fencing == "" || taskID == "" || artifactID == "" || err != nil || offset < 0 {
		teamError(w, http.StatusBadRequest, "AGENT_ARTIFACT_FIELDS_INVALID", "projectId、runtimeId、taskId、artifactId 和非负 offset 必须有效")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, teamArtifactChunkSize+1)
	chunk, err := io.ReadAll(r.Body)
	if err != nil {
		teamError(w, http.StatusRequestEntityTooLarge, "ARTIFACT_CHUNK_TOO_LARGE", "Artifact 分块超过 8MiB 限制")
		return
	}
	if len(chunk) == 0 || len(chunk) > teamArtifactChunkSize {
		teamError(w, http.StatusBadRequest, "ARTIFACT_CHUNK_INVALID", "Artifact 分块大小必须为 1 到 8MiB")
		return
	}
	teamArtifactUploadMu.Lock()
	defer teamArtifactUploadMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validTeamRuntimeLeaseLocked(s, projectID, runtimeID, fencing) {
		teamError(w, http.StatusConflict, "AGENT_RUNTIME_FENCED", "Agent runtime 租约已失效")
		return
	}
	task := findTeamTaskLocked(s, taskID)
	artifact := findTeamArtifactLocked(task, artifactID)
	if task == nil || aiString(task, "projectId") != projectID || artifact == nil {
		teamError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "任务 Artifact 不存在")
		return
	}
	if aiString(artifact, "uploadStatus") == "complete" {
		aiOK(w, map[string]any{"artifactId": artifactID, "offset": boundedInt64(artifact["size"], 0, teamArtifactMaxBytes), "complete": true})
		return
	}
	expectedSize := boundedInt64(artifact["size"], -1, teamArtifactMaxBytes)
	if expectedSize < 0 || offset > expectedSize || int64(len(chunk)) > expectedSize-offset {
		teamError(w, http.StatusRequestEntityTooLarge, "ARTIFACT_SIZE_EXCEEDED", "Artifact 分块超出登记大小")
		return
	}
	partPath, err := teamArtifactPath(projectID, taskID, artifactID, ".part")
	if err != nil {
		teamError(w, http.StatusBadRequest, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	info, err := os.Stat(partPath)
	if err != nil {
		teamError(w, http.StatusConflict, "ARTIFACT_UPLOAD_NOT_INITIALIZED", "Artifact 上传尚未初始化")
		return
	}
	if info.Size() != offset {
		teamError(w, http.StatusConflict, "ARTIFACT_OFFSET_MISMATCH", fmt.Sprintf("期望 offset %d，收到 %d", info.Size(), offset))
		return
	}
	file, err := os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STORAGE_FAILED", "无法打开 Artifact 临时文件")
		return
	}
	_, writeErr := file.Write(chunk)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STORAGE_FAILED", "无法写入 Artifact 分块")
		return
	}
	newOffset := offset + int64(len(chunk))
	if newOffset < expectedSize {
		updatedTask := updateTeamArtifactLocked(task, artifactID, map[string]any{"uploadStatus": "uploading", "uploadOffset": newOffset})
		before := snapshotTeamStateLocked(s)
		replaceTeamTaskLocked(s, updatedTask)
		if err := saveTeamStateLocked(s, before); err != nil {
			if truncateErr := os.Truncate(partPath, offset); truncateErr != nil {
				teamError(w, http.StatusInternalServerError, "ARTIFACT_STATE_ROLLBACK_FAILED", "Artifact 状态保存失败且临时文件无法回滚")
				return
			}
			teamError(w, http.StatusInternalServerError, "ARTIFACT_STATE_SAVE_FAILED", err.Error())
			return
		}
		aiOK(w, map[string]any{"artifactId": artifactID, "offset": newOffset, "complete": false})
		return
	}
	actualHash, err := teamArtifactSHA256(partPath)
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_HASH_FAILED", "无法计算 Artifact 摘要")
		return
	}
	if actualHash != strings.ToLower(aiString(artifact, "sha256")) {
		_ = os.Remove(partPath)
		before := snapshotTeamStateLocked(s)
		updatedTask := updateTeamArtifactLocked(task, artifactID, map[string]any{"uploadStatus": "hash_mismatch", "uploadOffset": int64(0)})
		replaceTeamTaskLocked(s, updatedTask)
		_ = saveTeamStateLocked(s, before)
		teamError(w, http.StatusUnprocessableEntity, "ARTIFACT_HASH_MISMATCH", "Artifact SHA-256 校验失败")
		return
	}
	finalPath, err := teamArtifactPath(projectID, taskID, artifactID, ".bin")
	if err != nil {
		teamError(w, http.StatusBadRequest, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STORAGE_FAILED", "无法提交 Artifact 文件")
		return
	}
	before := snapshotTeamStateLocked(s)
	updatedTask := updateTeamArtifactLocked(task, artifactID, map[string]any{
		"uploadStatus": "complete",
		"storedSize":   newOffset,
		"storageRef":   teamArtifactStorageRef(projectID, taskID, artifactID),
		"uploadedAt":   time.Now().UTC().Format(time.RFC3339Nano),
	})
	replaceTeamTaskLocked(s, updatedTask)
	if err := saveTeamStateLocked(s, before); err != nil {
		_ = os.Rename(finalPath, partPath)
		teamError(w, http.StatusInternalServerError, "ARTIFACT_STATE_SAVE_FAILED", err.Error())
		return
	}
	aiOK(w, map[string]any{"artifactId": artifactID, "offset": newOffset, "complete": true, "sha256": actualHash})
}

func handleDevTaskArtifactDownload(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	artifactID := validTeamID(r.PathValue("artifactId"))
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	artifact := findTeamArtifactLocked(task, artifactID)
	metadata := cloneMap(artifact)
	s.mu.RUnlock()
	if task == nil || artifact == nil || aiString(metadata, "uploadStatus") != "complete" {
		teamError(w, http.StatusNotFound, "ARTIFACT_NOT_READY", "Artifact 文件尚未准备好")
		return
	}
	path, err := teamArtifactPathReadOnly(aiString(task, "projectId"), taskID, artifactID, ".bin")
	if err != nil {
		teamError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "Artifact 文件不存在")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		teamError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "Artifact 文件不存在")
		return
	}
	defer file.Close()
	if mediaType := aiString(metadata, "mediaType"); mediaType != "" && !strings.ContainsAny(mediaType, "\r\n") {
		w.Header().Set("Content-Type", mediaType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, artifactID, fileModTime(file), file)
}

// handleDevTaskArtifactVerify 核验任务已登记 Artifact 的文件状态和 SHA-256，不修改文件。
func handleDevTaskArtifactVerify(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	projectID := aiString(task, "projectId")
	artifacts := teamArtifactHistory(nil)
	if task != nil {
		artifacts = teamArtifactHistory(task["artifacts"])
	}
	s.mu.RUnlock()
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	teamArtifactUploadMu.Lock()
	defer teamArtifactUploadMu.Unlock()
	results := make([]map[string]any, 0, len(artifacts))
	healthy := 0
	for _, artifact := range artifacts {
		result := verifyTeamArtifactFile(projectID, taskID, artifact)
		if aiString(result, "status") == "healthy" {
			healthy++
		}
		results = append(results, result)
	}
	aiOK(w, map[string]any{"items": results, "total": len(results), "healthy": healthy, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano)})
}

// handleDevTaskArtifactReconcile 只读扫描任务 Artifact 目录，报告未登记或未完成的文件。
func handleDevTaskArtifactReconcile(w http.ResponseWriter, r *http.Request) {
	taskID := validTeamID(r.PathValue("taskId"))
	s := getAIState()
	s.mu.RLock()
	task := findTeamTaskLocked(s, taskID)
	projectID := aiString(task, "projectId")
	artifacts := teamArtifactHistory(nil)
	if task != nil {
		artifacts = teamArtifactHistory(task["artifacts"])
	}
	s.mu.RUnlock()
	if task == nil {
		teamError(w, http.StatusNotFound, "TASK_NOT_FOUND", "任务不存在")
		return
	}
	teamArtifactUploadMu.Lock()
	defer teamArtifactUploadMu.Unlock()
	// 通过只读路径解析同时校验 project/task 两级目录，避免对账跟随符号链接。
	probePath, err := teamArtifactPathReadOnly(projectID, taskID, "reconcile-probe", ".bin")
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	root := filepath.Dir(probePath)
	dir, err := os.Open(root)
	if os.IsNotExist(err) {
		aiOK(w, map[string]any{"projectId": projectID, "taskId": taskID, "items": []map[string]any{}, "total": 0, "truncated": false, "destructive": false, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano)})
		return
	}
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECONCILE_FAILED", "无法打开 Artifact 目录")
		return
	}
	defer dir.Close()
	registered := make(map[string]map[string]any, len(artifacts))
	for _, artifact := range artifacts {
		registered[aiString(artifact, "artifactId")] = artifact
	}
	files := make([]map[string]any, 0, teamArtifactReconcileMaxFiles)
	// 只读取上限加一条来判定截断，避免目录中有大量残留文件时先全部载入内存。
	entries, readErr := dir.ReadDir(teamArtifactReconcileMaxFiles + 1)
	if readErr != nil && readErr != io.EOF {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECONCILE_FAILED", "无法读取 Artifact 目录")
		return
	}
	truncated := len(entries) > teamArtifactReconcileMaxFiles
	if truncated {
		entries = entries[:teamArtifactReconcileMaxFiles]
	}
	for _, entry := range entries {
		if entry.IsDir() || validTeamID(strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".part"), ".bin")) == "" {
			continue
		}
		name := entry.Name()
		artifactID := strings.TrimSuffix(strings.TrimSuffix(name, ".part"), ".bin")
		status := "unregistered"
		if metadata, ok := registered[artifactID]; ok {
			status = "registered"
			if strings.HasSuffix(name, ".part") {
				status = "partial"
			}
			if strings.HasSuffix(name, ".bin") && aiString(metadata, "uploadStatus") != "complete" {
				status = "completed_file_without_metadata"
			}
		} else if strings.HasSuffix(name, ".part") {
			status = "unregistered_partial"
		}
		info, statErr := entry.Info()
		item := map[string]any{"name": name, "artifactId": artifactID, "status": status}
		if statErr == nil {
			item["size"] = info.Size()
			item["modifiedAt"] = info.ModTime().UTC().Format(time.RFC3339Nano)
		}
		files = append(files, item)
	}
	aiOK(w, map[string]any{"projectId": projectID, "taskId": taskID, "items": files, "total": len(files), "truncated": truncated, "destructive": false, "checkedAt": time.Now().UTC().Format(time.RFC3339Nano)})
}

// handleProjectArtifactReconcile 只读扫描项目下所有任务的 Artifact 文件，给出回收候选摘要。
// 该接口不计算文件摘要、不修改状态、不删除文件，适合页面展示和后续人工回收前的 dry-run。
func handleProjectArtifactReconcile(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}

	// 先复制元数据再释放状态锁，避免目录扫描阻塞任务事件和命令状态更新。
	s := getAIState()
	registrations := make(map[string]map[string]map[string]any)
	artifactOwners := make(map[string][]string)
	taskSnapshots := make([]map[string]any, 0)
	s.mu.RLock()
	for _, task := range s.data.TeamTasks {
		if aiString(task, "projectId") != projectID {
			continue
		}
		taskID := validTeamID(aiString(task, "taskId", "id"))
		if taskID == "" {
			continue
		}
		taskSnapshots = append(taskSnapshots, cloneMap(task))
		items := make(map[string]map[string]any)
		for _, artifact := range teamArtifactHistory(task["artifacts"]) {
			if artifactID := validTeamID(aiString(artifact, "artifactId")); artifactID != "" {
				items[artifactID] = artifact
				artifactOwners[artifactID] = append(artifactOwners[artifactID], taskID)
			}
		}
		registrations[taskID] = items
	}
	s.mu.RUnlock()
	for artifactID, owners := range artifactOwners {
		sort.Strings(owners)
		artifactOwners[artifactID] = uniqueTeamStrings(owners)
	}
	references := collectProjectArtifactReferences(taskSnapshots, artifactOwners, teamArtifactProjectReconcileMaxReferences)
	referenceIndex := make(map[string]map[string]any, len(references))
	for _, reference := range references {
		referenceIndex[projectArtifactReferenceKey(aiString(reference, "taskId"), aiString(reference, "artifactId"))] = reference
	}

	teamArtifactUploadMu.Lock()
	defer teamArtifactUploadMu.Unlock()
	checkedAt := time.Now().UTC()
	projectDir, err := teamArtifactProjectDirReadOnly(projectID)
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_PATH_INVALID", err.Error())
		return
	}
	dir, err := os.Open(projectDir)
	if os.IsNotExist(err) {
		for _, reference := range references {
			if metadata := registrations[aiString(reference, "taskId")][aiString(reference, "artifactId")]; metadata != nil {
				if retentionUntil, ok := artifactRetentionUntil(metadata, time.Time{}); ok {
					reference["retentionUntil"] = retentionUntil.Format(time.RFC3339Nano)
					reference["retentionState"] = projectArtifactRetentionState(retentionUntil, checkedAt)
				}
				reference["fileStatus"] = "missing"
			} else {
				reference["fileStatus"] = "unknown_reference"
			}
		}
		aiOK(w, map[string]any{
			"projectId": projectID, "items": []map[string]any{}, "total": 0,
			"scannedTasks": 0, "scannedFiles": 0, "truncated": false,
			"summary": map[string]any{}, "references": references, "referenceCount": len(references),
			"reclaimCandidates": []map[string]any{}, "retentionDays": int(teamArtifactRetentionDuration / (24 * time.Hour)), "destructive": false,
			"checkedAt": checkedAt.Format(time.RFC3339Nano),
		})
		return
	}
	if err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECONCILE_FAILED", "无法打开项目 Artifact 目录")
		return
	}
	defer dir.Close()
	entries, readErr := dir.ReadDir(teamArtifactProjectReconcileMaxTasks + 1)
	if readErr != nil && readErr != io.EOF {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECONCILE_FAILED", "无法读取项目 Artifact 目录")
		return
	}
	truncated := len(entries) > teamArtifactProjectReconcileMaxTasks
	if truncated {
		entries = entries[:teamArtifactProjectReconcileMaxTasks]
	}

	items := make([]map[string]any, 0, teamArtifactProjectReconcileMaxFiles)
	fileRecords := make(map[string]map[string]any)
	reclaimCandidates := make([]map[string]any, 0, teamArtifactProjectReconcileMaxFiles)
	summary := make(map[string]map[string]int64)
	scannedTasks, scannedFiles := 0, 0
	addItem := func(item map[string]any) {
		if len(items) >= teamArtifactProjectReconcileMaxFiles {
			truncated = true
			return
		}
		status := aiString(item, "status")
		bytes := boundedInt64(item["size"], 0, teamArtifactMaxBytes)
		bucket := summary[status]
		if bucket == nil {
			bucket = map[string]int64{}
			summary[status] = bucket
		}
		bucket["count"]++
		bucket["bytes"] += bytes
		if taskID, ok := item["taskId"].(string); ok {
			if artifactID, ok := item["artifactId"].(string); ok {
				fileRecords[taskID+"\x00"+artifactID] = cloneMap(item)
			}
		}
		if projectArtifactBool(item["reclaimCandidate"]) && len(reclaimCandidates) < teamArtifactProjectReconcileMaxFiles {
			reclaimCandidates = append(reclaimCandidates, cloneMap(item))
		}
		items = append(items, item)
	}

	for _, taskEntry := range entries {
		if len(items) >= teamArtifactProjectReconcileMaxFiles {
			truncated = true
			break
		}
		if taskEntry.Type()&os.ModeSymlink != 0 {
			addItem(map[string]any{"taskId": taskEntry.Name(), "name": taskEntry.Name(), "status": "symlink_rejected"})
			continue
		}
		if !taskEntry.IsDir() {
			addItem(map[string]any{"name": taskEntry.Name(), "status": "unrecognized_file"})
			continue
		}
		taskID := validTeamID(taskEntry.Name())
		if taskID == "" {
			addItem(map[string]any{"name": taskEntry.Name(), "status": "unrecognized_directory"})
			continue
		}
		scannedTasks++
		taskPath, pathErr := teamArtifactPathReadOnly(projectID, taskID, "reconcile-probe", ".bin")
		if pathErr != nil {
			addItem(map[string]any{"taskId": taskID, "status": "path_invalid"})
			continue
		}
		taskDir, openErr := os.Open(filepath.Dir(taskPath))
		if os.IsNotExist(openErr) {
			continue
		}
		if openErr != nil {
			addItem(map[string]any{"taskId": taskID, "status": "read_failed"})
			continue
		}
		fileEntries, fileReadErr := taskDir.ReadDir(teamArtifactProjectReconcileMaxFiles - scannedFiles + 1)
		_ = taskDir.Close()
		if fileReadErr != nil && fileReadErr != io.EOF {
			addItem(map[string]any{"taskId": taskID, "status": "read_failed"})
			continue
		}
		if len(fileEntries) > teamArtifactProjectReconcileMaxFiles-scannedFiles {
			truncated = true
			fileEntries = fileEntries[:teamArtifactProjectReconcileMaxFiles-scannedFiles]
		}
		registered := registrations[taskID]
		for _, fileEntry := range fileEntries {
			if len(items) >= teamArtifactProjectReconcileMaxFiles {
				truncated = true
				break
			}
			if fileEntry.IsDir() {
				continue
			}
			scannedFiles++
			name := fileEntry.Name()
			artifactID, suffix, validName := teamArtifactFileName(name)
			item := map[string]any{"taskId": taskID, "name": name}
			filePath := filepath.Join(filepath.Dir(taskPath), name)
			info, statErr := os.Lstat(filePath)
			if statErr == nil {
				item["size"] = info.Size()
				item["modifiedAt"] = info.ModTime().UTC().Format(time.RFC3339Nano)
			}
			if statErr != nil {
				item["status"] = "read_failed"
				addItem(item)
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				item["status"] = "symlink_rejected"
				addItem(item)
				continue
			}
			if !validName {
				item["status"] = "unrecognized_file"
				addItem(item)
				continue
			}
			item["artifactId"] = artifactID
			metadata, registeredOK := registered[artifactID]
			if registeredOK {
				for _, key := range []string{"uploadedAt", "createdAt", "updatedAt", "sha256"} {
					if value := aiString(metadata, key); value != "" {
						item[key] = value
					}
				}
			}
			switch {
			case !registeredOK && suffix == ".part":
				item["status"] = "unregistered_partial"
			case !registeredOK:
				item["status"] = "unregistered_complete"
			case suffix == ".part":
				if aiString(metadata, "uploadStatus") == "complete" {
					item["status"] = "partial_after_complete"
				} else {
					item["status"] = "partial"
				}
			case aiString(metadata, "uploadStatus") == "complete":
				item["status"] = "registered_complete"
			default:
				item["status"] = "completed_file_without_metadata"
			}
			annotateProjectArtifactFile(item, referenceIndex[projectArtifactReferenceKey(taskID, artifactID)], checkedAt)
			addItem(item)
		}
	}
	for _, reference := range references {
		key := projectArtifactReferenceKey(aiString(reference, "taskId"), aiString(reference, "artifactId"))
		if _, exists := fileRecords[key]; exists {
			continue
		}
		if metadata := registrations[aiString(reference, "taskId")][aiString(reference, "artifactId")]; metadata != nil {
			if retentionUntil, ok := artifactRetentionUntil(metadata, time.Time{}); ok {
				reference["retentionUntil"] = retentionUntil.Format(time.RFC3339Nano)
				reference["retentionState"] = projectArtifactRetentionState(retentionUntil, checkedAt)
			}
			reference["fileStatus"] = "missing"
		} else {
			reference["fileStatus"] = "unknown_reference"
		}
	}

	aiOK(w, map[string]any{
		"projectId": projectID, "items": items, "total": len(items),
		"scannedTasks": scannedTasks, "scannedFiles": scannedFiles,
		"truncated": truncated, "summary": summary, "references": references,
		"referenceCount": len(references), "reclaimCandidates": reclaimCandidates,
		"retentionDays": int(teamArtifactRetentionDuration / (24 * time.Hour)), "destructive": false,
		"checkedAt": checkedAt.Format(time.RFC3339Nano),
	})
}

// handleProjectArtifactReclaimPlan 生成人工批准前的只读回收计划，不删除任何文件。
func handleProjectArtifactReclaimPlan(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		projectID = projectArtifactPlanPathID(r.URL.Path)
	}
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	body, err := aiBody(r)
	if err != nil {
		teamError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	idempotencyKey := boundedText(aiString(body, "idempotencyKey"), 160)
	s := getAIState()
	s.mu.Lock()
	if idempotencyKey != "" {
		for _, existing := range s.data.ArtifactReclaimPlans {
			if aiString(existing, "projectId") == projectID && aiString(existing, "idempotencyKey") == idempotencyKey {
				s.mu.Unlock()
				aiOK(w, map[string]any{"plan": sanitizeTeamMap(existing), "replayed": true})
				return
			}
		}
	}
	if reclaimPlanCountForProjectLocked(s, projectID) >= teamArtifactReclaimPlanLimit {
		s.mu.Unlock()
		teamError(w, http.StatusTooManyRequests, "ARTIFACT_RECLAIM_PLAN_LIMIT", "项目回收计划已达到保留上限")
		return
	}
	s.mu.Unlock()

	// 复用项目对账的有界、只读实现，避免计划和 dry-run 使用不同的候选判定。
	reconcileRequest := httptest.NewRequest(http.MethodGet, "/api/v2/projects/"+projectID+"/artifacts/reconcile", nil)
	reconcileRequest.SetPathValue("projectId", projectID)
	reconcileResponse := httptest.NewRecorder()
	handleProjectArtifactReconcile(reconcileResponse, reconcileRequest)
	if reconcileResponse.Code != http.StatusOK {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(reconcileResponse.Code)
		_, _ = w.Write(reconcileResponse.Body.Bytes())
		return
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(reconcileResponse.Body.Bytes(), &envelope); err != nil {
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECLAIM_PLAN_FAILED", "无法解析 Artifact 对账结果")
		return
	}
	if truncated, _ := envelope.Data["truncated"].(bool); truncated {
		teamError(w, http.StatusConflict, "ARTIFACT_RECLAIM_PLAN_TRUNCATED", "Artifact 对账结果已截断，不能生成回收计划")
		return
	}
	rawCandidates, _ := envelope.Data["reclaimCandidates"].([]any)
	items := make([]map[string]any, 0, len(rawCandidates))
	var totalBytes int64
	for _, raw := range rawCandidates {
		candidate, ok := raw.(map[string]any)
		if !ok || !projectArtifactBool(candidate["reclaimCandidate"]) || aiString(candidate, "retentionState") != "expired" || projectArtifactCandidateExternalReferenceCount(candidate) != 0 {
			continue
		}
		// 只有登记完成的 .bin 文件可以进入人工回收计划；残留分块和未知文件保留给审计处理。
		if aiString(candidate, "status") != "registered_complete" || !strings.HasSuffix(aiString(candidate, "name"), ".bin") || validTeamID(aiString(candidate, "taskId")) == "" || validTeamID(aiString(candidate, "artifactId")) == "" {
			continue
		}
		item := map[string]any{
			"taskId": aiString(candidate, "taskId"), "artifactId": aiString(candidate, "artifactId"),
			"name": aiString(candidate, "name"), "size": boundedInt64(candidate["size"], 0, teamArtifactMaxBytes),
			"sha256": aiString(candidate, "sha256"), "retentionUntil": aiString(candidate, "retentionUntil"),
			"referenceCount": int64(0), "status": "eligible", "storageRef": teamArtifactStorageRef(projectID, aiString(candidate, "taskId"), aiString(candidate, "artifactId")),
		}
		totalBytes += boundedInt64(item["size"], 0, teamArtifactMaxBytes)
		items = append(items, item)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	plan := map[string]any{
		"planId": teamNewID("reclaim"), "projectId": projectID, "status": "awaiting_approval", "destructive": false,
		"approvalRequired": true, "idempotencyKey": idempotencyKey, "createdAt": now, "checkedAt": aiString(envelope.Data, "checkedAt"),
		"retentionDays": boundedInt64(envelope.Data["retentionDays"], int64(teamArtifactRetentionDuration/(24*time.Hour)), 3650),
		"items":         items, "itemCount": len(items), "totalBytes": totalBytes,
	}
	s.mu.Lock()
	before := snapshotTeamStateLocked(s)
	if idempotencyKey != "" {
		for _, existing := range s.data.ArtifactReclaimPlans {
			if aiString(existing, "projectId") == projectID && aiString(existing, "idempotencyKey") == idempotencyKey {
				s.mu.Unlock()
				aiOK(w, map[string]any{"plan": sanitizeTeamMap(existing), "replayed": true})
				return
			}
		}
	}
	if reclaimPlanCountForProjectLocked(s, projectID) >= teamArtifactReclaimPlanLimit {
		s.mu.Unlock()
		teamError(w, http.StatusTooManyRequests, "ARTIFACT_RECLAIM_PLAN_LIMIT", "项目回收计划已达到保留上限")
		return
	}
	s.data.ArtifactReclaimPlans = append(s.data.ArtifactReclaimPlans, plan)
	if err := saveTeamStateLocked(s, before); err != nil {
		s.mu.Unlock()
		teamError(w, http.StatusInternalServerError, "ARTIFACT_RECLAIM_PLAN_SAVE_FAILED", err.Error())
		return
	}
	s.mu.Unlock()
	aiOK(w, map[string]any{"plan": sanitizeTeamMap(plan), "replayed": false})
}

func reclaimPlanCountForProjectLocked(s *executionState, projectID string) int {
	count := 0
	for _, plan := range s.data.ArtifactReclaimPlans {
		if aiString(plan, "projectId") == projectID {
			count++
		}
	}
	return count
}

// handleProjectArtifactReclaimPlanList 返回项目已有回收计划，计划仍处于人工审批控制面。
func handleProjectArtifactReclaimPlanList(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	if projectID == "" {
		projectID = projectArtifactPlanPathID(r.URL.Path)
	}
	if projectID == "" {
		teamError(w, http.StatusBadRequest, "PROJECT_ID_INVALID", "projectId 无效")
		return
	}
	page := boundedQueryInt64(r.URL.Query().Get("page"), 1, 10000)
	pageSize := boundedQueryInt64(r.URL.Query().Get("pageSize"), 50, 100)
	if page == 0 || pageSize == 0 {
		teamError(w, http.StatusBadRequest, "ARTIFACT_RECLAIM_PLAN_PAGE_INVALID", "page 和 pageSize 必须为正数")
		return
	}
	s := getAIState()
	s.mu.RLock()
	all := make([]map[string]any, 0)
	for i := len(s.data.ArtifactReclaimPlans) - 1; i >= 0; i-- {
		if aiString(s.data.ArtifactReclaimPlans[i], "projectId") == projectID {
			all = append(all, sanitizeTeamMap(s.data.ArtifactReclaimPlans[i]))
		}
	}
	s.mu.RUnlock()
	start := (page - 1) * pageSize
	items := make([]map[string]any, 0, pageSize)
	for i, item := range all {
		if int64(i) >= start && int64(i) < start+pageSize {
			items = append(items, item)
		}
	}
	aiOK(w, map[string]any{"projectId": projectID, "items": items, "total": len(all), "page": page, "pageSize": pageSize})
}

// handleProjectArtifactReclaimPlanGet 返回单个项目回收计划，项目不匹配时统一返回不存在。
func handleProjectArtifactReclaimPlanGet(w http.ResponseWriter, r *http.Request) {
	projectID := validTeamID(r.PathValue("projectId"))
	planID := validTeamID(r.PathValue("planId"))
	if projectID == "" || planID == "" {
		teamError(w, http.StatusBadRequest, "ARTIFACT_RECLAIM_PLAN_ID_INVALID", "projectId 或 planId 无效")
		return
	}
	s := getAIState()
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, plan := range s.data.ArtifactReclaimPlans {
		if aiString(plan, "planId") == planID && aiString(plan, "projectId") == projectID {
			aiOK(w, map[string]any{"plan": sanitizeTeamMap(plan)})
			return
		}
	}
	teamError(w, http.StatusNotFound, "ARTIFACT_RECLAIM_PLAN_NOT_FOUND", "回收计划不存在")
}

func projectArtifactPlanPathID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 6 && parts[0] == "api" && parts[1] == "v2" && parts[2] == "projects" && parts[4] == "artifacts" && (parts[5] == "reclaim-plan" || parts[5] == "reclaim-plans") {
		return validTeamID(parts[3])
	}
	return ""
}

type projectArtifactReference struct {
	taskID       string
	artifactID   string
	count        int
	sources      map[string]int
	ownerTaskIDs []string
}

func projectArtifactReferenceKey(taskID, artifactID string) string {
	return taskID + "\x00" + artifactID
}

func collectProjectArtifactReferences(tasks []map[string]any, owners map[string][]string, limit int) []map[string]any {
	refs := make(map[string]*projectArtifactReference)
	for _, task := range tasks {
		taskID := validTeamID(aiString(task, "taskId", "id"))
		if taskID == "" {
			continue
		}
		for _, artifact := range teamArtifactHistory(task["artifacts"]) {
			addProjectArtifactReference(refs, taskID, aiString(artifact, "artifactId"), "task_metadata", owners)
		}
		for _, report := range teamCompletionReports(task) {
			for _, artifactID := range projectArtifactIDList(report["artifactIds"]) {
				addProjectArtifactReference(refs, taskID, artifactID, "completion_report", owners)
			}
		}
	}
	keys := make([]string, 0, len(refs))
	for key := range refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	result := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		ref := refs[key]
		sources := make([]string, 0, len(ref.sources))
		for source := range ref.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		ownersCopy := append([]string(nil), ref.ownerTaskIDs...)
		result = append(result, map[string]any{
			"taskId": ref.taskID, "artifactId": ref.artifactID, "referenceCount": ref.count,
			"sources": sources, "sourceCounts": ref.sources, "ownerTaskIds": ownersCopy,
			"registered": containsTeamString(ownersCopy, ref.taskID),
			"crossTask":  len(ownersCopy) > 0 && !containsOnlyTeamString(ownersCopy, ref.taskID),
		})
	}
	return result
}

func addProjectArtifactReference(refs map[string]*projectArtifactReference, taskID, artifactID, source string, owners map[string][]string) {
	artifactID = validTeamID(artifactID)
	if taskID == "" || artifactID == "" {
		return
	}
	key := projectArtifactReferenceKey(taskID, artifactID)
	ref := refs[key]
	if ref == nil {
		ref = &projectArtifactReference{taskID: taskID, artifactID: artifactID, sources: make(map[string]int), ownerTaskIDs: append([]string(nil), owners[artifactID]...)}
		refs[key] = ref
	}
	ref.count++
	ref.sources[source]++
}

func teamCompletionReports(task map[string]any) []map[string]any {
	result := make([]map[string]any, 0, 21)
	if report, ok := task["completionReport"].(map[string]any); ok {
		result = append(result, cloneMap(report))
	}
	switch reports := task["completionReports"].(type) {
	case []any:
		for _, raw := range reports {
			if report, ok := raw.(map[string]any); ok {
				result = append(result, cloneMap(report))
			}
		}
	case []map[string]any:
		for _, report := range reports {
			result = append(result, cloneMap(report))
		}
	}
	return result
}

func projectArtifactIDList(value any) []string {
	result := make([]string, 0, 64)
	switch values := value.(type) {
	case []any:
		for _, raw := range values {
			if id := validTeamID(fmt.Sprint(raw)); id != "" {
				result = append(result, id)
			}
		}
	case []string:
		for _, raw := range values {
			if id := validTeamID(raw); id != "" {
				result = append(result, id)
			}
		}
	}
	return result
}

func annotateProjectArtifactFile(item map[string]any, reference map[string]any, checkedAt time.Time) {
	if reference != nil {
		item["referenceCount"] = boundedInt64(reference["referenceCount"], 0, teamArtifactProjectReconcileMaxReferences)
		item["sources"] = reference["sources"]
		item["crossTask"] = projectArtifactBool(reference["crossTask"])
	} else {
		item["referenceCount"] = int64(0)
		item["sources"] = []string{}
	}
	if retentionUntil, ok := artifactRetentionUntil(item, checkedAt); ok {
		item["retentionUntil"] = retentionUntil.Format(time.RFC3339Nano)
		item["retentionState"] = projectArtifactRetentionState(retentionUntil, checkedAt)
		item["reclaimCandidate"] = projectArtifactRetentionState(retentionUntil, checkedAt) == "expired" && projectArtifactExternalReferenceCount(reference) == 0
	} else {
		item["retentionState"] = "unknown"
		item["reclaimCandidate"] = false
	}
}

// projectArtifactExternalReferenceCount 不把当前任务的 Artifact 元数据登记视为外部引用。
// 完成报告引用和跨任务登记仍会阻止回收，避免删除交付证据或共享产物。
func projectArtifactExternalReferenceCount(reference map[string]any) int64 {
	if reference == nil {
		return 0
	}
	count := boundedInt64(reference["referenceCount"], 0, teamArtifactProjectReconcileMaxReferences)
	sources, _ := reference["sources"].([]string)
	if len(sources) == 1 && sources[0] == "task_metadata" && count > 0 {
		return 0
	}
	return count
}

func projectArtifactCandidateExternalReferenceCount(candidate map[string]any) int64 {
	count := boundedInt64(candidate["referenceCount"], 0, teamArtifactProjectReconcileMaxReferences)
	switch sources := candidate["sources"].(type) {
	case []any:
		if len(sources) == 1 && fmt.Sprint(sources[0]) == "task_metadata" && count > 0 {
			return 0
		}
	case []string:
		if len(sources) == 1 && sources[0] == "task_metadata" && count > 0 {
			return 0
		}
	}
	return count
}

func projectArtifactBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func artifactRetentionUntil(item map[string]any, fallback time.Time) (time.Time, bool) {
	for _, key := range []string{"uploadedAt", "modifiedAt", "updatedAt", "createdAt"} {
		if value := aiString(item, key); value != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				return parsed.Add(teamArtifactRetentionDuration), true
			}
		}
	}
	if !fallback.IsZero() {
		return fallback.Add(teamArtifactRetentionDuration), true
	}
	return time.Time{}, false
}

func projectArtifactRetentionState(until, checkedAt time.Time) string {
	if !until.IsZero() && !until.After(checkedAt) {
		return "expired"
	}
	return "within_retention"
}

func uniqueTeamStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsTeamString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsOnlyTeamString(values []string, target string) bool {
	for _, value := range values {
		if value != target {
			return false
		}
	}
	return true
}

func teamArtifactProjectDirReadOnly(projectID string) (string, error) {
	probe, err := teamArtifactPathReadOnly(projectID, "reconcile-probe-task", "reconcile-probe", ".bin")
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(probe)), nil
}

func teamArtifactFileName(name string) (string, string, bool) {
	suffix := ""
	switch {
	case strings.HasSuffix(name, ".part"):
		suffix = ".part"
	case strings.HasSuffix(name, ".bin"):
		suffix = ".bin"
	default:
		return "", "", false
	}
	artifactID := strings.TrimSuffix(name, suffix)
	return artifactID, suffix, validTeamID(artifactID) != ""
}

func verifyTeamArtifactFile(projectID, taskID string, artifact map[string]any) map[string]any {
	artifactID := aiString(artifact, "artifactId")
	result := map[string]any{"artifactId": artifactID, "expectedSize": boundedInt64(artifact["size"], -1, teamArtifactMaxBytes), "status": "metadata_invalid"}
	if aiString(artifact, "uploadStatus") != "complete" {
		result["status"] = "uploading"
		if offset := boundedInt64(artifact["uploadOffset"], 0, teamArtifactMaxBytes); offset > 0 {
			result["offset"] = offset
		}
		return result
	}
	path, err := teamArtifactPathReadOnly(projectID, taskID, artifactID, ".bin")
	if err != nil {
		result["status"] = "path_invalid"
		return result
	}
	info, err := os.Stat(path)
	if err != nil {
		result["status"] = "missing"
		return result
	}
	result["actualSize"] = info.Size()
	expectedSize := boundedInt64(artifact["size"], -1, teamArtifactMaxBytes)
	if expectedSize < 0 || info.Size() != expectedSize {
		result["status"] = "size_mismatch"
		return result
	}
	digest, err := teamArtifactSHA256(path)
	if err != nil {
		result["status"] = "read_failed"
		return result
	}
	result["sha256"] = digest
	if !strings.EqualFold(digest, aiString(artifact, "sha256")) {
		result["status"] = "hash_mismatch"
		return result
	}
	result["status"] = "healthy"
	return result
}

func validTeamRuntimeLeaseLocked(s *executionState, projectID, runtimeID, fencing string) bool {
	item := findRuntimeLocked(s, projectID, runtimeID)
	return item != nil && aiString(item, "fencingToken") == fencing && aiString(item, "status") != "fenced" && !parseTeamTime(aiString(item, "leaseExpiresAt")).Before(time.Now().UTC())
}

func findTeamArtifactLocked(task map[string]any, artifactID string) map[string]any {
	if task == nil {
		return nil
	}
	for _, item := range teamArtifactHistory(task["artifacts"]) {
		if aiString(item, "artifactId") == artifactID {
			return item
		}
	}
	return nil
}

func updateTeamArtifactLocked(task map[string]any, artifactID string, fields map[string]any) map[string]any {
	updated := cloneMap(task)
	artifacts := teamArtifactHistory(task["artifacts"])
	for i, item := range artifacts {
		if aiString(item, "artifactId") != artifactID {
			continue
		}
		copy := cloneMap(item)
		for key, value := range fields {
			copy[key] = value
		}
		artifacts[i] = copy
		break
	}
	updated["artifacts"] = artifacts
	return updated
}

func teamArtifactStorageRoot() (string, error) {
	root, err := teamArtifactStorageRootReadOnly()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func teamArtifactStorageRootReadOnly() (string, error) {
	dataDir := strings.TrimSpace(os.Getenv("WORKMESH_DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}
	dataRoot, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	root := filepath.Join(dataRoot, "workmesh-artifacts")
	info, statErr := os.Lstat(root)
	if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("Artifact 存储根目录不能是符号链接")
	}
	if statErr == nil {
		resolved, resolveErr := filepath.EvalSymlinks(root)
		if resolveErr != nil || filepath.Clean(resolved) != filepath.Clean(root) {
			return "", fmt.Errorf("Artifact 存储根目录不能是符号链接")
		}
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	return root, nil
}

func teamArtifactPath(projectID, taskID, artifactID, suffix string) (string, error) {
	return teamArtifactPathInternal(projectID, taskID, artifactID, suffix, true)
}

// teamArtifactPathReadOnly 计算 Artifact 路径但不创建任何目录，供下载和核验使用。
func teamArtifactPathReadOnly(projectID, taskID, artifactID, suffix string) (string, error) {
	return teamArtifactPathInternal(projectID, taskID, artifactID, suffix, false)
}

func teamArtifactPathInternal(projectID, taskID, artifactID, suffix string, create bool) (string, error) {
	if validTeamID(projectID) == "" || validTeamID(taskID) == "" || validTeamID(artifactID) == "" || (suffix != ".part" && suffix != ".bin") {
		return "", fmt.Errorf("Artifact 标识无效")
	}
	root, err := teamArtifactStorageRootReadOnly()
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return "", err
		}
	}
	parent := filepath.Join(root, projectID, taskID)
	if create {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return "", err
		}
	}
	// 逐级检查，既拒绝现有符号链接，也允许只读核验访问尚不存在的目录。
	for _, part := range []string{root, filepath.Join(root, projectID), parent} {
		info, statErr := os.Lstat(part)
		if os.IsNotExist(statErr) {
			if !create {
				break
			}
			return "", statErr
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Artifact 目录不能是符号链接")
		}
		if !info.IsDir() {
			return "", fmt.Errorf("Artifact 目录不是目录")
		}
	}
	path := filepath.Join(parent, artifactID+suffix)
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("Artifact 文件不能是符号链接")
	}
	return path, nil
}

func teamArtifactStorageRef(projectID, taskID, artifactID string) string {
	digest := sha256.Sum256([]byte(projectID + "\x00" + taskID + "\x00" + artifactID))
	return "artifact-" + hex.EncodeToString(digest[:16])
}

func teamArtifactSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, io.LimitReader(file, teamArtifactMaxBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func fileModTime(file *os.File) time.Time {
	info, err := file.Stat()
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
