local cjson = require("cjson.safe")
local cache = ngx.shared.workmesh_waf
local root = "/opt/workmesh/waf/data"
local default_global = {
    enabled = true,
    standardRules = true,
    mode = "observe",
    requestBodyLimit = 1048576,
}

-- ngx.stat is not available in all OpenResty builds.  Keep file based
-- configuration functional by falling back to a non-cached read when the
-- helper is missing instead of aborting the request with a 500.
local function file_stamp(name)
    if ngx.stat then
        local stat = ngx.stat(name)
        if not stat then return nil end
        return tostring(stat.mtime or 0) .. ":" .. tostring(stat.size or 0) .. ":" .. tostring(stat.ino or 0)
    end
    local probe = io.open(name, "r")
    if not probe then return nil end
    probe:close()
    return nil
end

-- Cache decoded JSON by mtime/size so a saved panel configuration is visible
-- without waiting for a worker restart or a fixed TTL to expire.
local function decode_file(name, fallback)
    local stamp = file_stamp(name)
    if not stamp and not ngx.stat then
        local handle = io.open(name, "r")
        if not handle then return fallback end
        local value = cjson.decode(handle:read("*a"))
        handle:close()
        return value or fallback
    end
    if not stamp then return fallback end
    local cache_key = "json:" .. name
    local stamp_key = cache_key .. ":stamp"
    local cached = cache:get(cache_key)
    if cached and cache:get(stamp_key) == stamp then
        return cjson.decode(cached) or fallback
    end
    local handle = io.open(name, "r")
    if not handle then return fallback end
    local data = handle:read("*a")
    handle:close()
    local value = cjson.decode(data)
    if not value then
        ngx.log(ngx.ERR, "workmesh_waf invalid JSON: ", name)
        return fallback
    end
    cache:set(cache_key, cjson.encode(value), 30)
    cache:set(stamp_key, stamp, 30)
    return value
end

local function append_audit(event)
    local function optional_var(name)
        local ok, value = pcall(function() return ngx.var[name] end)
        if not ok or value == nil or value == "" then return nil end
        return tostring(value)
    end
    if event.ipRegion == nil then
        event.ipRegion = optional_var("geoip2_data_country_name")
            or optional_var("geoip_country_name")
            or optional_var("geoip2_data_country_code")
            or optional_var("geoip_country_code")
    end
    local encoded = cjson.encode(event)
    if not encoded then return end
    ngx.log(ngx.WARN, "workmesh_waf ", encoded)
    local audit = io.open("/opt/workmesh/waf/logs/workmesh-custom-audit.jsonl", "a")
    if audit then audit:write(encoded, "\n"); audit:close() end
end

local function host_name()
    local host = ngx.var.host or "_"
    if not host:match("^[%w%._%-]+$") then return "_" end
    return host
end

local function decode_site_config(site_dir, fallback)
    -- config.json is the current Server contract. Keep site.json as a
    -- read-only compatibility fallback for sites created by the old node
    -- runtime; otherwise those settings would be silently ignored.
    local site = decode_file(site_dir .. "config.json", nil)
    if type(site) == "table" then return site end
    site = decode_file(site_dir .. "site.json", nil)
    if type(site) == "table" then return site end
    return fallback
end

local global = decode_file(root .. "/global.json", default_global)
if not global.enabled then return end
local access_lists = decode_file(root .. "/access-lists.json", { whitelist = {}, blacklist = {} })
local client_ip = ngx.var.remote_addr or ""
local list_enabled = access_lists.enabled or {}

local function trim(value)
    return tostring(value or ""):match("^%s*(.-)%s*$")
end

local function split_colons(value)
    local result = {}
    for item in tostring(value or ""):gmatch("[^:]+") do
        result[#result + 1] = item
    end
    return result
end

local function normalize_ipv6(value)
    value = trim(value):lower()
    if value == "" or not value:find(":", 1, true) then return nil end
    local left, right = value:match("^(.-)::(.-)$")
    local parts
    if left ~= nil then
        local left_parts = split_colons(left)
        local right_parts = split_colons(right)
        if #left_parts + #right_parts >= 8 then return nil end
        parts = {}
        for _, item in ipairs(left_parts) do parts[#parts + 1] = item end
        for _ = 1, 8 - #left_parts - #right_parts do parts[#parts + 1] = "0" end
        for _, item in ipairs(right_parts) do parts[#parts + 1] = item end
    else
        parts = split_colons(value)
        if #parts ~= 8 then return nil end
    end
    if #parts ~= 8 then return nil end
    for index, item in ipairs(parts) do
        if #item < 1 or #item > 4 or not item:match("^[0-9a-fA-F]+$") then return nil end
        parts[index] = string.format("%04x", tonumber(item, 16))
    end
    return table.concat(parts)
end

local function cidr_match(ip, cidr)
    ip, cidr = trim(ip), trim(cidr)
    local ia, ib, ic, id = ip:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
    local na, nb, nc, nd, prefix = cidr:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)/(%d+)$")
    if ia and na then
        local octets = { tonumber(ia), tonumber(ib), tonumber(ic), tonumber(id) }
        local network = { tonumber(na), tonumber(nb), tonumber(nc), tonumber(nd) }
        for index = 1, 4 do
            if octets[index] > 255 or network[index] > 255 then return false end
        end
        local bits = tonumber(prefix)
        if not bits or bits < 0 or bits > 32 then return false end
        local ipnum = ((octets[1] * 256 + octets[2]) * 256 + octets[3]) * 256 + octets[4]
        local netnum = ((network[1] * 256 + network[2]) * 256 + network[3]) * 256 + network[4]
        if bits == 0 then return true end
        local block = 2 ^ (32 - bits)
        return ipnum - ipnum % block == netnum - netnum % block
    end

    local ip6, prefix6 = ip:match("^(.+)/(%d+)$")
    if ip6 then ip = ip6 end
    local network6, network_prefix = cidr:match("^(.+)/(%d+)$")
    if not network6 or ip:find("%.", 1, true) or network6:find("%.", 1, true) then return false end
    local bits6 = tonumber(network_prefix or prefix6)
    if not bits6 or bits6 < 0 or bits6 > 128 then return false end
    local ip_hex, network_hex = normalize_ipv6(ip), normalize_ipv6(network6)
    if not ip_hex or not network_hex then return false end
    if bits6 == 0 then return true end
    local full_nibbles = math.floor(bits6 / 4)
    if full_nibbles > 0 and ip_hex:sub(1, full_nibbles) ~= network_hex:sub(1, full_nibbles) then
        return false
    end
    local remaining = bits6 % 4
    if remaining == 0 then return true end
    local ip_nibble = tonumber(ip_hex:sub(full_nibbles + 1, full_nibbles + 1), 16)
    local network_nibble = tonumber(network_hex:sub(full_nibbles + 1, full_nibbles + 1), 16)
    local mask = 2 ^ (4 - remaining) - 1
    return (ip_nibble - ip_nibble % (mask + 1)) == (network_nibble - network_nibble % (mask + 1))
end

local function list_item_enabled(category, item)
    local meta = access_lists.listMeta or {}
    local entry = meta[tostring(category or "") .. ":" .. tostring(item or "")]
    return type(entry) ~= "table" or entry.enabled ~= false
end

local function in_list(items, category)
    for _, item in ipairs(items or {}) do
        item = trim(item)
        if list_item_enabled(category, item) and (item == client_ip or cidr_match(client_ip, item)) then return true end
    end
    return false
end

local ip_groups = {}
for _, group in ipairs(access_lists.ipGroups or {}) do
    if type(group) == "table" and group.enabled ~= false then
        local name = trim(group.name)
        if name ~= "" then
            ip_groups[string.lower(name)] = group.entries or {}
        end
    end
end

local function ip_group_match(name)
    local entries = ip_groups[string.lower(trim(name))]
    return entries ~= nil and in_list(entries)
end

local redis_config = global.redis or {}
local redis_enabled = redis_config.enabled == true
local redis_host = trim(redis_config.host)
local redis_port = tonumber(redis_config.port) or 6379
local redis_db = tonumber(redis_config.db) or 0
local redis_password = tostring(redis_config.password or "")
local redis_error_key = "redis-error:" .. redis_host .. ":" .. tostring(redis_port)
local redis_eval_rate = "local v=redis.call('INCR',KEYS[1]);if v==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]);end;return v"

local function redis_log_error(message)
    message = tostring(message or "unknown error")
    local should_log = true
    if cache and cache.add then
        local ok = cache:add(redis_error_key, true, 60)
        should_log = ok == true
    end
    if should_log then
        ngx.log(ngx.ERR, "workmesh_waf Redis unavailable (local fallback): ", message)
    end
end

local function redis_response(sock)
    local line, err = sock:receive("*l")
    if not line then return nil, err or "empty response" end
    local prefix, payload = line:sub(1, 1), line:sub(2)
    if prefix == "+" then return payload end
    if prefix == "-" then return nil, payload end
    if prefix == ":" then
        local number = tonumber(payload)
        if number == nil then return nil, "invalid integer response" end
        return number
    end
    if prefix == "$" then
        local length = tonumber(payload)
        if length == nil then return nil, "invalid bulk response" end
        if length < 0 then return nil end
        local value, read_err = sock:receive(length)
        if not value then return nil, read_err or "bulk response read failed" end
        local _, crlf_err = sock:receive(2)
        if crlf_err then return nil, crlf_err end
        return value
    end
    return nil, "unsupported Redis response"
end

local function redis_command(parts)
    if not redis_enabled then return nil, "disabled" end
    if redis_host == "" or redis_port < 1 or redis_port > 65535 or redis_db < 0 then
        redis_log_error("invalid Redis configuration")
        return nil, "invalid configuration"
    end
    local socket = ngx.socket.tcp()
    socket:settimeout(100)
    local connected, connect_err = socket:connect(redis_host, redis_port)
    if not connected then
        redis_log_error("connect " .. redis_host .. ":" .. tostring(redis_port) .. ": " .. tostring(connect_err))
        return nil, connect_err
    end
    local commands = {}
    if redis_password ~= "" then
        commands[#commands + 1] = {"AUTH", redis_password}
    end
    if redis_db ~= 0 then
        commands[#commands + 1] = {"SELECT", tostring(redis_db)}
    end
    commands[#commands + 1] = parts
    -- Pipeline AUTH/SELECT/command on one connection so every request uses
    -- the configured Redis user context without exposing credentials in logs.
    local payload = ""
    for _, command in ipairs(commands) do
        payload = payload .. "*" .. tostring(#command) .. "\r\n"
        for _, value in ipairs(command) do
            value = tostring(value)
            payload = payload .. "$" .. tostring(#value) .. "\r\n" .. value .. "\r\n"
        end
    end
    local sent, send_err = socket:send(payload)
    if not sent then
        socket:close()
        redis_log_error("send failed: " .. tostring(send_err))
        return nil, send_err
    end
    for index = 1, #commands do
        local response, response_err = redis_response(socket)
        if response == nil and response_err then
            socket:close()
            redis_log_error("command failed: " .. tostring(response_err))
            return nil, response_err
        end
        if index < #commands and response ~= "OK" then
            socket:close()
            redis_log_error("Redis authentication or database selection failed")
            return nil, "Redis context setup failed"
        end
        if index == #commands then
            socket:setkeepalive(10000, 20)
            return response
        end
    end
    socket:close()
    return nil, "Redis command did not return"
end

local function local_cache_get(key)
    return cache and cache:get(key) ~= nil
end

local function local_cache_incr(key, period)
    if not cache then return nil, "shared dict unavailable" end
    local count, err = cache:incr(key, 1, 0, period)
    if not count then return nil, err or "shared dict incr failed" end
    return count
end

local function redis_exists(key)
    if not redis_enabled then return nil, "disabled" end
    local value, err = redis_command({"EXISTS", key})
    if value == nil then return nil, err end
    return tonumber(value) == 1
end

local function redis_increment(key, period)
    local value, err = redis_command({"EVAL", redis_eval_rate, "1", key, tostring(period)})
    if value == nil then return nil, err end
    return tonumber(value)
end

local function redis_set_expiry(key, seconds)
    local value, err = redis_command({"SET", key, "1", "EX", tostring(seconds)})
    if value == nil then return nil, err end
    return value == "OK"
end

local host = host_name()
local function text_in_list(value, items, category)
    value = tostring(value or "")
    for _, item in ipairs(items or {}) do
        if list_item_enabled(category, item) and
            (value == tostring(item) or string.find(string.lower(value), string.lower(tostring(item)), 1, true)) then
            return true
        end
    end
    return false
end

if list_enabled.whitelist ~= false and in_list(access_lists.whitelist, "whitelist") then return end
if list_enabled.urlWhitelist ~= false and text_in_list(ngx.var.uri, access_lists.urlWhitelist, "urlWhitelist") then return end
if list_enabled.uaWhitelist ~= false and text_in_list(ngx.var.http_user_agent, access_lists.uaWhitelist, "uaWhitelist") then return end
if list_enabled.blacklist ~= false and in_list(access_lists.blacklist, "blacklist") then
    append_audit({source = "access-list", category = "blacklist", action = "block", host = host,
        client_ip = client_ip, uri = ngx.var.request_uri, method = ngx.req.get_method(), time = ngx.utctime(), status = 403})
    return ngx.exit(ngx.HTTP_FORBIDDEN)
end
if list_enabled.urlBlacklist ~= false and text_in_list(ngx.var.uri, access_lists.urlBlacklist, "urlBlacklist") then
    append_audit({source = "access-list", category = "urlBlacklist", action = "block", host = host,
        client_ip = client_ip, uri = ngx.var.request_uri, method = ngx.req.get_method(), time = ngx.utctime(), status = 403})
    return ngx.exit(ngx.HTTP_FORBIDDEN)
end
if list_enabled.uaBlacklist ~= false and text_in_list(ngx.var.http_user_agent, access_lists.uaBlacklist, "uaBlacklist") then
    append_audit({source = "access-list", category = "uaBlacklist", action = "block", host = host,
        client_ip = client_ip, uri = ngx.var.request_uri, method = ngx.req.get_method(), time = ngx.utctime(), status = 403})
    return ngx.exit(ngx.HTTP_FORBIDDEN)
end

local site_dir = "/www/wwwroot/" .. host .. "/waf/"
local site = decode_site_config(site_dir, {enabled = true, mode = global.mode})
if not site.enabled then return end
local site_rules = decode_file(site_dir .. "rules.json", nil)
if type(site_rules) == "table" and site_rules.rules then site_rules = site_rules.rules end
if type(site_rules) ~= "table" then site_rules = site.rules or {} end
local default_rules = decode_file(root .. "/default-rules.json", {})
if type(default_rules) == "table" and default_rules.rules then default_rules = default_rules.rules end
if type(default_rules) ~= "table" then default_rules = {} end
local global_rules = decode_file(root .. "/custom-rules.json", {})
if type(global_rules) == "table" and global_rules.rules then global_rules = global_rules.rules end
if type(global_rules) ~= "table" then global_rules = {} end

local effective_mode = site.mode or global.mode or "observe"
if global.mode == "block" then effective_mode = "block" end

local function banned(kind, suffix)
    local key = "ban:" .. kind .. ":" .. client_ip .. ":" .. (suffix or "*")
    if local_cache_get(key) then return true end
    local exists, err = redis_exists(key)
    if exists == true then return true end
    if err and err ~= "disabled" then redis_log_error("ban lookup failed: " .. tostring(err)) end
    return false
end

local function rate_config(name)
    if site.frequencyEnabled == false then return nil end
    local limits = site.rateLimits or site.frequencyLimit or global.rateLimits or global.frequencyLimit or global.frequency or {}
    local value = limits[name]
    if value == nil and name == "access" then value = limits.accessFrequency end
    if value == nil and name == "attack" then value = limits.attackFrequency end
    if value == nil and name == "notFound" then value = limits.notFoundFrequency end
    if value == nil and name == "url" then value = limits.urlFrequency end
    if type(value) ~= "table" then return nil end
    local enabled = value.enabled
    local limit = tonumber(value.limit or value.count or value.times)
    local period = tonumber(value.period or value.seconds or value.interval)
    if enabled == false or not limit or limit < 1 or not period or period < 1 then return nil end
    local block_time = tonumber(value.blockTime or value.block_time or value.banTime)
    return {limit = limit, period = period, block_time = block_time}
end

local function rate_hit(kind, suffix)
    local config = rate_config(kind)
    if not config then return false end
    local key = "rate:" .. kind .. ":" .. client_ip .. ":" .. (suffix or "*")
    local backend = "redis"
    local count, err = redis_increment(key, config.period)
    if not count then
        if err and err ~= "disabled" then redis_log_error("rate counter failed: " .. tostring(err)) end
        backend = "shared-dict"
        count, err = local_cache_incr(key, config.period)
    end
    if not count then
        ngx.log(ngx.ERR, "workmesh_waf rate counter failed: ", err or "unknown")
        return false
    end
    if count <= config.limit then return false end
    append_audit({source = "rate-limit", category = kind, action = "block", host = host,
        client_ip = client_ip, uri = ngx.var.request_uri, method = ngx.req.get_method(), time = ngx.utctime(),
        count = count, limit = config.limit, period = config.period, rate_backend = backend, status = 429})
    if effective_mode == "block" then
        local ban_ttl = (config.block_time and config.block_time > 0) and config.block_time * 60 or config.period
        local ban_key = "ban:" .. kind .. ":" .. client_ip .. ":" .. (suffix or "*")
        if cache then cache:set(ban_key, true, ban_ttl) end
        if redis_enabled then
            local ok, ban_err = redis_set_expiry(ban_key, ban_ttl)
            if not ok and ban_err then redis_log_error("ban write failed: " .. tostring(ban_err)) end
        end
    end
    return true
end

if banned("access") or banned("url", ngx.var.uri or "/") or banned("notFound") or banned("attack") then
    return ngx.exit(ngx.HTTP_TOO_MANY_REQUESTS)
end

if rate_hit("access") or rate_hit("url", ngx.var.uri or "/") then
    if effective_mode == "block" then return ngx.exit(ngx.HTTP_TOO_MANY_REQUESTS) end
end

local function header_value(key)
    local headers = ngx.req.get_headers(100)
    for name, value in pairs(headers) do
        if string.lower(name) == string.lower(key or "") then return tostring(value) end
    end
    return ""
end

local body = ""
local body_limit = tonumber(site.requestBodyLimit or global.requestBodyLimit) or 1048576
local length = tonumber(ngx.var.http_content_length or "0") or 0
if length <= body_limit then
    ngx.req.read_body()
    body = ngx.req.get_body_data() or ""
end

local function value_for(rule)
    if rule.location == "ip" then return client_ip end
    if rule.location == "ipGroup" or rule.location == "ip-group" then return client_ip end
    if rule.location == "uri" then return ngx.var.uri or "" end
    if rule.location == "args" then return ngx.var.args or "" end
    if rule.location == "method" then return ngx.req.get_method() or "" end
    if rule.location == "body" then return body end
    if rule.location == "header" then return header_value(rule.key or rule.name) end
    if rule.location == "cookie" then return header_value("Cookie") end
    return ""
end

local function referenced_ip_group(rule)
    if list_enabled.ipGroups == false then return nil end
    local location = tostring(rule.location or "")
    local key = string.lower(trim(rule.key))
    local expected = trim(rule.value)
    if location == "ipGroup" or location == "ip-group" or key == "ipgroup" or key == "ip-group" then
        return expected
    end
    if location == "ip" and ip_groups[string.lower(expected)] ~= nil then
        return expected
    end
    local prefix = expected:match("^@group:(.+)$") or expected:match("^group:(.+)$") or expected:match("^ip%-group:(.+)$")
    if prefix then return prefix end
    if location == "ip" and ip_groups[string.lower(expected)] ~= nil then return expected end
    return nil
end

local function matches(rule, value)
    local group = referenced_ip_group(rule)
    if group then return ip_group_match(group) end
    value = tostring(value or "")
    local expected = tostring(rule.value or "")
    if rule.operator == "equals" then return value == expected end
    if rule.operator == "contains" then return string.find(string.lower(value), string.lower(expected), 1, true) ~= nil end
    if rule.operator == "regex" then return ngx.re.find(value, expected, "ijo") ~= nil end
    if rule.operator == "ip-cidr" then return cidr_match(value, expected) end
    return false
end

local function apply_rules(rules, source)
    for _, rule in ipairs(rules or {}) do
        if rule.enabled ~= false and matches(rule, value_for(rule)) then
            local action = rule.action or "block"
            append_audit({source = source, category = "rule", rule = rule.id, action = action, host = host,
                client_ip = client_ip, uri = ngx.var.request_uri, method = ngx.req.get_method(), time = ngx.utctime(),
                status = action == "block" and 403 or 200})
            if action == "allow" then return true end
            if action == "block" then
                if rate_hit("attack") then
                    if effective_mode == "block" then return ngx.exit(ngx.HTTP_TOO_MANY_REQUESTS) end
                elseif effective_mode == "block" then
                    return ngx.exit(ngx.HTTP_FORBIDDEN)
                end
            end
        end
    end
    return false
end

local function inspect_standard_rules()
    if global.standardRules == false then return false end
    local sample = table.concat({
        ngx.var.uri or "",
        ngx.var.args or "",
        ngx.req.get_method() or "",
        body,
        header_value("User-Agent"),
        header_value("Cookie"),
    }, "\n")
    local patterns = {
        {id = "CRS-942100", pattern = [[(?i)\bunion\s+select\b|\b(or|and)\s+['"]?1['"]?\s*=\s*['"]?1]]},
        {id = "CRS-941100", pattern = [[(?i)<script\b|javascript:]]},
        {id = "CRS-930110", pattern = [[\.\./|/etc/passwd|boot\.ini]]},
    }
    local detection_level = tonumber(site.detectionLevel or global.paranoiaLevel or 1) or 1
    local threshold = tonumber(global.inboundThreshold or 5) or 5
    local score = 0
    for _, item in ipairs(patterns) do
        if ngx.re.find(sample, item.pattern, "ijo") then
            score = score + 5
            append_audit({source = "standard-rules", category = "crs", rule = item.id,
                action = "block", host = host, client_ip = client_ip, uri = ngx.var.request_uri,
                method = ngx.req.get_method(), time = ngx.utctime(), score = score, status = 403})
        end
    end
    if detection_level >= 2 and string.find(string.lower(sample), "sqlmap", 1, true) then
        score = score + 5
        append_audit({source = "standard-rules", category = "scanner", rule = "CRS-913100",
            action = "block", host = host, client_ip = client_ip, uri = ngx.var.request_uri,
            method = ngx.req.get_method(), time = ngx.utctime(), score = score, status = 403})
    end
    if global.strictMode then
        local strict = {
            {id = "CRS-932100", category = "command-injection", pattern = [[(?i)(;|\||`|\$\().*(cat|curl|wget|chmod|id|whoami)]]},
            {id = "CRS-911100", category = "scanner", pattern = [[(?i)\b(nikto|nmap|masscan)\b]]},
        }
        for _, item in ipairs(strict) do
            if ngx.re.find(sample, item.pattern, "ijo") then
                score = score + 5
                append_audit({source = "strict-mode", category = item.category, rule = item.id,
                    action = "block", host = host, client_ip = client_ip, uri = ngx.var.request_uri,
                    method = ngx.req.get_method(), time = ngx.utctime(), score = score, status = 403})
            end
        end
    end
    if score < threshold then return false end
    if effective_mode == "block" then return ngx.exit(ngx.HTTP_FORBIDDEN) end
    return true
end

if inspect_standard_rules() then return end
if apply_rules(default_rules, "global-default") then return end
if apply_rules(global_rules, "global-custom") then return end
if apply_rules(site_rules, "site-custom") then return end
