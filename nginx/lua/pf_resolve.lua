-- port-forwardの{hex32}.{stack}.<internal_domain> Hostをrunnerの:app_portへ振り分ける。
-- 所有stackでなければ404。
-- brokerがsessionを持たなければ404。
-- 成功したら$pf_upstreamを組み立ててproxy_passに渡す。
local core = require("resolve_core")

local arrival = core.decide_app_arrival(ngx.var.host)
if arrival.exit then
    return ngx.exit(arrival.exit)
end

local res = ngx.location.capture("/_pf_resolve")

local decided = core.decide_app_resolve(res.status, res.header)
if decided.log then
    ngx.log(ngx.ERR, decided.log)
end
if decided.exit then
    return ngx.exit(decided.exit)
end

ngx.var.pf_upstream = decided.upstream
