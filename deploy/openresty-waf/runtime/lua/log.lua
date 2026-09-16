local cjson = require("cjson.safe")
local cache = ngx.shared.workmesh_waf
local root = "/opt/workmesh/waf/data"

local function decode_file(name, fallback)
    local handle = io.open(name, "r")
    if not handle then return fallback end
    local value = cjson.decode(handle:read("*a"))
    handle:close()
    return value or fallback
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

local function trim(value)
    return tostring(value or ""):match("^%s*(.-)%s*$")
end

local function decode_site_config(site_dir, fallback)
    -- Prefer the current Server config name while keeping existing site.json
    -- deployments readable until they are rewritten by the control plane.
    local site = decode_file(site_dir .. "config.json", nil)
    if type(site) == "table" then return site end
    site = decode_file(site_dir .. "site.json", nil)
    if type(site) == "table" then return site end
    return fallback
end

local function redis_log_error(message)
    local should_log = true
    if cache and cache.add then
        local ok = cache:add("redis-error:log", true, 60)
        should_log = ok == true
    end
    if should_log then
        ngx.log(ngx.ERR, "workmesh_waf Redis unavailable (local fallback): ", tostring(message or "unknown error"))
    end
end

local function redis_response(sock)
    local line, err = sock:receive("*l")
    if not line then return nil, err or "empty response" end
    local prefix, payload = line:sub(1, 1), line:sub(2)
    if prefix == "+" then return payload end
    if prefix == "-" then return nil, payload end
    if prefix == ":" then return tonumber(payload) end
    if prefix == "$" then
        local length = tonumber(payload)
        if not length then return nil, "invalid bulk response" end
        if length < 0 then return nil end
        local value, read_err = sock:receive(length)
        if not value then return nil, read_err or "bulk response read failed" end
        local _, crlf_err = sock:receive(2)
        if crlf_err then return nil, crlf_err end
        return value
    end
    return nil, "unsupported Redis response"
end

local function redis_command(config, parts)
    if type(config) ~= "table" or config.enabled ~= true then return nil, "disabled" end
    local host, port, db = trim(config.host), tonumber(config.port) or 6379, tonumber(config.db) or 0
    if host == "" or port < 1 or port > 65535 or db < 0 then
        redis_log_error("invalid Redis configuration")
        return nil, "invalid configuration"
    end
    local socket = ngx.socket.tcp()
    socket:settimeout(100)
    local connected, connect_err = socket:connect(host, port)
    if not connected then
        redis_log_error("connect " .. host .. ":" .. tostring(port) .. ": " .. tostring(connect_err))
        return nil, connect_err
    end
    local commands = {}
    local password = tostring(config.password or "")
    if password ~= "" then commands[#commands + 1] = {"AUTH", password} end
    if db ~= 0 then commands[#commands + 1] = {"SELECT", tostring(db)} end
    commands[#commands + 1] = parts
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

local function local_cache_incr(key, period)
    if not cache then return nil, "shared dict unavailable" end
    local count, err = cache:incr(key, 1, 0, period)
    if not count then return nil, err or "shared dict incr failed" end
    return count
end

local host = ngx.var.host or "_"
if not host:match("^[%w%._%-]+$") then host = "_" end
local global = decode_file(root .. "/global.json", {enabled = true})
local site = decode_site_config("/www/wwwroot/" .. host .. "/waf/", {enabled = true})
if not global.enabled or not site.enabled or ngx.status ~= 404 then return end
if site.frequencyEnabled == false then return end
local mode = site.mode or global.mode or "observe"
if global.mode == "block" then mode = "block" end
local limits = site.rateLimits or site.frequencyLimit or global.rateLimits or global.frequencyLimit or global.frequency or {}
local cfg = limits.notFound or limits.notFoundFrequency
if type(cfg) ~= "table" or cfg.enabled == false then return end
local limit = tonumber(cfg.limit or cfg.count or cfg.times)
local period = tonumber(cfg.period or cfg.seconds or cfg.interval)
if not limit or limit < 1 or not period or period < 1 then return end
local block_time = tonumber(cfg.blockTime or cfg.block_time or cfg.banTime)
local key = "rate:notFound:" .. (ngx.var.remote_addr or "")
local redis_script = "local v=redis.call('INCR',KEYS[1]);if v==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]);end;return v"
local backend = "redis"
local count, redis_err = redis_command(global.redis, {"EVAL", redis_script, "1", key, tostring(period)})
count = tonumber(count)
if not count then
    if redis_err and redis_err ~= "disabled" then redis_log_error("404 rate counter failed: " .. tostring(redis_err)) end
    backend = "shared-dict"
    count = local_cache_incr(key, period)
end
if count and count > limit then
    append_audit({source = "rate-limit", category = "notFound", action = "block", host = host,
        client_ip = ngx.var.remote_addr, uri = ngx.var.request_uri, method = ngx.req.get_method(),
        time = ngx.utctime(), count = count, limit = limit, period = period, rate_backend = backend, status = 404})
    if mode == "block" then
        local ban_key = "ban:notFound:" .. (ngx.var.remote_addr or "") .. ":*"
        local ban_ttl = (block_time and block_time > 0) and block_time * 60 or period
        if cache then cache:set(ban_key, true, ban_ttl) end
        if type(global.redis) == "table" and global.redis.enabled == true then
            local ok, ban_err = redis_command(global.redis, {"SET", ban_key, "1", "EX", tostring(ban_ttl)})
            if ok == nil and ban_err then redis_log_error("404 ban write failed: " .. tostring(ban_err)) end
        end
    end
end
