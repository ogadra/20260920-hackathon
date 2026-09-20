-- /resolve/sessionサブリクエスト応答 (res) から、クライアントへの振る舞いを決める純関数。
-- brokerはhost-only契約 (X-Runner-Host) を返し、nginxが用途別portを貼る。
local runner_host = require("runner_host")

local _M = {}

local HTTP_OK = 200
local HTTP_NOT_FOUND = 404
local HTTP_INTERNAL_ERROR = 500
local HTTP_SERVICE_UNAVAILABLE = 503

local own_stack_name = ""
local internal_domain_name = ""
local allowed_stacks = {}
local ordered_stacks = {}
local api_port_number = 0
local app_port_number = 0
local cloud_name = ""

local ALLOWED_CLOUDS = { AWS = true, GOOGLE_CLOUD = true }

local function string_header(headers, name, label)
    local value = headers[name]
    if value ~= nil and type(value) ~= "string" then
        return nil, "resolve: invalid " .. label .. " header type: " .. type(value)
    end
    return value, nil
end

local function validate_port(v, name)
    local port = tonumber(v)
    if port == nil or port <= 0 or port > 65535 or port ~= math.floor(port) then
        error("resolve_core: " .. name .. " must be a valid TCP port")
    end
    return port
end

function _M.configure(stack, domain, stack_names, api_port, app_port, cloud)
    if stack == nil or stack == "" or domain == nil or domain == "" then
        error("resolve_core: STACK_NAME and INTERNAL_DOMAIN must be set")
    end
    if stack_names == nil or stack_names == "" then
        error("resolve_core: BUNSHIN_STACKS must be set")
    end
    if cloud == nil or not ALLOWED_CLOUDS[cloud] then
        error("resolve_core: CLOUD must be one of AWS|GOOGLE_CLOUD")
    end
    api_port_number = validate_port(api_port, "RUNNER_API_PORT")
    app_port_number = validate_port(app_port, "RUNNER_APP_PORT")
    local set = {}
    local list = {}
    for s in stack_names:gmatch("[^,]+") do
        set[s] = true
        table.insert(list, s)
    end
    if next(set) == nil then
        error("resolve_core: BUNSHIN_STACKS must be set")
    end
    if not set[stack] then
        error("resolve_core: STACK_NAME must be included in BUNSHIN_STACKS")
    end
    own_stack_name = stack
    internal_domain_name = domain
    allowed_stacks = set
    ordered_stacks = list
    cloud_name = cloud
end

function _M.own_stack()
    return own_stack_name
end

function _M.internal_domain()
    return internal_domain_name
end

function _M.stacks()
    return allowed_stacks
end

function _M.fallback_remaining_excluding(stack)
    local remaining = {}
    for _, candidate in ipairs(ordered_stacks) do
        if candidate ~= own_stack_name and candidate ~= stack then
            table.insert(remaining, candidate)
        end
    end
    if #remaining == 0 then
        return nil
    end
    return table.concat(remaining, ",")
end

function _M.cookie_stack(session_id)
    if type(session_id) ~= "string" then
        return nil
    end
    return session_id:match("^([^_]+)_")
end

-- 既知 stack のみ許可し、未知/詐称値が proxy_pass の host へ流れるのを防ぐ。
function _M.host_of(stack, stacks, domain)
    if type(stacks) ~= "table" or not stacks[stack] then
        return nil
    end
    return stack .. "." .. domain
end

-- 内部ALB経由 (= Hostが<stack>.<internal_domain>の完全一致) の要求か判定する。
-- 完全一致にしないと、公開経路からregexにマッチするHostを作られX-Fallback-*を詐称できる。
function _M.is_internal_host(host)
    if type(host) ~= "string" or internal_domain_name == "" then
        return false
    end
    local dot = host:find(".", 1, true)
    if not dot then
        return false
    end
    return allowed_stacks[host:sub(1, dot - 1)] == true
        and host:sub(dot + 1) == internal_domain_name
end

function _M.relay_if_internal(from_internal, value)
    if from_internal and value ~= nil then
        return value
    end
    return ""
end

-- ALB は client が送った X-Forwarded-For の後ろに自分が観測した IP:port を足す。
-- 先頭側は client が詐称できるので、ALB が書いた末尾だけを採用する。
function _M.last_forwarded_for(value)
    if value == nil then
        return nil
    end
    local last = value:match("([^,%s]+)%s*$")
    if last == nil or last == "" then
        return nil
    end
    return last
end

function _M.client_address(from_internal, bunshin_header, forwarded_for, edge_header, remote_addr, remote_port)
    if from_internal and bunshin_header ~= nil and bunshin_header ~= "" then
        return bunshin_header
    end
    -- forwarded_for は AWS ALB / edge_header は GCP LB でしか上書き保証がない。
    -- 経路と異なる方は client 詐称可能なので必ず無視する。
    if cloud_name == "AWS" then
        local forwarded = _M.last_forwarded_for(forwarded_for)
        if forwarded ~= nil then
            return forwarded
        end
    end
    if cloud_name == "GOOGLE_CLOUD" and edge_header ~= nil and edge_header ~= "" then
        return edge_header
    end
    return tostring(remote_addr or "") .. ":" .. tostring(remote_port or "")
end

function _M.decide_arrival(session_id, stack, stacks, domain, fallback_stack)
    -- fallback 転送は session 所属未確定の到着なので、この stack の broker で解決を続ける。
    if fallback_stack ~= nil and fallback_stack ~= "" then
        if fallback_stack ~= stack then
            return { exit = HTTP_INTERNAL_ERROR, log = "resolve: fallback stack does not match own stack: " .. tostring(fallback_stack) }
        end
        return nil
    end
    -- session_id prefix は所属 stack 確定済みの印なので、自 stack 以外なら所属先へ到着させる。
    local owner = _M.cookie_stack(session_id)
    if owner == nil or owner == stack then
        return nil
    end
    local host = _M.host_of(owner, stacks, domain)
    if host == nil then
        return { exit = HTTP_INTERNAL_ERROR, log = "resolve: unknown stack in session_id: " .. tostring(owner) }
    end
    return { forward_host = host, owner_stack = owner }
end

function _M.validate_resolve_response(res)
    if res.status ~= HTTP_SERVICE_UNAVAILABLE then
        return nil
    end
    local _, err = string_header(res.header, "X-Fallback-Stack", "fallback stack")
    if err ~= nil then
        return { exit = HTTP_SERVICE_UNAVAILABLE, log = err }
    end
    local _, remaining_err = string_header(res.header, "X-Fallback-Remaining", "fallback remaining")
    if remaining_err ~= nil then
        return { exit = HTTP_SERVICE_UNAVAILABLE, log = remaining_err }
    end
    return nil
end

function _M.decide(res, stacks, domain)
    if res.status == HTTP_SERVICE_UNAVAILABLE then
        local next_stack = res.header["X-Fallback-Stack"]
        if next_stack ~= nil and next_stack ~= "" then
            local remaining = res.header["X-Fallback-Remaining"]
            local host = _M.host_of(next_stack, stacks, domain)
            if host == nil then
                return { exit = HTTP_SERVICE_UNAVAILABLE, log = "resolve: invalid fallback stack: " .. tostring(next_stack) }
            end
            return {
                forward_host       = host,
                fallback_stack     = next_stack,
                fallback_remaining = remaining,
            }
        end
        return { exit = HTTP_SERVICE_UNAVAILABLE }
    end

    if res.status ~= HTTP_OK then
        return { exit = res.status }
    end

    local host = res.header["X-Runner-Host"]
    if not runner_host.is_valid(host) then
        return {
            exit = HTTP_INTERNAL_ERROR,
            log = "resolve: invalid X-Runner-Host from broker: " .. tostring(host),
        }
    end

    -- ngx.location.capture は複数 Set-Cookie をテーブルで返すため文字列へ畳む。
    local set_cookie = res.header["Set-Cookie"]
    if type(set_cookie) == "table" then
        set_cookie = table.concat(set_cookie, ", ")
    end
    return {
        runner_url = "http://" .. host .. ":" .. tostring(api_port_number),
        set_cookie = set_cookie,
        reassigned = res.header["X-Session-Reassigned"],
        session_hex = res.header["X-Session-Hex"],
        stack_name = res.header["X-Stack-Name"],
    }
end

-- port-forwardのHost `<hex32>.<stack>.<internal_domain>` を分解する。
-- suffixがinternal_domainと完全一致しなければnil。
-- stackはBUNSHIN_STACKSのallowlistで絞る。
-- server_name段階のregexを通過しても未知stackはここで落とす。
function _M.parse_app_host(host)
    if type(host) ~= "string" or internal_domain_name == "" then
        return nil
    end
    local hex, stack, suffix = host:match("^([0-9a-f]+)%.([a-z0-9-]+)%.(.+)$")
    if hex == nil or #hex ~= 32 then
        return nil
    end
    if suffix ~= internal_domain_name then
        return nil
    end
    if not allowed_stacks[stack] then
        return nil
    end
    return { hex = hex, stack = stack }
end

-- 所有stackへはDNSで直接着弾する前提。
-- 他stackのHostは解決せず404に落とす。
-- cross-stack forwardを許すとfallback / relay経路と混ざる。
function _M.decide_app_arrival(host)
    local parsed = _M.parse_app_host(host)
    if parsed == nil then
        return { exit = HTTP_NOT_FOUND }
    end
    if parsed.stack ~= own_stack_name then
        return { exit = HTTP_NOT_FOUND }
    end
    return { hex = parsed.hex }
end

-- 200以外はそのまま透過する。broker 404はsession不在で、decide_app_arrivalの404と同形なので推測されない。
-- 200 + 不正hostはbroker側のバグ。
-- 500 + logで観測できるようにする。
function _M.decide_app_resolve(status, headers)
    if status ~= HTTP_OK then
        return { exit = status }
    end
    local host = headers["X-Runner-Host"]
    if not runner_host.is_valid(host) then
        return {
            exit = HTTP_INTERNAL_ERROR,
            log = "pf_resolve: invalid X-Runner-Host from broker: " .. tostring(host),
        }
    end
    return { upstream = "http://" .. host .. ":" .. tostring(app_port_number) }
end

return _M
