// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todaybin/workmesh-server/internal/storage"
)

// DatabaseRuntimeContainer describes the externally observable state of one
// panel-managed database container.
type DatabaseRuntimeContainer struct {
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name"`
	Status    string                `json:"status"`
	Health    string                `json:"health,omitempty"`
	Image     string                `json:"image,omitempty"`
	Ports     []DatabaseRuntimePort `json:"ports,omitempty"`
	Error     string                `json:"error,omitempty"`
	ExitCode  int                   `json:"exitCode,omitempty"`
	UpdatedAt time.Time             `json:"updatedAt,omitempty"`
}

// DatabaseRuntimePort is a normalized port binding from Docker Compose or
// docker inspect. A zero HostPort means that the container port is not
// published to the host.
type DatabaseRuntimePort struct {
	HostIP        string `json:"hostIP,omitempty"`
	HostPort      int    `json:"hostPort,omitempty"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

// DatabaseRuntimeState is the SQLite-facing runtime model. The API layer can
// persist this model through DatabaseRuntimeStateStore when a caller has a
// database handle; command execution deliberately remains independent.
type DatabaseRuntimeState struct {
	ContainerName string                `json:"containerName"`
	Status        string                `json:"status"`
	Health        string                `json:"health,omitempty"`
	Image         string                `json:"image,omitempty"`
	Ports         []DatabaseRuntimePort `json:"ports,omitempty"`
	Error         string                `json:"error,omitempty"`
	ExitCode      int                   `json:"exitCode,omitempty"`
	ObservedAt    time.Time             `json:"observedAt"`
}

// DatabaseRuntimeStateStore is intentionally small so the API can add
// persistence without coupling Docker command execution to SQLite.
type DatabaseRuntimeStateStore interface {
	EnsureDatabaseRuntimeStateSchema(context.Context) error
	UpsertDatabaseRuntimeStates(context.Context, []DatabaseRuntimeState) error
	ListDatabaseRuntimeStates(context.Context) ([]DatabaseRuntimeState, error)
}

const databaseRuntimeStateSchemaSQL = `
	CREATE TABLE IF NOT EXISTS database_runtime_states (
		container_name TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT '',
		health TEXT NOT NULL DEFAULT '',
		image TEXT NOT NULL DEFAULT '',
		ports_json TEXT NOT NULL DEFAULT '[]',
		error TEXT NOT NULL DEFAULT '',
		exit_code INTEGER NOT NULL DEFAULT 0,
		observed_at TEXT NOT NULL
	);`

// DatabaseRuntimeStateSchemaMigration creates the SQLite table for the last
// observed state of the panel-managed database containers. This is the
// authoritative startup migration; the store-level CREATE TABLE guard below
// only protects callers that use the store in isolation.
func DatabaseRuntimeStateSchemaMigration() storage.Migration {
	return storage.SQLMigration("0009-database-runtime-states", databaseRuntimeStateSchemaSQL)
}

// SQLiteDatabaseRuntimeStateStore persists the last observed container state.
// EnsureDatabaseRuntimeStateSchema is deliberately idempotent for isolated
// callers; normal server startup applies 0009 before this store is used.
type SQLiteDatabaseRuntimeStateStore struct {
	// DB is retained for isolated callers and older tests. Production callers
	// should provide Repository so the state store uses the shared boundary.
	DB         *sql.DB
	Repository storage.Transactional
}

func (s SQLiteDatabaseRuntimeStateStore) executor() (storage.Transactional, error) {
	if s.Repository != nil {
		return s.Repository, nil
	}
	if s.DB == nil {
		return nil, errors.New("SQLite 数据库不能为空")
	}
	return storage.NewSQLiteRepository(s.DB)
}

func (s SQLiteDatabaseRuntimeStateStore) EnsureDatabaseRuntimeStateSchema(ctx context.Context) error {
	repository, err := s.executor()
	if err != nil {
		return err
	}
	_, err = repository.ExecContext(ctx, databaseRuntimeStateSchemaSQL)
	return err
}

func (s SQLiteDatabaseRuntimeStateStore) UpsertDatabaseRuntimeStates(ctx context.Context, states []DatabaseRuntimeState) error {
	if err := s.EnsureDatabaseRuntimeStateSchema(ctx); err != nil {
		return err
	}
	repository, err := s.executor()
	if err != nil {
		return err
	}
	return repository.WithTx(ctx, func(tx storage.SQLExecutor) error {
		for _, state := range states {
			name := normalizeContainerName(state.ContainerName)
			if name == "" {
				return errors.New("数据库容器名称不能为空")
			}
			portsValue := state.Ports
			if portsValue == nil {
				portsValue = []DatabaseRuntimePort{}
			}
			ports, err := json.Marshal(portsValue)
			if err != nil {
				return err
			}
			observedAt := state.ObservedAt.UTC()
			if observedAt.IsZero() {
				observedAt = time.Now().UTC()
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO database_runtime_states(container_name,status,health,image,ports_json,error,exit_code,observed_at)
				VALUES(?,?,?,?,?,?,?,?)
				ON CONFLICT(container_name) DO UPDATE SET
					status=excluded.status,
					health=excluded.health,
					image=excluded.image,
					ports_json=excluded.ports_json,
					error=excluded.error,
					exit_code=excluded.exit_code,
					observed_at=excluded.observed_at`,
				name, strings.TrimSpace(state.Status), strings.TrimSpace(state.Health),
				strings.TrimSpace(state.Image), string(ports), strings.TrimSpace(state.Error),
				state.ExitCode, observedAt.Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s SQLiteDatabaseRuntimeStateStore) ListDatabaseRuntimeStates(ctx context.Context) ([]DatabaseRuntimeState, error) {
	if err := s.EnsureDatabaseRuntimeStateSchema(ctx); err != nil {
		return nil, err
	}
	repository, err := s.executor()
	if err != nil {
		return nil, err
	}
	rows, err := repository.QueryContext(ctx, `SELECT container_name,status,health,image,ports_json,error,exit_code,observed_at
		FROM database_runtime_states ORDER BY container_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []DatabaseRuntimeState
	for rows.Next() {
		var state DatabaseRuntimeState
		var portsJSON, observedAt string
		if err := rows.Scan(&state.ContainerName, &state.Status, &state.Health, &state.Image, &portsJSON, &state.Error, &state.ExitCode, &observedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(portsJSON), &state.Ports); err != nil {
			return nil, err
		}
		if observedAt != "" {
			state.ObservedAt, err = time.Parse(time.RFC3339Nano, observedAt)
			if err != nil {
				return nil, err
			}
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// ValidateDatabaseRuntimeComposeConfig verifies that a rendered Compose
// config contains exactly the panel-owned fixed container names.
func ValidateDatabaseRuntimeComposeConfig(output []byte, expected []string) error {
	var document struct {
		Services map[string]struct {
			ContainerName string `json:"container_name"`
		} `json:"services"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &document); err != nil {
		return fmt.Errorf("Compose config 不是有效 JSON: %w", err)
	}
	if len(document.Services) == 0 {
		return errors.New("Compose config 未定义服务")
	}
	expectedSet, err := expectedContainerSet(expected)
	if err != nil {
		return err
	}
	seen := make(map[string]string, len(document.Services))
	for serviceName, serviceConfig := range document.Services {
		containerName := normalizeContainerName(serviceConfig.ContainerName)
		if containerName == "" {
			return fmt.Errorf("Compose 服务 %q 未声明固定 container_name", serviceName)
		}
		if _, ok := expectedSet[containerName]; !ok {
			return fmt.Errorf("Compose 服务 %q 使用了未授权容器名 %q", serviceName, containerName)
		}
		if previous, exists := seen[containerName]; exists {
			return fmt.Errorf("Compose 容器名 %q 被服务 %q 和 %q 重复使用", containerName, previous, serviceName)
		}
		seen[containerName] = serviceName
	}
	if len(seen) != len(expectedSet) {
		return fmt.Errorf("Compose 固定容器名不完整: got=%v want=%v", sortedStringKeys(seen), sortedContainerNames(expectedSet))
	}
	for name := range expectedSet {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("Compose 缺少固定容器名 %q", name)
		}
	}
	return nil
}

// ParseDatabaseRuntimeComposePS parses the JSON emitted by
// docker compose ps --all --format json. It also accepts JSON-lines output
// emitted by older Compose versions.
func ParseDatabaseRuntimeComposePS(output []byte) ([]DatabaseRuntimeContainer, error) {
	var records []composePSRecord
	if err := unmarshalJSONArrayOrJSONLines(output, &records); err != nil {
		return nil, fmt.Errorf("Compose ps 输出不是有效 JSON: %w", err)
	}
	containers := make([]DatabaseRuntimeContainer, 0, len(records))
	for _, record := range records {
		name := normalizeContainerName(firstDatabaseRuntimeNonEmpty(record.Name, record.Names))
		if name == "" {
			continue
		}
		ports := make([]DatabaseRuntimePort, 0, len(record.Publishers))
		for _, publisher := range record.Publishers {
			ports = append(ports, DatabaseRuntimePort{
				HostIP:        strings.TrimSpace(publisher.URL),
				HostPort:      publisher.PublishedPort,
				ContainerPort: publisher.TargetPort,
				Protocol:      strings.ToLower(strings.TrimSpace(publisher.Protocol)),
			})
		}
		if len(ports) == 0 {
			ports = parsePortString(record.Ports)
		}
		containers = append(containers, DatabaseRuntimeContainer{
			ID:       strings.TrimSpace(record.ID),
			Name:     name,
			Status:   normalizeStatus(firstDatabaseRuntimeNonEmpty(record.State, record.Status)),
			Health:   normalizeHealth(record.Health),
			Image:    strings.TrimSpace(record.Image),
			Ports:    ports,
			Error:    strings.TrimSpace(firstDatabaseRuntimeNonEmpty(record.Error, record.Message)),
			ExitCode: record.ExitCode,
		})
	}
	return containers, nil
}

// ParseDatabaseRuntimeInspect parses docker inspect JSON and normalizes the
// fields needed by the API and SQLite state model.
func ParseDatabaseRuntimeInspect(output []byte) ([]DatabaseRuntimeContainer, error) {
	var records []inspectRecord
	if err := unmarshalJSONArrayOrJSONLines(output, &records); err != nil {
		return nil, fmt.Errorf("docker inspect 输出不是有效 JSON: %w", err)
	}
	containers := make([]DatabaseRuntimeContainer, 0, len(records))
	for _, record := range records {
		name := normalizeContainerName(record.Name)
		if name == "" {
			continue
		}
		ports := parseInspectPorts(record.NetworkSettings.Ports)
		status := normalizeStatus(record.State.Status)
		if status == "" && record.State.Running {
			status = "running"
		}
		containers = append(containers, DatabaseRuntimeContainer{
			ID:       strings.TrimSpace(record.ID),
			Name:     name,
			Status:   status,
			Health:   normalizeHealth(record.State.Health.Status),
			Image:    strings.TrimSpace(record.Config.Image),
			Ports:    ports,
			Error:    strings.TrimSpace(record.State.Error),
			ExitCode: record.State.ExitCode,
		})
	}
	return containers, nil
}

// MergeDatabaseRuntimeContainers combines ps and inspect data by exact
// container name. Inspect is authoritative for image, health, ports and
// errors when those values are available.
func MergeDatabaseRuntimeContainers(ps, inspect []DatabaseRuntimeContainer) []DatabaseRuntimeContainer {
	merged := make(map[string]DatabaseRuntimeContainer, len(ps)+len(inspect))
	order := make([]string, 0, len(ps)+len(inspect))
	add := func(item DatabaseRuntimeContainer) {
		name := normalizeContainerName(item.Name)
		if name == "" {
			return
		}
		item.Name = name
		current, exists := merged[name]
		if !exists {
			order = append(order, name)
		}
		if item.ID != "" {
			current.ID = item.ID
		}
		if item.Status != "" {
			current.Status = item.Status
		}
		if item.Health != "" {
			current.Health = item.Health
		}
		if item.Image != "" {
			current.Image = item.Image
		}
		if item.Ports != nil {
			current.Ports = append([]DatabaseRuntimePort(nil), item.Ports...)
		}
		if item.Error != "" {
			current.Error = item.Error
		}
		if item.ExitCode != 0 {
			current.ExitCode = item.ExitCode
		}
		current.Name = name
		merged[name] = current
	}
	for _, item := range ps {
		add(item)
	}
	for _, item := range inspect {
		add(item)
	}
	result := make([]DatabaseRuntimeContainer, 0, len(order))
	for _, name := range order {
		result = append(result, merged[name])
	}
	return result
}

// CompleteDatabaseRuntimeContainers returns one record per expected name and
// marks containers absent from Docker as missing instead of dropping them.
func CompleteDatabaseRuntimeContainers(containers []DatabaseRuntimeContainer, expected []string) ([]DatabaseRuntimeContainer, error) {
	expectedSet, err := expectedContainerSet(expected)
	if err != nil {
		return nil, err
	}
	actual := make(map[string]DatabaseRuntimeContainer, len(containers))
	for _, item := range containers {
		name := normalizeContainerName(item.Name)
		if name == "" {
			continue
		}
		if _, ok := expectedSet[name]; !ok {
			return nil, fmt.Errorf("Docker 返回未授权容器名 %q", name)
		}
		if _, duplicate := actual[name]; duplicate {
			return nil, fmt.Errorf("Docker 返回重复容器名 %q", name)
		}
		item.Name = name
		actual[name] = item
	}
	result := make([]DatabaseRuntimeContainer, 0, len(expected))
	for _, name := range expected {
		item, ok := actual[name]
		if !ok {
			item = DatabaseRuntimeContainer{Name: name, Status: "missing", Error: "容器不存在"}
		}
		result = append(result, item)
	}
	return result, nil
}

type composePSRecord struct {
	ID         string             `json:"ID"`
	Name       string             `json:"Name"`
	Names      string             `json:"Names"`
	State      string             `json:"State"`
	Status     string             `json:"Status"`
	Health     string             `json:"Health"`
	Image      string             `json:"Image"`
	Ports      string             `json:"Ports"`
	Publishers []composePublisher `json:"Publishers"`
	Error      string             `json:"Error"`
	Message    string             `json:"Message"`
	ExitCode   int                `json:"ExitCode"`
}

type composePublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

type inspectRecord struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
	} `json:"Config"`
	State struct {
		Status   string `json:"Status"`
		Running  bool   `json:"Running"`
		ExitCode int    `json:"ExitCode"`
		Error    string `json:"Error"`
		Health   struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

func unmarshalJSONArrayOrJSONLines(data []byte, target any) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	if data[0] == '[' {
		return json.Unmarshal(data, target)
	}
	lines := bytes.Split(data, []byte{'\n'})
	joined := make([]json.RawMessage, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			return errors.New("JSON 行无效")
		}
		joined = append(joined, append(json.RawMessage(nil), line...))
	}
	raw, err := json.Marshal(joined)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func parseInspectPorts(bindings map[string][]struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) []DatabaseRuntimePort {
	var ports []DatabaseRuntimePort
	for key, values := range bindings {
		containerPort, protocol := parseContainerPortKey(key)
		for _, binding := range values {
			hostPort, _ := strconv.Atoi(strings.TrimSpace(binding.HostPort))
			ports = append(ports, DatabaseRuntimePort{
				HostIP:        strings.TrimSpace(binding.HostIP),
				HostPort:      hostPort,
				ContainerPort: containerPort,
				Protocol:      protocol,
			})
		}
		if len(values) == 0 {
			ports = append(ports, DatabaseRuntimePort{ContainerPort: containerPort, Protocol: protocol})
		}
	}
	return ports
}

func parseContainerPortKey(value string) (int, string) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	port, _ := strconv.Atoi(parts[0])
	protocol := "tcp"
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		protocol = strings.ToLower(strings.TrimSpace(parts[1]))
	}
	return port, protocol
}

func parsePortString(value string) []DatabaseRuntimePort {
	var ports []DatabaseRuntimePort
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		protocol := "tcp"
		if index := strings.LastIndex(entry, "/"); index >= 0 {
			protocol = strings.ToLower(strings.TrimSpace(entry[index+1:]))
			entry = strings.TrimSpace(entry[:index])
		}
		parts := strings.Split(entry, "->")
		containerPort, _ := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		port := DatabaseRuntimePort{ContainerPort: containerPort, Protocol: protocol}
		if len(parts) == 2 {
			host := strings.TrimSpace(parts[0])
			if index := strings.LastIndex(host, ":"); index >= 0 {
				port.HostIP = strings.Trim(host[:index], "[]")
				port.HostPort, _ = strconv.Atoi(host[index+1:])
			} else {
				port.HostPort, _ = strconv.Atoi(host)
			}
		}
		if containerPort > 0 {
			ports = append(ports, port)
		}
	}
	return ports
}

func expectedContainerSet(expected []string) (map[string]struct{}, error) {
	if len(expected) == 0 {
		return nil, errors.New("固定容器名列表不能为空")
	}
	set := make(map[string]struct{}, len(expected))
	for _, item := range expected {
		name := normalizeContainerName(item)
		if !validDatabaseRuntimeContainerName(name) {
			return nil, fmt.Errorf("固定容器名无效: %q", item)
		}
		if _, exists := set[name]; exists {
			return nil, fmt.Errorf("固定容器名重复: %q", name)
		}
		set[name] = struct{}{}
	}
	return set, nil
}

func validDatabaseRuntimeContainerName(value string) bool {
	if value == "" || len(value) > 255 || strings.ContainsAny(value, " \t\r\n\x00") {
		return false
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("._-", ch) {
			continue
		}
		return false
	}
	return true
}

func normalizeContainerName(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "/")
}

func normalizeStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func normalizeHealth(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func firstDatabaseRuntimeNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedContainerNames(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
