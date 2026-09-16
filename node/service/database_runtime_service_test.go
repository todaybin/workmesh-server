// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"strings"
	"testing"
)

var databaseRuntimeServiceTestContainers = []string{
	"workmesh-panel-postgres",
	"workmesh-panel-redis",
	"workmesh-panel-mariadb",
}

func TestValidateDatabaseRuntimeComposeConfigRequiresExactFixedNames(t *testing.T) {
	valid := `{"services":{"postgres":{"container_name":"workmesh-panel-postgres"},"redis":{"container_name":"workmesh-panel-redis"},"mariadb":{"container_name":"workmesh-panel-mariadb"}}}`
	if err := ValidateDatabaseRuntimeComposeConfig([]byte(valid), databaseRuntimeServiceTestContainers); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing service", raw: `{"services":{"postgres":{"container_name":"workmesh-panel-postgres"},"redis":{"container_name":"workmesh-panel-redis"}}}`},
		{name: "wrong name", raw: `{"services":{"postgres":{"container_name":"other"}}}`},
		{name: "missing container_name", raw: `{"services":{"postgres":{}}}`},
		{name: "invalid json", raw: `services: {}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if err := ValidateDatabaseRuntimeComposeConfig([]byte(testCase.raw), databaseRuntimeServiceTestContainers); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestParseDatabaseRuntimeComposePSAndInspect(t *testing.T) {
	ps := `[
		{"ID":"abc","Name":"workmesh-panel-postgres","State":"running","Health":"healthy","Publishers":[]},
		{"ID":"def","Name":"workmesh-panel-redis","State":"exited","Health":"unhealthy","Publishers":[{"URL":"127.0.0.1","TargetPort":6379,"PublishedPort":16379,"Protocol":"tcp"}]}
	]`
	parsedPS, err := ParseDatabaseRuntimeComposePS([]byte(ps))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsedPS) != 2 || parsedPS[1].Ports[0].HostPort != 16379 {
		t.Fatalf("unexpected ps parse: %#v", parsedPS)
	}

	inspect := `[
		{"Id":"abc","Name":"/workmesh-panel-postgres","Config":{"Image":"postgres:18-alpine"},"State":{"Status":"running","Running":true,"Health":{"Status":"healthy"}},"NetworkSettings":{"Ports":{"5432/tcp":[]}}},
		{"Id":"def","Name":"/workmesh-panel-redis","Config":{"Image":"redis:8-alpine"},"State":{"Status":"exited","ExitCode":17,"Error":"crashed","Health":{"Status":"unhealthy"}},"NetworkSettings":{"Ports":{"6379/tcp":[{"HostIp":"127.0.0.1","HostPort":"16379"}]}}}
	]`
	parsedInspect, err := ParseDatabaseRuntimeInspect([]byte(inspect))
	if err != nil {
		t.Fatal(err)
	}
	merged := MergeDatabaseRuntimeContainers(parsedPS, parsedInspect)
	completed, err := CompleteDatabaseRuntimeContainers(merged, databaseRuntimeServiceTestContainers)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 3 {
		t.Fatalf("completed=%#v", completed)
	}
	if completed[0].Image != "postgres:18-alpine" || completed[1].Error != "crashed" {
		t.Fatalf("inspect fields not merged: %#v", completed)
	}
	if completed[2].Status != "missing" || completed[2].Error == "" {
		t.Fatalf("missing container not represented: %#v", completed[2])
	}
}

func TestParseDatabaseRuntimeComposePSJSONLinesAndPorts(t *testing.T) {
	raw := `{"Name":"workmesh-panel-mariadb","State":"running","Ports":"0.0.0.0:13306->3306/tcp"}
{"Name":"workmesh-panel-redis","State":"running","Ports":"[::1]:16379->6379/tcp"}`
	containers, err := ParseDatabaseRuntimeComposePS([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 2 {
		t.Fatalf("containers=%#v", containers)
	}
	if containers[0].Ports[0].ContainerPort != 3306 || containers[0].Ports[0].HostPort != 13306 {
		t.Fatalf("mariadb ports=%#v", containers[0].Ports)
	}
	if containers[1].Ports[0].HostIP != "::1" {
		t.Fatalf("redis host ip=%q", containers[1].Ports[0].HostIP)
	}
}

func TestCompleteDatabaseRuntimeContainersRejectsUnknownAndDuplicateNames(t *testing.T) {
	_, err := CompleteDatabaseRuntimeContainers([]DatabaseRuntimeContainer{{Name: "other"}}, databaseRuntimeServiceTestContainers)
	if err == nil || !strings.Contains(err.Error(), "未授权") {
		t.Fatalf("unknown container error=%v", err)
	}
	_, err = CompleteDatabaseRuntimeContainers([]DatabaseRuntimeContainer{
		{Name: databaseRuntimeServiceTestContainers[0]},
		{Name: databaseRuntimeServiceTestContainers[0]},
	}, databaseRuntimeServiceTestContainers)
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("duplicate container error=%v", err)
	}
}
