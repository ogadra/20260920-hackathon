package.path = "/usr/local/openresty/nginx/lua/?.lua;" .. package.path
local core = require("resolve_core")

core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")

local failed = 0
local function check(name, cond)
    if not cond then
        failed = failed + 1
        io.stderr:write("FAIL " .. name .. "\n")
    end
end

local function run(res, method, uri)
    local executed = nil
    _G.ngx = {
        ERR = "ERR",
        WARN = "WARN",
        var = {
            request_uri = uri,
            uri = uri,
        },
        req = {
            get_method = function()
                return method
            end,
        },
        location = {
            capture = function(path)
                check("captures reassigned resolve", path == "/_resolve_reassigned")
                return res
            end,
        },
        exec = function(target)
            executed = target
            return target
        end,
        exit = function(status)
            return "exit:" .. tostring(status)
        end,
        log = function() end,
    }

    local returned = dofile("/usr/local/openresty/nginx/lua/reassign.lua")
    return returned, executed, _G.ngx.var
end

local _, executed, vars = run({
    status = 503,
    header = {
        ["X-Fallback-Stack"] = "ap-northeast-3",
    },
}, "POST", "/api/execute")
check("fallback executes reassigned location", executed == "@forward_reassigned_fallback")
check("fallback keeps reassigned signal", vars.session_reassigned == "true")
check("fallback clears stale shell cookie", vars.resolve_expire_shell_cookie ~= nil)
check("fallback forwards stack", vars.forward_host == "ap-northeast-3.example.com")
check("fallback forwards fallback stack", vars.fwd_fallback_stack == "ap-northeast-3")

_, executed, vars = run({
    status = 503,
    header = {
        ["X-Fallback-Stack"] = "ap-northeast-3",
    },
}, "POST", "/api/shell")
check("shell fallback executes reassigned location", executed == "@forward_reassigned_fallback")
check("shell fallback keeps reassigned signal", vars.session_reassigned == "true")
check("shell fallback does not expire new shell cookie", vars.resolve_expire_shell_cookie == nil)

_, executed, vars = run({
    status = 200,
    header = {
        ["X-Runner-Host"] = "runner-1",
        ["Set-Cookie"] = "session_id=ap-northeast-1_deadbeef; Path=/",
    },
}, "POST", "/api/execute")
check("local reassign does not forward", executed == nil)
check("local reassign sets runner", vars.runner_url == "http://runner-1:3000")
check("local reassign keeps reassigned signal", vars.session_reassigned == "true")
check("local reassign propagates session cookie", vars.resolve_set_cookie == "session_id=ap-northeast-1_deadbeef; Path=/")
check("local reassign leaves session hex unset when absent", vars.session_hex == nil)
check("local reassign leaves stack name unset when absent", vars.stack_name == nil)

_, _, vars = run({
    status = 200,
    header = {
        ["X-Runner-Host"] = "runner-1",
        ["Set-Cookie"] = "session_id=ap-northeast-1_deadbeef; Path=/",
        ["X-Session-Hex"] = "0123456789abcdef0123456789abcdef",
        ["X-Stack-Name"] = "ap-northeast-1",
    },
}, "POST", "/api/execute")
check("local reassign propagates session hex", vars.session_hex == "0123456789abcdef0123456789abcdef")
check("local reassign propagates stack name", vars.stack_name == "ap-northeast-1")

if failed > 0 then
    io.stderr:write(string.format("reassign: %d check(s) failed\n", failed))
    os.exit(1)
end
print("reassign: all checks passed")
