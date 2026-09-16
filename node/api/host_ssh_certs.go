// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	wmhttp "github.com/todaybin/workmesh-server/runtime/http"
)

var sshCertNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type sshCertRequest struct {
	ID             uint   `json:"id"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	EncryptionMode string `json:"encryptionMode"`
	PassPhrase     string `json:"passPhrase"`
	PublicKey      string `json:"publicKey"`
	PrivateKey     string `json:"privateKey"`
	Description    string `json:"description"`
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	Info           string `json:"info"`
	ForceDelete    bool   `json:"forceDelete"`
	IDs            []uint `json:"ids"`
}

func registerHostSSHCertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert", handleSSHCertCreate)
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/update", handleSSHCertUpdate)
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/sync", handleSSHCertSync)
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/search", handleSSHCertSearch)
	mux.HandleFunc("POST /api/v2/hosts/ssh/cert/delete", handleSSHCertDelete)
}

// sshCertHome returns the account home used by SSH key management. The
// override is intentionally explicit so isolated tests and operators can use
// a dedicated key directory without changing the process account.
func sshCertHome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("WORKMESH_SSH_HOME")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}

func handleSSHCertCreate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeSSHCertRequest(r)
	if err != nil {
		sshCertError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateSSHCertRequest(request, false); err != nil {
		sshCertError(w, http.StatusBadRequest, err)
		return
	}
	name := request.Name
	home, err := sshCertHome()
	if err != nil {
		sshCertError(w, http.StatusInternalServerError, err)
		return
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		sshCertError(w, 500, err)
		return
	}
	privatePath := filepath.Join(sshDir, name)
	publicPath := privatePath + ".pub"
	if _, err := os.Stat(privatePath); err == nil {
		sshCertError(w, http.StatusConflict, errors.New("SSH 密钥已存在"))
		return
	}
	private, public, err := decodeSSHCertMaterial(request)
	if err != nil {
		sshCertError(w, http.StatusBadRequest, err)
		return
	}
	if request.Mode != "input" && request.Mode != "import" {
		if request.EncryptionMode == "" {
			request.EncryptionMode = "ed25519"
		}
		private, public, err = generateSSHKey(r.Context(), request.EncryptionMode, request.PassPhrase, privatePath)
		if err != nil {
			sshCertError(w, http.StatusBadGateway, err)
			return
		}
	} else if err := writeSSHKeyPair(privatePath, publicPath, private, public); err != nil {
		sshCertError(w, http.StatusInternalServerError, err)
		return
	}
	if err := appendAuthorizedKey(home, public); err != nil {
		_ = os.Remove(privatePath)
		_ = os.Remove(publicPath)
		sshCertError(w, http.StatusInternalServerError, err)
		return
	}
	repository, err := hostRepository()
	if err != nil {
		_ = os.Remove(privatePath)
		_ = os.Remove(publicPath)
		sshCertError(w, 500, err)
		return
	}
	pass, err := encryptHostCredentials(hostCredentials{PassPhrase: request.PassPhrase})
	if err != nil {
		_ = os.Remove(privatePath)
		_ = os.Remove(publicPath)
		sshCertError(w, http.StatusServiceUnavailable, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := repository.Exec(`INSERT INTO node_ssh_certs(name,encryption_mode,pass_phrase,public_key_path,private_key_path,description,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, name, request.EncryptionMode, pass, publicPath, privatePath, request.Description, now, now)
	if err != nil {
		_ = removeAuthorizedKey(home, public)
		_ = os.Remove(privatePath)
		_ = os.Remove(publicPath)
		sshCertError(w, http.StatusConflict, err)
		return
	}
	id, _ := result.LastInsertId()
	item := map[string]any{"id": id, "name": name, "encryptionMode": request.EncryptionMode, "description": request.Description, "createdAt": now}
	wmhttp.JSON(w, http.StatusOK, map[string]any{"code": 200, "data": item})
}

func handleSSHCertUpdate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeSSHCertRequest(r)
	if err != nil {
		sshCertError(w, 400, err)
		return
	}
	if request.ID == 0 {
		sshCertError(w, 400, errors.New("SSH 密钥 ID 不能为空"))
		return
	}
	if err := validateSSHCertRequest(request, true); err != nil {
		sshCertError(w, 400, err)
		return
	}
	repository, err := hostRepository()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	var oldName, oldPub, oldPriv, oldMode, oldPass, oldDesc string
	err = repository.QueryRow(`SELECT name,public_key_path,private_key_path,encryption_mode,pass_phrase,description FROM node_ssh_certs WHERE id=?`, request.ID).Scan(&oldName, &oldPub, &oldPriv, &oldMode, &oldPass, &oldDesc)
	if errors.Is(err, sql.ErrNoRows) {
		sshCertError(w, 404, errors.New("SSH 密钥不存在"))
		return
	}
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	home, err := sshCertHome()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	private, public, err := decodeSSHCertMaterial(request)
	if err != nil {
		sshCertError(w, 400, err)
		return
	}
	if len(private) == 0 || len(public) == 0 {
		sshCertError(w, 400, errors.New("公钥和私钥不能为空"))
		return
	}
	newName := oldName
	if request.Name != "" {
		newName = request.Name
	}
	if !sshCertNamePattern.MatchString(newName) {
		sshCertError(w, 400, errors.New("SSH 密钥名称无效"))
		return
	}
	newPriv, newPub := filepath.Join(home, ".ssh", newName), filepath.Join(home, ".ssh", newName+".pub")
	// Preserve the old files when updating in place (same key name), so any
	// validation, encryption, or SQLite failure can restore the exact bytes.
	oldPrivBytes, oldPrivErr := os.ReadFile(oldPriv)
	oldPubBytes, oldPubErr := os.ReadFile(oldPub)
	oldPrivMode, oldPubMode := os.FileMode(0o600), os.FileMode(0o644)
	if info, statErr := os.Stat(oldPriv); statErr == nil {
		oldPrivMode = info.Mode().Perm()
	}
	if info, statErr := os.Stat(oldPub); statErr == nil {
		oldPubMode = info.Mode().Perm()
	}
	restoreOldFiles := func() {
		if oldPrivErr == nil {
			_ = writeAtomicSSHFile(oldPriv, oldPrivBytes, oldPrivMode)
		} else {
			_ = os.Remove(oldPriv)
		}
		if oldPubErr == nil {
			_ = writeAtomicSSHFile(oldPub, oldPubBytes, oldPubMode)
		} else {
			_ = os.Remove(oldPub)
		}
	}
	if err := writeSSHKeyPair(newPriv, newPub, private, public); err != nil {
		sshCertError(w, 500, err)
		return
	}
	oldPublic := readSSHKey(oldPub)
	if err := removeAuthorizedKey(home, oldPublic); err != nil {
		restoreOldFiles()
		sshCertError(w, 500, err)
		return
	}
	if err := appendAuthorizedKey(home, public); err != nil {
		_ = appendAuthorizedKey(home, oldPublic)
		restoreOldFiles()
		sshCertError(w, 500, err)
		return
	}
	pass, err := encryptHostCredentials(hostCredentials{PassPhrase: request.PassPhrase})
	if err != nil {
		_ = removeAuthorizedKey(home, public)
		_ = appendAuthorizedKey(home, oldPublic)
		restoreOldFiles()
		sshCertError(w, 503, err)
		return
	}
	mode := request.EncryptionMode
	if mode == "" {
		mode = oldMode
	}
	desc := request.Description
	if desc == "" {
		desc = oldDesc
	}
	_, err = repository.Exec(`UPDATE node_ssh_certs SET name=?,encryption_mode=?,pass_phrase=?,public_key_path=?,private_key_path=?,description=?,updated_at=? WHERE id=?`, newName, mode, pass, newPub, newPriv, desc, time.Now().UTC().Format(time.RFC3339Nano), request.ID)
	if err != nil {
		_ = removeAuthorizedKey(home, public)
		_ = appendAuthorizedKey(home, oldPublic)
		restoreOldFiles()
		sshCertError(w, 409, err)
		return
	}
	if oldPriv != newPriv {
		_ = os.Remove(oldPriv)
		_ = os.Remove(oldPub)
	}
	_ = oldPass
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": nil})
}

func handleSSHCertSync(w http.ResponseWriter, r *http.Request) {
	home, err := sshCertHome()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	entries, err := os.ReadDir(filepath.Join(home, ".ssh"))
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	repository, err := hostRepository()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".pub")
		if !sshCertNamePattern.MatchString(name) {
			continue
		}
		priv := filepath.Join(home, ".ssh", name)
		pub := filepath.Join(home, ".ssh", entry.Name())
		if _, err := os.Stat(priv); err != nil {
			continue
		}
		var exists int
		_ = repository.QueryRow(`SELECT 1 FROM node_ssh_certs WHERE name=?`, name).Scan(&exists)
		if exists == 1 {
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := repository.Exec(`INSERT INTO node_ssh_certs(name,encryption_mode,public_key_path,private_key_path,created_at,updated_at) VALUES(?,?,?,?,?,?)`, name, "", pub, priv, now, now); err == nil {
			count++
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"synced": count}})
}

func handleSSHCertSearch(w http.ResponseWriter, r *http.Request) {
	request, err := decodeSSHCertRequest(r)
	if err != nil {
		sshCertError(w, 400, err)
		return
	}
	repository, err := hostRepository()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	page, size := request.Page, request.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	keyword := strings.ToLower(strings.TrimSpace(request.Info))
	like := "%" + keyword + "%"
	var total int
	if err := repository.QueryRow(`SELECT COUNT(*) FROM node_ssh_certs WHERE (?='' OR lower(name) LIKE ? OR lower(description) LIKE ?)`, keyword, like, like).Scan(&total); err != nil {
		sshCertError(w, 500, err)
		return
	}
	rows, err := repository.Query(`SELECT id,name,encryption_mode,pass_phrase,public_key_path,private_key_path,description,created_at FROM node_ssh_certs WHERE (?='' OR lower(name) LIKE ? OR lower(description) LIKE ?) ORDER BY id DESC LIMIT ? OFFSET ?`, keyword, like, like, size, (page-1)*size)
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id uint
		var name, mode, pass, pubPath, privPath, desc, created string
		if err := rows.Scan(&id, &name, &mode, &pass, &pubPath, &privPath, &desc, &created); err != nil {
			continue
		}
		pub, _ := os.ReadFile(pubPath)
		priv, _ := os.ReadFile(privPath)
		item := map[string]any{"id": id, "name": name, "encryptionMode": mode, "description": desc, "publicKey": base64.StdEncoding.EncodeToString(pub), "privateKey": base64.StdEncoding.EncodeToString(priv), "createdAt": created}
		if pass != "" {
			if creds, e := decryptHostCredentials(pass); e == nil {
				item["passPhrase"] = creds.PassPhrase
			}
		}
		items = append(items, item)
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"total": total, "items": items, "page": page, "pageSize": size}})
}

func handleSSHCertDelete(w http.ResponseWriter, r *http.Request) {
	request, err := decodeSSHCertRequest(r)
	if err != nil {
		sshCertError(w, 400, err)
		return
	}
	if len(request.IDs) == 0 && request.ID != 0 {
		request.IDs = []uint{request.ID}
	}
	if len(request.IDs) == 0 {
		sshCertError(w, 400, errors.New("SSH 密钥 ID 不能为空"))
		return
	}
	repository, err := hostRepository()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	home, err := sshCertHome()
	if err != nil {
		sshCertError(w, 500, err)
		return
	}
	deleted := 0
	for _, id := range request.IDs {
		var pubPath, privPath string
		if err := repository.QueryRow(`SELECT public_key_path,private_key_path FROM node_ssh_certs WHERE id=?`, id).Scan(&pubPath, &privPath); err != nil {
			if request.ForceDelete {
				continue
			}
			sshCertError(w, 404, errors.New("SSH 密钥不存在"))
			return
		}
		pub := readSSHKey(pubPath)
		if err := removeAuthorizedKey(home, pub); err != nil && !request.ForceDelete {
			sshCertError(w, 500, err)
			return
		}
		_ = os.Remove(pubPath)
		_ = os.Remove(privPath)
		if _, err := repository.Exec(`DELETE FROM node_ssh_certs WHERE id=?`, id); err == nil {
			deleted++
		}
	}
	wmhttp.JSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"deleted": deleted}})
}

func decodeSSHCertRequest(r *http.Request) (sshCertRequest, error) {
	var request sshCertRequest
	if err := decodeJSON(r, &request); err != nil {
		return request, err
	}
	return request, nil
}

func validateSSHCertRequest(request sshCertRequest, update bool) error {
	if !update && !sshCertNamePattern.MatchString(strings.TrimSpace(request.Name)) {
		return errors.New("SSH 密钥名称无效")
	}
	mode := strings.ToLower(strings.TrimSpace(request.EncryptionMode))
	if mode != "" && mode != "rsa" && mode != "ed25519" && mode != "ecdsa" && mode != "dsa" {
		return errors.New("SSH 密钥类型无效")
	}
	if len(request.Name) > 64 || len(request.Description) > 512 || len(request.PassPhrase) > 1024 {
		return errors.New("SSH 密钥字段过长")
	}
	return nil
}

func decodeSSHCertMaterial(request sshCertRequest) ([]byte, []byte, error) {
	decode := func(value string) ([]byte, error) {
		if value == "" {
			return nil, nil
		}
		b, err := base64.StdEncoding.DecodeString(value)
		if err == nil {
			return b, nil
		}
		return []byte(value), nil
	}
	private, err := decode(request.PrivateKey)
	if err != nil {
		return nil, nil, err
	}
	public, err := decode(request.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return private, public, nil
}

func generateSSHKey(ctx context.Context, mode, passPhrase, path string) ([]byte, []byte, error) {
	if mode == "" {
		mode = "ed25519"
	}
	if mode == "dsa" {
		mode = "dsa"
	}
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return nil, nil, fmt.Errorf("ssh-keygen 不可用: %w", err)
	}
	args := []string{"-q", "-t", mode, "-f", path, "-N", passPhrase}
	command := exec.CommandContext(ctx, keygen, args...)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("生成 SSH 密钥失败: %s", strings.TrimSpace(string(output)))
	}
	private, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	public, err := os.ReadFile(path + ".pub")
	if err != nil {
		return nil, nil, err
	}
	return private, public, nil
}

func writeSSHKeyPair(privatePath, publicPath string, private, public []byte) error {
	if len(private) == 0 || len(public) == 0 {
		return errors.New("公钥和私钥不能为空")
	}
	if bytes.IndexByte(private, 0) >= 0 || bytes.IndexByte(public, 0) >= 0 {
		return errors.New("密钥内容无效")
	}
	if err := writeAtomicSSHFile(privatePath, private, 0o600); err != nil {
		return err
	}
	if err := writeAtomicSSHFile(publicPath, public, 0o644); err != nil {
		_ = os.Remove(privatePath)
		return err
	}
	return nil
}

func appendAuthorizedKey(home string, public []byte) error {
	path := filepath.Join(home, ".ssh", "authorized_keys")
	old, _ := os.ReadFile(path)
	line := strings.TrimSpace(string(public))
	if line == "" {
		return errors.New("公钥内容为空")
	}
	if strings.Contains(string(old), line) {
		return nil
	}
	content := append([]byte{}, old...)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content = append(content, '\n')
	}
	content = append(content, []byte(line+"\n")...)
	return writeAtomicSSHFile(path, content, 0o600)
}

func removeAuthorizedKey(home string, public []byte) error {
	line := strings.TrimSpace(string(public))
	if line == "" {
		return nil
	}
	path := filepath.Join(home, ".ssh", "authorized_keys")
	old, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(old), "\n")
	out := make([]string, 0, len(lines))
	for _, item := range lines {
		if strings.TrimSpace(item) != line {
			out = append(out, item)
		}
	}
	return writeAtomicSSHFile(path, []byte(strings.Join(out, "\n")), 0o600)
}

func readSSHKey(path string) []byte { value, _ := os.ReadFile(path); return value }

func sshCertError(w http.ResponseWriter, status int, err error) {
	wmhttp.JSON(w, status, map[string]any{"code": "ERR", "message": err.Error()})
}
