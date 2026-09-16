// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package database

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDockerComposeTemplateKeepsDatabaseResourceContract(t *testing.T) {
	raw, err := os.ReadFile("docker-compose.yml.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	template := string(raw)

	required := []string{
		"services:\n",
		"  postgres:\n    image: postgres:18-alpine\n    container_name: workmesh-panel-postgres\n",
		"  redis:\n    image: redis:8-alpine\n    container_name: workmesh-panel-redis\n",
		"  mariadb:\n    image: mariadb:11\n    container_name: workmesh-panel-mariadb\n",
		"      - workmesh-panel-db\n",
		"  postgres_data:\n    name: workmesh-panel-postgres-data\n",
		"  redis_data:\n    name: workmesh-panel-redis-data\n",
		"  mariadb_data:\n    name: workmesh-panel-mariadb-data\n",
		"  workmesh-panel-db:\n    name: workmesh-panel-db\n    internal: true\n",
		"    file: ${WORKMESH_DB_SECRETS_DIR:-./secrets}/postgres_password\n",
		"    file: ${WORKMESH_DB_SECRETS_DIR:-./secrets}/redis_password\n",
		"    file: ${WORKMESH_DB_SECRETS_DIR:-./secrets}/mariadb_password\n",
		"    file: ${WORKMESH_DB_SECRETS_DIR:-./secrets}/mariadb_root_password\n",
		"POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password\n",
		"MARIADB_PASSWORD_FILE: /run/secrets/mariadb_password\n",
		"MARIADB_ROOT_PASSWORD_FILE: /run/secrets/mariadb_root_password\n",
		"redis-server --appendonly yes --requirepass",
		"pg_isready -U \"$${POSTGRES_USER}\" -d \"$${POSTGRES_DB}\"",
		"redis-cli --no-auth-warning ping | grep -q '^PONG$$'",
		"mariadb-admin ping -h 127.0.0.1 -u root",
	}
	for _, fragment := range required {
		if !strings.Contains(template, fragment) {
			t.Fatalf("Compose template is missing contract fragment %q", fragment)
		}
	}

	if got := strings.Count(template, "container_name:"); got != 3 {
		t.Fatalf("container_name declarations = %d, want 3", got)
	}
	if got := strings.Count(template, "    healthcheck:\n"); got != 3 {
		t.Fatalf("healthcheck declarations = %d, want 3", got)
	}
	if regexp.MustCompile(`(?m)^[ \t]*ports[ \t]*:`).MatchString(template) {
		t.Fatal("Compose template must not publish host ports")
	}
	if regexp.MustCompile(`(?m)^[ \t]*network_mode[ \t]*:`).MatchString(template) {
		t.Fatal("Compose template must not use host network mode")
	}
	for _, legacy := range []string{"WorkMesh-postgresql-ZNMP", "WorkMesh-redis-ZNMP"} {
		if strings.Contains(template, legacy) {
			t.Fatalf("Compose template must not reference legacy production resource %q", legacy)
		}
	}
	for _, service := range []string{"postgres", "redis", "mariadb"} {
		if !strings.Contains(template, "  "+service+":") {
			t.Fatalf("Compose service %q is missing", service)
		}
	}
	if strings.Contains(template, "image: postgres:latest") ||
		strings.Contains(template, "image: redis:latest") ||
		strings.Contains(template, "image: mariadb:latest") {
		t.Fatal("database images must not use latest tags")
	}
}

func TestDatabaseAcceptanceScriptKeepsIsolationGuardrails(t *testing.T) {
	raw, err := os.ReadFile("acceptance.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, fragment := range []string{
		"workmesh-acceptance-db",
		"WorkMesh-postgresql-ZNMP",
		"WorkMesh-redis-ZNMP",
		"docker info",
		"docker compose",
		"docker cp",
		"docker rm --force",
		"WORKMESH_POSTGRES_DB",
		"WORKMESH_POSTGRES_USER",
		"WORKMESH_MARIADB_DATABASE",
		"WORKMESH_MARIADB_USER",
		`cat "$SECRETS/mariadb_password"`,
		`--user="$WORKMESH_MARIADB_USER"`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("acceptance script is missing guardrail or operation %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"docker compose down -v",
		"docker system prune",
		"docker volume rm",
		"docker volume rm WorkMesh-postgresql-ZNMP",
		"docker volume rm WorkMesh-redis-ZNMP",
		"-U workmesh -d workmesh",
		"--database=workmesh",
		"--databases workmesh",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("acceptance script contains forbidden operation %q", forbidden)
		}
	}
}

func TestDatabasePreflightKeepsReadOnlyGuardrails(t *testing.T) {
	raw, err := os.ReadFile("preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(raw)
	for _, fragment := range []string{
		`SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`,
		"WORKMESH_DB_SECRETS_DIR",
		"docker compose",
		"docker ps -a",
		"WorkMesh-postgresql-ZNMP",
		"WorkMesh-redis-ZNMP",
		"config --quiet",
		"stat -c '%a'",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("database preflight is missing guardrail or check %q", fragment)
		}
	}
	for _, forbidden := range []string{
		"docker compose up",
		"docker compose down",
		"docker compose stop",
		"docker compose restart",
		"docker rm",
		"docker volume rm",
		"docker system prune",
		"certbot",
		"rm -rf",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("database preflight contains forbidden mutating operation %q", forbidden)
		}
	}
}

func TestDatabaseReadmeMatchesRuntimeAPIComposeContract(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(raw)
	for _, fragment := range []string{
		"export WORKMESH_DATA_DIR=/opt/workmesh-server/data",
		"export WORKMESH_DB_ROOT=\"$WORKMESH_DATA_DIR/database\"",
		"人工命令和 WorkMesh Runtime API 都必须使用同一个 `$WORKMESH_DATA_DIR/database/docker-compose.yml`",
		"不要额外指定 `--project-name`",
		"临时 named volume",
	} {
		if !strings.Contains(readme, fragment) {
			t.Fatalf("README is missing production contract fragment %q", fragment)
		}
	}
	if strings.Contains(readme, "--project-name workmesh-panel") {
		t.Fatal("README must not tell operators to use a project name that differs from the Runtime API")
	}
}
