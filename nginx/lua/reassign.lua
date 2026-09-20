local core = require("resolve_core")

local function mark_reassigned()
    ngx.var.session_reassigned = "true"
    if ngx.req.get_method() ~= "POST" or ngx.var.uri ~= "/api/shell" then
        ngx.var.resolve_expire_shell_cookie = "shell_id=; Max-Age=0; Path=/; Secure; HttpOnly; SameSite=Strict"
    end
end

local res = ngx.location.capture("/_resolve_reassigned")
local invalid = core.validate_resolve_response(res)
if invalid then
    if invalid.log then
        ngx.log(ngx.ERR, invalid.log)
    end
    return ngx.exit(invalid.exit)
end

local action = core.decide(res, core.stacks(), core.internal_domain())

if action.log then
    ngx.log(ngx.ERR, action.log)
end
if action.exit then
    return ngx.exit(action.exit)
end

if action.forward_host then
    mark_reassigned()
    ngx.var.forward_host = action.forward_host
    ngx.var.fwd_fallback_stack = action.fallback_stack
    ngx.var.fwd_fallback_remaining = action.fallback_remaining
    ngx.log(
        ngx.WARN,
        "cross_stack_forward reason=no_idle",
        " fallback_stack=", tostring(action.fallback_stack),
        " fallback_remaining=", tostring(action.fallback_remaining or ""),
        " target_host=", tostring(action.forward_host),
        " uri=", tostring(ngx.var.request_uri)
    )
    return ngx.exec("@forward_reassigned_fallback")
end

if action.set_cookie then
    ngx.var.resolve_set_cookie = action.set_cookie
else
    ngx.log(ngx.ERR, "reassign: broker did not return session cookie")
    return ngx.exit(500)
end
if action.session_hex then
    ngx.var.session_hex = action.session_hex
end
if action.stack_name then
    ngx.var.stack_name = action.stack_name
end
ngx.var.runner_url = action.runner_url
mark_reassigned()

ngx.log(
    ngx.WARN,
    "session_reassigned reason=owner_unavailable",
    " stack=", tostring(core.own_stack()),
    " uri=", tostring(ngx.var.request_uri)
)
