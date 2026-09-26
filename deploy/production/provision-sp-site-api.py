#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-only
# Copyright (c) 2026 WorkMesh contributors

"""通过 workmesh-server API 部署 sp.sopvip.com。"""

from __future__ import annotations

import argparse
import base64
import io
import json
import os
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


DOMAIN = os.environ.get("WORKMESH_SP_DOMAIN", "sp.sopvip.com")
PORT = int(os.environ.get("WORKMESH_SP_PORT", "8090"))
API_URL = os.environ.get("WORKMESH_API_URL", "http://127.0.0.1:9999").rstrip("/")
API_TOKEN = os.environ.get("WORKMESH_API_TOKEN", "")
API_USER = os.environ.get("WORKMESH_ADMIN_USER", "")
API_PASSWORD = os.environ.get("WORKMESH_ADMIN_PASSWORD", "")
PG_SERVER = os.environ.get("WORKMESH_SP_PG_SERVER", "sp-postgres")
PG_HOST = os.environ.get("WORKMESH_SP_PG_HOST", "127.0.0.1")
PG_PORT = int(os.environ.get("WORKMESH_SP_PG_PORT", "5432"))
PG_ADMIN_USER = os.environ.get("WORKMESH_PG_ADMIN_USER", "")
PG_ADMIN_PASSWORD = os.environ.get("WORKMESH_PG_ADMIN_PASSWORD", "")
PG_ADMIN_DB = os.environ.get("WORKMESH_SP_PG_ADMIN_DB", "postgres")
DB_NAME = os.environ.get("WORKMESH_SP_DB_NAME", "sp_sopvip_com")
DB_USER = os.environ.get("WORKMESH_SP_DB_USER", "sp_sopvip_app")
GATEWAY_BINARY = Path(os.environ.get("WORKMESH_SP_GATEWAY_BINARY", "/www/.tmp/gateway-build-20260921/workmesh-gateway"))
FRONTEND_DIST = Path(os.environ.get("WORKMESH_SP_FRONTEND_DIST", "/www/apps/apps-view/apps/admin-view/dist"))
REFERENCE_ROOT = Path(os.environ.get("WORKMESH_SP_REFERENCE_ROOT", "/www/wwwroot/znmp.sopvip.com"))
SITE_ROOT = Path(os.environ.get("WORKMESH_SP_SITE_ROOT", f"/www/wwwroot/{DOMAIN}"))
STAGING_ROOT = Path(os.environ.get("WORKMESH_SP_STAGING_ROOT", "/www/.tmp"))


def log(message: str) -> None:
    print(f"[sp-api-provision] {message}")


def fail(message: str) -> None:
    raise SystemExit(f"[sp-api-provision] FAIL: {message}")


def encode_database_secret(value: str) -> str:
    """数据库 API 的敏感字段契约要求使用 Base64 传输，服务端会解码。"""
    return base64.b64encode(value.encode("utf-8")).decode("ascii")


def parse_pgadmin_url() -> None:
    global PG_HOST, PG_PORT, PG_ADMIN_USER, PG_ADMIN_PASSWORD, PG_ADMIN_DB
    value = os.environ.get("PGADMIN_URL", "")
    if not value:
        return
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme not in {"postgres", "postgresql"} or not parsed.hostname:
        fail("PGADMIN_URL 必须是 postgresql:// URL")
    PG_HOST = parsed.hostname
    PG_PORT = parsed.port or 5432
    PG_ADMIN_USER = urllib.parse.unquote(parsed.username or "")
    PG_ADMIN_PASSWORD = urllib.parse.unquote(parsed.password or "")
    PG_ADMIN_DB = (parsed.path or "/postgres").lstrip("/") or "postgres"


def api_call(method: str, path: str, body: object | None = None) -> dict:
    headers = {"Accept": "application/json"}
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    payload = None
    if body is not None:
        headers["Content-Type"] = "application/json"
        payload = json.dumps(body, ensure_ascii=False).encode()
    request = urllib.request.Request(f"{API_URL}{path}", data=payload, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            result = json.loads(response.read().decode("utf-8"))
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError) as exc:
        fail(f"API {method} {path} 请求失败: {exc}")
    if result.get("code") != 200:
        fail(f"API {method} {path} 业务失败: {result.get('message', 'unknown')}")
    return result


def api_upload_file(target_dir: Path, source: Path) -> None:
    """通过面板文件接口上传单个文件，避免部署脚本绕过面板直接写站点目录。"""
    boundary = f"----workmesh-{secrets.token_hex(12)}".encode()
    filename = source.name.encode("utf-8")
    content = source.read_bytes()
    body = io.BytesIO()

    def field(name: str, value: str) -> None:
        body.write(b"--" + boundary + b"\r\n")
        body.write(f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode())
        body.write(value.encode("utf-8"))
        body.write(b"\r\n")

    field("path", str(target_dir))
    body.write(b"--" + boundary + b"\r\n")
    body.write(b'Content-Disposition: form-data; name="file"; filename="' + filename + b'"\r\n')
    body.write(b"Content-Type: application/octet-stream\r\n\r\n")
    body.write(content)
    body.write(b"\r\n--" + boundary + b"--\r\n")
    headers = {
        "Accept": "application/json",
        "Content-Type": f"multipart/form-data; boundary={boundary.decode()}",
        "Content-Length": str(body.tell()),
    }
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    request = urllib.request.Request(f"{API_URL}/api/v2/files/upload", data=body.getvalue(), headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=120) as response:
            result = json.loads(response.read().decode("utf-8"))
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError) as exc:
        fail(f"面板上传文件失败 {target_dir}/{source.name}: {exc}")
    if result.get("code") != 200:
        fail(f"面板上传文件失败 {target_dir}/{source.name}: {result.get('message', 'unknown')}")

    mode = source.stat().st_mode & 0o777
    api_call("POST", "/api/v2/files/mode", {"path": str(target_dir / source.name), "mode": mode})


def api_create_directory(path: Path) -> None:
    if api_directory_exists(path):
        return
    result = api_call("POST", "/api/v2/files", {"path": str(path), "isDir": True})
    if result.get("code") != 200:
        fail(f"面板创建目录失败: {path}")


def api_directory_exists(path: Path) -> bool:
    """查询目录是否已由面板登记，避免重复部署因 409 中断。"""
    headers = {"Accept": "application/json"}
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    payload = json.dumps({"path": str(path), "showHidden": True}, ensure_ascii=False).encode()
    request = urllib.request.Request(
        f"{API_URL}/api/v2/files/search", data=payload,
        headers={**headers, "Content-Type": "application/json"}, method="POST"
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            result = json.loads(response.read().decode("utf-8"))
            return result.get("code") == 200 and bool((result.get("data") or {}).get("isDir"))
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError):
        return False


def copy_tree_to_panel_site(source_root: Path, target_root: Path) -> None:
    """把制品快速复制到面板已创建的标准站点目录。

    网站、站点目录和关系数据仍由面板 API 创建；应用制品本身不需要逐文件
    经过 HTTP 上传，直接复制可避免大型前端构建产物部署耗时过长。
    """
    target_root.mkdir(parents=True, exist_ok=True)
    for path in source_root.rglob("*"):
        if path.is_symlink():
            fail(f"应用制品包含不允许复制的符号链接: {path}")
    shutil.copytree(source_root, target_root, dirs_exist_ok=True)
    for path in source_root.rglob("*"):
        if path.is_file():
            target = target_root / path.relative_to(source_root)
            target.chmod(path.stat().st_mode & 0o777)


def login() -> None:
    global API_TOKEN
    if API_TOKEN:
        return
    if not API_USER or not API_PASSWORD:
        fail("请提供 WORKMESH_API_TOKEN，或 WORKMESH_ADMIN_USER/PASSWORD")
    result = api_call_without_auth("POST", "/api/v2/core/auth/login", {"name": API_USER, "password": API_PASSWORD})
    API_TOKEN = str(((result.get("data") or {}).get("token") or "")).strip()
    if not API_TOKEN:
        fail("面板登录响应缺少 token")


def api_call_without_auth(method: str, path: str, body: object) -> dict:
    payload = json.dumps(body, ensure_ascii=False).encode()
    request = urllib.request.Request(
        f"{API_URL}{path}",
        data=payload,
        headers={"Accept": "application/json", "Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            result = json.loads(response.read().decode("utf-8"))
    except (urllib.error.HTTPError, urllib.error.URLError, TimeoutError) as exc:
        fail(f"面板登录请求失败: {exc}")
    if result.get("code") != 200:
        fail(f"面板登录失败: {result.get('message', 'unknown')}")
    return result


def items(result: dict) -> list[dict]:
    data = result.get("data") or {}
    return data.get("items") or []


def find_item(result: dict, field: str, value: str) -> dict | None:
    for item in items(result):
        if str(item.get(field, "")).lower() == value.lower():
            return item
    return None


def delete_panel_resource(path: str, body: dict, label: str) -> None:
    """删除已经精确定位的面板资源，并保留面板的清理语义。"""
    api_call("POST", path, body)
    log(f"已通过面板接口移除旧资源: {label}")


def recreate_existing_resources() -> None:
    """仅清理 sp 自己的旧资源，绝不触碰 znmp 或双 p 历史目录。"""
    expected_site_root = Path("/www/wwwroot") / DOMAIN
    expected_runtime_id = f"{DOMAIN}-gateway"

    websites = api_call("GET", f"/api/v2/websites?name={urllib.parse.quote(DOMAIN)}&limit=20")
    website = find_item(websites, "primaryDomain", DOMAIN)
    if website:
        site_dir = Path(str(website.get("siteDir") or expected_site_root))
        if site_dir != expected_site_root:
            fail(f"拒绝删除目录异常的 {DOMAIN} 网站: {site_dir}")
        website_id = int(website.get("id") or 0)
        if website_id <= 0:
            fail(f"{DOMAIN} 网站记录缺少有效 ID，拒绝删除")
        delete_panel_resource("/api/v2/websites/del", {
            "id": website_id, "deleteApp": True, "deleteBackup": False,
            "forceDelete": True, "deleteDB": False,
        }, f"website:{DOMAIN}")

    runtimes = api_call("POST", "/api/v2/runtimes/search", {
        "page": 1, "pageSize": 200, "type": "go", "name": expected_runtime_id,
    })
    runtime = find_item(runtimes, "id", expected_runtime_id)
    if runtime:
        runtime_dir = str(runtime.get("codeDir") or runtime.get("code_dir") or "").strip()
        if runtime_dir and not (Path(runtime_dir) == expected_site_root or Path(runtime_dir).is_relative_to(expected_site_root)):
            fail(f"拒绝删除目录异常的 Go runtime {expected_runtime_id}: {runtime_dir}")
        delete_panel_resource("/api/v2/runtimes/del", {
            "id": expected_runtime_id, "forceDelete": True,
        }, f"runtime:{expected_runtime_id}")

    databases = api_call("POST", "/api/v2/databases/pg/search", {"info": DB_NAME})
    database = find_item(databases, "name", DB_NAME)
    if database:
        database_id = int(database.get("id") or 0)
        if database_id <= 0:
            fail(f"{DB_NAME} 数据库记录缺少有效 ID，拒绝删除")
        delete_panel_resource("/api/v2/databases/pg/del", {
            "id": database_id, "database": str(database.get("postgresqlName") or PG_SERVER),
        }, f"postgresql:{DB_NAME}")

    servers = api_call("POST", "/api/v2/databases/db/search", {
        "type": "postgresql", "name": PG_SERVER, "page": 1, "pageSize": 100,
    })
    server = find_item(servers, "name", PG_SERVER)
    if server:
        server_id = int(server.get("id") or 0)
        if server_id <= 0:
            fail(f"{PG_SERVER} 实例记录缺少有效 ID，拒绝删除")
        delete_panel_resource("/api/v2/databases/db/del", {"id": server_id}, f"database-server:{PG_SERVER}")


def validate() -> None:
    if DOMAIN == "znmp.sopvip.com" or DB_NAME == "znmp_sopvip_com" or DB_USER == "znmp_sopvip_app":
        fail("禁止复用 znmp 资源")
    if not re.fullmatch(r"[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}", DOMAIN):
        fail("域名无效")
    if not (1 <= PORT <= 65535 and 1 <= PG_PORT <= 65535):
        fail("端口无效")
    if SITE_ROOT != Path("/www/wwwroot") / DOMAIN:
        fail("站点目录必须遵循面板域名目录规则")
    if Path(REFERENCE_ROOT).resolve() == Path(SITE_ROOT).resolve():
        fail("参考应用目录不能与 sp.sopvip.com 站点目录相同")
    if not re.fullmatch(r"[a-z][a-z0-9_]{0,62}", DB_NAME) or not re.fullmatch(r"[a-z][a-z0-9_]{0,62}", DB_USER):
        fail("数据库名称或用户无效")


def validate_site_root(site_root: Path) -> None:
    expected = Path("/www/wwwroot") / DOMAIN
    if site_root != expected:
        fail(f"面板返回的站点目录不符合标准布局: {site_root}（期望 {expected}）")


def prepare_application_files(site_root: Path, db_password: str) -> None:
    if not GATEWAY_BINARY.is_file() or not os.access(GATEWAY_BINARY, os.X_OK):
        fail(f"Gateway 不存在或不可执行: {GATEWAY_BINARY}")
    if not (FRONTEND_DIST / "index.html").is_file():
        fail(f"前端构建产物不完整: {FRONTEND_DIST}")
    if not (REFERENCE_ROOT / "app/config").is_dir():
        fail(f"参考站点配置目录不存在: {REFERENCE_ROOT}/app/config")

    validate_site_root(site_root)
    STAGING_ROOT.mkdir(parents=True, exist_ok=True)
    staging = Path(tempfile.mkdtemp(prefix="sp-sopvip-app-", dir=str(STAGING_ROOT)))
    for name in ("config", "public", "bin", "addon", "database", "resources"):
        (staging / "app" / name).mkdir(parents=True, exist_ok=True)
    shutil.copytree(FRONTEND_DIST, staging / "app/public", dirs_exist_ok=True)
    shutil.copy2(GATEWAY_BINARY, staging / "app/bin/workmesh-gateway")
    ignored_reference_state = shutil.ignore_patterns(
        "*.db", "*.sqlite", "*.sqlite3", ".env", ".secrets", "runtime", "tmp", "cache",
        "rollback-*", "*.log",
    )
    for name in ("addon", "database", "resources", "config"):
        source = REFERENCE_ROOT / "app" / name
        if source.is_dir():
            shutil.copytree(source, staging / "app" / name, dirs_exist_ok=True, ignore=ignored_reference_state)

    def replace(path: Path, pattern: str, value: str) -> None:
        text = path.read_text(encoding="utf-8")
        path.write_text(re.sub(pattern, value, text, flags=re.MULTILINE), encoding="utf-8")

    replace(staging / "app/config/app.yaml", r"^Port: .*$", f"Port: {PORT}")
    replace(staging / "app/config/database.yaml", r"^(\s+dbname: ).*$", rf"\g<1>{DB_NAME}")
    replace(staging / "app/config/database.yaml", r"^(\s+user: ).*$", rf"\g<1>{DB_USER}")
    replace(staging / "app/config/database.yaml", r"^(\s+password: ).*$", rf'\g<1>"{db_password}"')
    replace(staging / "app/config/cache.yaml", r"^(\s+Prefix: ).*$", r'\g<1>"sp-sopvip:"')
    access_secret = ""
    existing_auth = site_root / "app/config/auth.yaml"
    if existing_auth.is_file():
        existing_text = existing_auth.read_text(encoding="utf-8")
        secret_match = re.search(r'^\s+AccessSecret:\s*["\']?([^"\'\n]+)["\']?\s*$', existing_text, flags=re.MULTILINE)
        if secret_match:
            access_secret = secret_match.group(1).strip()
    if not access_secret:
        access_secret = secrets.token_hex(32)
    replace(staging / "app/config/auth.yaml", r"^(\s+AccessSecret: ).*$", rf'\g<1>"{access_secret}"')

    try:
        # 网站、nginx、WAF、日志、SSL 和 .workmesh 目录由创建网站的面板服务维护；
        # 应用制品直接写入面板已创建的 app 根目录，避免逐文件 HTTP 上传。
        copy_tree_to_panel_site(staging / "app", site_root / "app")
    finally:
        shutil.rmtree(staging, ignore_errors=True)


def main() -> None:
    global SITE_ROOT
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--activate", action="store_true")
    parser.add_argument(
        "--recreate", action="store_true",
        help="通过面板删除并重建精确匹配的 sp 资源；不会删除历史归档",
    )
    args = parser.parse_args()
    validate()
    parse_pgadmin_url()
    if not args.apply:
        log(f"模式: dry-run，将通过 {API_URL} 部署 {DOMAIN}")
        log(f"独立 PostgreSQL: {PG_SERVER}/{DB_NAME}/{DB_USER}；Go host runtime: {DOMAIN}-gateway:{PORT}")
        return
    if os.geteuid() != 0:
        fail("--apply 需要 root")
    if not PG_ADMIN_USER or not PG_ADMIN_PASSWORD:
        fail("缺少 PostgreSQL 管理账号密码")
    login()

    if args.recreate:
        recreate_existing_resources()

    validate_site_root(SITE_ROOT)
    configured_password = os.environ.get("WORKMESH_SP_DB_PASSWORD", "").strip()
    if configured_password:
        db_password = configured_password
    else:
        config_path = SITE_ROOT / "app/config/database.yaml"
        db_password = ""
        if config_path.is_file():
            config_text = config_path.read_text(encoding="utf-8")
            match = re.search(r"^\s+password:\s*[\"']?([^\"'\n]+)[\"']?\s*$", config_text, flags=re.MULTILINE)
            if match:
                db_password = match.group(1).strip()
        if not db_password:
            db_password = secrets.token_hex(24)

    # 先让面板登记独立网站并准备 app；这样生成的数据库密码会先进入
    # sp.sopvip.com 自己的配置，数据库接口失败后重试仍能保持同一密码。
    websites = api_call("GET", f"/api/v2/websites?name={urllib.parse.quote(DOMAIN)}&limit=20")
    website = find_item(websites, "primaryDomain", DOMAIN)
    if website:
        if str(website.get("type") or "").lower() not in {"", "proxy"}:
            fail(f"{DOMAIN} 已存在但不是反向站点，拒绝覆盖其他网站类型")
        existing_proxy = str(website.get("proxy") or "").strip()
        if existing_proxy and existing_proxy != f"http://127.0.0.1:{PORT}":
            fail(f"{DOMAIN} 已指向其他代理目标: {existing_proxy}")
        SITE_ROOT = Path(str(website.get("siteDir") or SITE_ROOT))
    else:
        created = api_call("POST", "/api/v2/websites", {
            "primaryDomain": DOMAIN, "alias": DOMAIN, "type": "proxy", "protocol": "HTTP",
            "remark": "sp.sopvip.com 独立 Gateway 反向代理",
            "proxy": f"http://127.0.0.1:{PORT}",
            "domains": [{"domain": DOMAIN, "port": 80, "ssl": False}],
        })
        SITE_ROOT = Path(str((created.get("data") or {}).get("siteDir") or SITE_ROOT))
        log(f"网站已通过面板接口创建: {DOMAIN}")
    validate_site_root(SITE_ROOT)
    prepare_application_files(SITE_ROOT, db_password)

    server_search = api_call("POST", "/api/v2/databases/db/search", {"type": "postgresql", "name": PG_SERVER})
    if not find_item(server_search, "name", PG_SERVER):
        api_call("POST", "/api/v2/databases/db", {
            "name": PG_SERVER, "type": "postgresql", "version": "18", "from": "external",
            "host": PG_HOST, "address": PG_HOST, "port": PG_PORT, "initialDB": PG_ADMIN_DB,
            "username": PG_ADMIN_USER, "password": encode_database_secret(PG_ADMIN_PASSWORD),
            "description": "sp.sopvip.com 独立 PostgreSQL 实例",
        })
        log(f"PostgreSQL 实例已通过面板接口登记: {PG_SERVER}")

    db_search = api_call("POST", "/api/v2/databases/pg/search", {"info": DB_NAME})
    existing_db = find_item(db_search, "name", DB_NAME)
    if existing_db:
        existing_user = str(existing_db.get("username") or existing_db.get("user") or "").strip()
        if existing_user and existing_user != DB_USER:
            fail(f"数据库 {DB_NAME} 已绑定其他账号，拒绝与 znmp 或其他站点混用")
    else:
        api_call("POST", "/api/v2/databases/pg", {
            "database": PG_SERVER, "name": DB_NAME, "username": DB_USER,
            "password": encode_database_secret(db_password), "from": "external",
            "description": "sp.sopvip.com 独立数据库", "superUser": False,
        })
        log(f"PostgreSQL 数据库已通过面板接口创建: {DB_NAME}")

    runtimes = api_call("POST", "/api/v2/runtimes/search", {"page": 1, "pageSize": 200, "type": "go", "name": f"{DOMAIN}-gateway"})
    existing_runtime = find_item(runtimes, "id", f"{DOMAIN}-gateway")
    if existing_runtime:
        existing_code_dir = str(existing_runtime.get("codeDir") or existing_runtime.get("code_dir") or "").strip()
        if existing_code_dir and Path(existing_code_dir) != SITE_ROOT / "app":
            fail(f"Go runtime 已指向其他站点目录: {existing_code_dir}")
    else:
        api_call("POST", "/api/v2/runtimes", {
            "id": f"{DOMAIN}-gateway", "name": f"{DOMAIN}-gateway", "type": "go",
            "mode": "host", "version": "host", "codeDir": str(SITE_ROOT / "app"),
            "workDir": str(SITE_ROOT / "app"), "remark": "宿主机 Supervisor 管理的 Gateway",
            "resource": "custom", "source": "host",
            "params": {
                "APP_PORT": PORT, "CONTAINER_NAME": f"{DOMAIN}-gateway",
                "EXEC_SCRIPT": "./bin/workmesh-gateway api -f config",
                "HOST_IP": "0.0.0.0", "RUNTIME_MODE": "host",
            },
            "port": PORT, "container": f"{DOMAIN}-gateway",
            "exposedPorts": [{"containerPort": PORT, "hostPort": PORT, "protocol": "tcp"}],
        })
        log(f"Go host runtime 已通过面板接口创建: {DOMAIN}-gateway")

    websites = api_call("GET", f"/api/v2/websites?name={urllib.parse.quote(DOMAIN)}&limit=20")
    runtimes = api_call("POST", "/api/v2/runtimes/search", {"page": 1, "pageSize": 200, "type": "go", "name": f"{DOMAIN}-gateway"})
    databases = api_call("POST", "/api/v2/databases/pg/search", {"info": DB_NAME})
    if not find_item(websites, "primaryDomain", DOMAIN):
        fail("API 校验失败：反向站点未显示")
    if not find_item(runtimes, "id", f"{DOMAIN}-gateway"):
        fail("API 校验失败：Go runtime 未显示")
    if not find_item(databases, "name", DB_NAME):
        fail("API 校验失败：PostgreSQL 数据库未显示")
    log("API 校验通过：反向站点、Go host runtime、PostgreSQL 资源均可读取")

    if args.activate:
        subprocess.run(["nginx", "-t"], check=True)
        subprocess.run(["nginx", "-s", "reload"], check=True)
    log("完成：面板 API 已写入权威状态，网站与 Go 运行环境应立即显示")


if __name__ == "__main__":
    main()
