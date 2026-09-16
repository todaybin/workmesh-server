// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package link

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// handshake 处理节点身份、角色和能力交换，并登记远端最近一次心跳信息。
func (s *Server) handshake(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request Handshake
	if err := json.Unmarshal(body, &request); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if request.NodeID == "" || request.NodeID == s.options.NodeID {
		writeLinkError(w, http.StatusBadRequest, errors.New("握手节点身份无效"))
		return
	}
	if err := s.validateSignedNode(r, request.NodeID); err != nil {
		writeLinkError(w, http.StatusUnauthorized, err)
		return
	}
	state := s.manager.State(r.Context())
	response := Handshake{
		NodeID:          state.NodeID,
		Role:            state.Role,
		RoleEpoch:       state.RoleEpoch,
		ProtocolVersion: s.options.ProtocolVersion,
		Capabilities:    append([]string(nil), s.options.Capabilities...),
		Nonce:           newResponseNonce(),
	}
	s.mu.Lock()
	s.peers[request.NodeID] = Heartbeat{
		NodeID:    request.NodeID,
		RoleEpoch: request.RoleEpoch,
		Version:   request.ProtocolVersion,
		SentAt:    s.clock().UTC().Format(time.RFC3339),
	}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": response})
}

// heartbeat 接收远端节点的存活状态，并返回本节点当前角色状态。
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request Heartbeat
	if err := json.Unmarshal(body, &request); err != nil || request.NodeID == "" {
		writeLinkError(w, http.StatusBadRequest, errors.New("心跳节点身份无效"))
		return
	}
	if err := s.validateSignedNode(r, request.NodeID); err != nil {
		writeLinkError(w, http.StatusUnauthorized, err)
		return
	}
	s.mu.Lock()
	s.peers[request.NodeID] = request
	s.mu.Unlock()
	state := s.manager.State(r.Context())
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"accepted": true, "current": state}})
}

// status 返回本节点角色和已知远端节点的最近状态。
// GET 请求没有业务请求体，但在启用链路密钥时仍必须携带完整 HMAC、时间戳、nonce 和节点身份。
func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	state := s.manager.State(r.Context())
	s.mu.Lock()
	peers := make([]Heartbeat, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"current": state, "peers": peers}})
}

// pull 读取远端请求的同步游标，并返回该游标之后的最新快照。
func (s *Server) pull(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var cursor SyncCursor
	if err := json.Unmarshal(body, &cursor); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	payload, next, err := s.store.Pull(r.Context(), cursor)
	if err != nil {
		writeLinkError(w, syncStatus(err), err)
		return
	}
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"payload": payload, "cursor": next}})
}

// push 校验同步游标并将请求快照写入共享同步存储。
func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.authenticate(r, body); err != nil {
		writeLinkError(w, authStatus(err), err)
		return
	}
	var request struct {
		Stream    string `json:"stream"`
		Version   uint64 `json:"version"`
		RoleEpoch uint64 `json:"roleEpoch"`
		Payload   []byte `json:"payload"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeLinkError(w, http.StatusBadRequest, err)
		return
	}
	state := s.manager.State(r.Context())
	if err := validateSyncRoleEpoch(request.Stream, request.RoleEpoch, state.RoleEpoch); err != nil {
		writeLinkError(w, syncStatus(err), err)
		return
	}
	next, err := s.store.Push(r.Context(), SyncCursor{Stream: request.Stream, Version: request.Version}, request.Payload)
	if err != nil {
		writeLinkError(w, syncStatus(err), err)
		return
	}
	next.RoleEpoch = state.RoleEpoch
	writeLinkJSON(w, http.StatusOK, map[string]any{"code": 200, "data": map[string]any{"cursor": next}})
}
