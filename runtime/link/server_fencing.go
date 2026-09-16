// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/todaybin/workmesh-server/runtime/role"
)

// fencing 创建角色切换动作处理器，并统一完成请求体读取和链路认证。
func (s *Server) fencing(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := readBody(r)
		if err != nil {
			writeLinkError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.authenticate(r, body); err != nil {
			writeLinkError(w, authStatus(err), err)
			return
		}
		var request role.Transition
		if len(body) > 0 && string(body) != "null" {
			if err := json.Unmarshal(body, &request); err != nil {
				writeLinkError(w, http.StatusBadRequest, err)
				return
			}
		}
		switch action {
		case "check":
			s.fencingCheck(w, r, request)
		case "prepare":
			s.fencingPrepare(w, r, request)
		case "commit":
			s.fencingCommit(w, r, request)
		case "abort":
			s.fencingAbort(w, r, request)
		}
	}
}

// fencingCheck 检查角色切换请求是否与当前节点状态匹配。
func (s *Server) fencingCheck(w http.ResponseWriter, r *http.Request, request role.Transition) {
	state := s.manager.State(r.Context())
	if err := validateTransition(state, request, false); err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"ready": true, "current": state}})
}

// fencingPrepare 记录待提交的角色切换，防止多个操作同时占用 fencing 状态。
func (s *Server) fencingPrepare(w http.ResponseWriter, r *http.Request, request role.Transition) {
	state := s.manager.State(r.Context())
	if err := validateTransition(state, request, true); err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending.OperationID != "" && s.pending.OperationID != request.OperationID {
		writeLinkError(w, http.StatusConflict, errors.New("已有角色切换操作正在准备"))
		return
	}
	s.pending = request
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "prepared", "operationId": request.OperationID}})
}

// fencingCommit 提交已准备的角色切换，并清理已完成的待处理状态。
func (s *Server) fencingCommit(w http.ResponseWriter, r *http.Request, request role.Transition) {
	s.mu.Lock()
	pending := s.pending
	s.mu.Unlock()
	if request.OperationID != "" && request.OperationID != pending.OperationID {
		writeLinkError(w, http.StatusConflict, errors.New("角色切换操作不匹配"))
		return
	}
	if pending.OperationID == "" {
		writeLinkError(w, http.StatusConflict, errors.New("没有待提交的角色切换"))
		return
	}
	state, err := s.manager.Switch(r.Context(), pending.ExpectedEpoch, pending.To)
	if err != nil {
		writeLinkError(w, http.StatusConflict, err)
		return
	}
	s.mu.Lock()
	s.pending = role.Transition{}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": state})
}

// fencingAbort 取消待处理角色切换，并在操作号冲突时拒绝误取消。
func (s *Server) fencingAbort(w http.ResponseWriter, _ *http.Request, request role.Transition) {
	s.mu.Lock()
	if request.OperationID != "" && s.pending.OperationID != "" && request.OperationID != s.pending.OperationID {
		s.mu.Unlock()
		writeLinkError(w, http.StatusConflict, errors.New("角色切换操作不匹配"))
		return
	}
	s.pending = role.Transition{}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]string{"status": "aborted"}})
}

// validateTransition 校验角色切换的节点身份、来源角色、epoch 和目标角色。
func validateTransition(state role.State, request role.Transition, requireOperation bool) error {
	if requireOperation && request.OperationID == "" {
		return errors.New("操作 ID 不能为空")
	}
	if request.NodeID != "" && request.NodeID != state.NodeID {
		return errors.New("节点身份冲突")
	}
	if request.From != "" && request.From != state.Role {
		return errors.New("源角色冲突")
	}
	if request.ExpectedEpoch != state.RoleEpoch {
		return ErrSyncConflict
	}
	if request.To != role.Primary && request.To != role.Secondary {
		return errors.New("目标角色无效")
	}
	return nil
}
