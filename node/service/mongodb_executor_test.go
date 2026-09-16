// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"context"
	"os"
	"strings"
	"testing"
)

type recordingMongoExecutor struct {
	statements []string
	failAt     int
	output     string
}

func (m *recordingMongoExecutor) Exec(_ context.Context, _ MongoDBTarget, statement string) (string, error) {
	m.statements = append(m.statements, statement)
	if m.failAt > 0 && len(m.statements) == m.failAt {
		return "", os.ErrPermission
	}
	if strings.Contains(statement, "getUser") {
		return "readWrite\n", nil
	}
	if m.output != "" {
		return m.output, nil
	}
	return "", nil
}

func TestCreateMongoDatabaseRollsBackAfterUserFailure(t *testing.T) {
	mock := &recordingMongoExecutor{failAt: 2}
	err := CreateMongoDatabase(context.Background(), mock, MongoDBTarget{}, "appdb", "appuser", "secret", "readWrite")
	if err == nil || len(mock.statements) != 3 || !strings.Contains(mock.statements[2], "dropDatabase") {
		t.Fatalf("MongoDB 创建失败未按顺序回滚: err=%v statements=%v", err, mock.statements)
	}
}

func TestMongoDatabaseScriptsMatchServerSemantics(t *testing.T) {
	mock := &recordingMongoExecutor{}
	if err := CreateMongoDatabase(context.Background(), mock, MongoDBTarget{}, "appdb", "appuser", "secret", "readWrite"); err != nil {
		t.Fatal(err)
	}
	if len(mock.statements) != 2 || !strings.Contains(mock.statements[0], `createCollection("_init")`) {
		t.Fatalf("MongoDB 创建脚本未使用合法占位 collection: %#v", mock.statements)
	}
	mock.statements = nil
	if err := DropMongoDatabase(context.Background(), mock, MongoDBTarget{}, "appdb", "appuser"); err != nil {
		t.Fatal(err)
	}
	if len(mock.statements) != 1 || !strings.Contains(mock.statements[0], "dropAllUsersFromDatabase") || !strings.Contains(mock.statements[0], "dropDatabase") {
		t.Fatalf("MongoDB 删除脚本未清理目标库用户: %#v", mock.statements)
	}
}

func TestNormalizeMongoDBOutputRemovesInteractivePrompts(t *testing.T) {
	raw := "Enter password: ***************\ntest> { ok: 1 }\nprobe_db> readWrite\n"
	if got := normalizeMongoDBOutput(raw); got != "{ ok: 1 }\nreadWrite" {
		t.Fatalf("unexpected normalized mongosh output: %q", got)
	}
}

func TestListMongoDatabasesParsesAndValidatesNames(t *testing.T) {
	mock := &recordingMongoExecutor{}
	mock.output = `["app","metrics","app"]`
	names, err := ListMongoDatabases(context.Background(), mock, MongoDBTarget{})
	if err != nil || len(names) != 2 || names[0] != "app" || names[1] != "metrics" {
		t.Fatalf("MongoDB 数据库列表解析错误: names=%v err=%v", names, err)
	}
	mock.output = `["bad;name"]`
	if _, err = ListMongoDatabases(context.Background(), mock, MongoDBTarget{}); err == nil {
		t.Fatal("非法 MongoDB 数据库名应被拒绝")
	}
}

func TestMongoDBValidatesIdentifiersAndRoles(t *testing.T) {
	mock := &recordingMongoExecutor{}
	if err := CreateMongoDatabase(context.Background(), mock, MongoDBTarget{}, "bad;drop", "user", "secret", "readWrite"); err == nil || len(mock.statements) != 0 {
		t.Fatalf("非法数据库名不应执行脚本: err=%v statements=%v", err, mock.statements)
	}
	if err := ChangeMongoPrivileges(context.Background(), mock, MongoDBTarget{}, "app", "user", "dbAdmin"); err == nil {
		t.Fatal("非法权限应被拒绝")
	}
}

func TestMongoPrivilegesScriptAndContainerCommand(t *testing.T) {
	script, err := MongoPrivilegesScript("app", "alice")
	if err != nil || !strings.Contains(script, "getUser") {
		t.Fatalf("权限脚本错误: %v %s", err, script)
	}
	t.Setenv("WORKMESH_MONGOSH_BIN", "mongosh")
	args, prompt, err := mongoDBCLICommand(MongoDBTarget{Host: "127.0.0.1", Port: 27017, Username: "root", ContainerName: "mongo-test"})
	if err != nil || !prompt || len(args) < 6 || args[0] != DockerBinary() || args[1] != "exec" {
		t.Fatalf("容器 mongosh 参数错误: args=%v prompt=%v err=%v", args, prompt, err)
	}
}
