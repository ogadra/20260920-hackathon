-- resolve_coreの分岐テスト。brokerはhost-only契約 (X-Runner-Host) を返す前提。
package.path = "/usr/local/openresty/nginx/lua/?.lua;" .. package.path
local core = require("resolve_core")

local STACKS = { ["ap-northeast-1"] = true, ["ap-northeast-3"] = true }

local failed = 0
local function check(name, cond)
    if not cond then
        failed = failed + 1
        io.stderr:write("FAIL " .. name .. "\n")
    end
end

-- 事前configure。RUNNER_API_PORT=3000 / RUNNER_APP_PORT=5000でdecideの組み立てを検証する。
core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")

-- broker 非 2xx はそのステータスを保持して終了 (503/500 透過)
local r = core.decide({ status = 503, header = {} }, STACKS, "example.com")
check("503 transparency exit", r.exit == 503)
check("503 transparency no runner", r.runner_url == nil)

r = core.decide({ status = 500, header = {} }, STACKS, "example.com")
check("500 transparency", r.exit == 500)

-- 200 + 不正X-Runner-Hostは500で遮断
for _, bad in ipairs({ "runner:3000", "http://runner", "runner/", "runner_01", "user@runner", "" }) do
    r = core.decide({ status = 200, header = { ["X-Runner-Host"] = bad } }, STACKS, "example.com")
    check("ssrf guard exit " .. bad, r.exit == 500)
    check("ssrf guard log " .. bad, r.log ~= nil)
end

-- 200 + X-Runner-Hostヘッダ欠落 (nil) も500
r = core.decide({ status = 200, header = {} }, STACKS, "example.com")
check("missing X-Runner-Host", r.exit == 500)

-- 正常: brokerのX-Runner-HostにRUNNER_API_PORTを貼ってrunner_urlを組む。
r = core.decide({ status = 200, header = { ["X-Runner-Host"] = "runner-1" } }, STACKS, "example.com")
check("valid proxy no exit", r.exit == nil)
check("valid proxy runner", r.runner_url == "http://runner-1:3000")
check("valid proxy no forward host", r.forward_host == nil)
check("no set-cookie when absent", r.set_cookie == nil)
check("no reassigned when absent", r.reassigned == nil)
check("no session-hex when absent", r.session_hex == nil)
check("no stack-name when absent", r.stack_name == nil)

-- Set-Cookie / X-Session-Reassigned / X-Session-Hex / X-Stack-Nameの伝播
r = core.decide({ status = 200, header = {
    ["X-Runner-Host"] = "runner-1",
    ["Set-Cookie"] = "session_id=abc; Path=/",
    ["X-Session-Reassigned"] = "true",
    ["X-Session-Hex"] = "0123456789abcdef0123456789abcdef",
    ["X-Stack-Name"] = "ap-northeast-1",
} }, STACKS, "example.com")
check("propagates set-cookie", r.set_cookie == "session_id=abc; Path=/")
check("propagates reassigned", r.reassigned == "true")
check("propagates session-hex", r.session_hex == "0123456789abcdef0123456789abcdef")
check("propagates stack-name", r.stack_name == "ap-northeast-1")

-- 複数 Set-Cookie (capture はテーブルで返す) を文字列へ畳む
r = core.decide({ status = 200, header = {
    ["X-Runner-Host"] = "runner-1",
    ["Set-Cookie"] = { "a=1", "b=2" },
} }, STACKS, "example.com")
check("joins multiple set-cookie", r.set_cookie == "a=1, b=2")

-- cookie_stack は session_id "<stack>_<hex>" の prefix を返す
check("cookie_stack extracts prefix", core.cookie_stack("ap-northeast-1_deadbeef") == "ap-northeast-1")
check("cookie_stack nil for non-string", core.cookie_stack(nil) == nil)
check("cookie_stack nil without underscore", core.cookie_stack("nounderscore") == nil)
check("cookie_stack nil for empty prefix", core.cookie_stack("_abc") == nil)

-- host_of は allowlist 内の stack のみ <stack>.<domain> を組み立てる
check("host_of builds host", core.host_of("ap-northeast-3", STACKS, "example.com") == "ap-northeast-3.example.com")
check("host_of rejects unknown stack", core.host_of("ap-southeast-9", STACKS, "example.com") == nil)
check("host_of rejects injection value", core.host_of("evil.example.com/", STACKS, "example.com") == nil)
check("host_of rejects nil stack", core.host_of(nil, STACKS, "example.com") == nil)

-- configureはSTACK_NAME / INTERNAL_DOMAIN / BUNSHIN_STACKS / RUNNER_API_PORT / RUNNER_APP_PORT / CLOUD未設定を許さず起動を失敗させる
check("configure rejects missing stack", not pcall(core.configure, nil, "example.com", "ap-northeast-1", 3000, 5000, "AWS"))
check("configure rejects empty stack", not pcall(core.configure, "", "example.com", "ap-northeast-1", 3000, 5000, "AWS"))
check("configure rejects missing domain", not pcall(core.configure, "ap-northeast-1", nil, "ap-northeast-1", 3000, 5000, "AWS"))
check("configure rejects missing stacks", not pcall(core.configure, "ap-northeast-1", "example.com", nil, 3000, 5000, "AWS"))
check("configure rejects empty stacks", not pcall(core.configure, "ap-northeast-1", "example.com", "", 3000, 5000, "AWS"))
check("configure rejects own stack outside allowlist", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-3", 3000, 5000, "AWS"))
check("configure rejects missing api_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", nil, 5000, "AWS"))
check("configure rejects non-numeric api_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", "abc", 5000, "AWS"))
check("configure rejects out-of-range api_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 70000, 5000, "AWS"))
check("configure rejects zero api_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 0, 5000, "AWS"))
check("configure rejects fractional api_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000.5, 5000, "AWS"))
check("configure rejects missing app_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, nil, "AWS"))
check("configure rejects non-numeric app_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, "abc", "AWS"))
check("configure rejects out-of-range app_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 70000, "AWS"))
check("configure rejects zero app_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 0, "AWS"))
check("configure rejects fractional app_port", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 5000.5, "AWS"))
check("configure rejects missing cloud", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 5000, nil))
check("configure rejects empty cloud", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 5000, ""))
check("configure rejects unknown cloud", not pcall(core.configure, "ap-northeast-1", "example.com", "ap-northeast-1", 3000, 5000, "azure"))

core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-2,ap-northeast-3", 3000, 5000, "AWS")
check("fallback excludes attempted owner", core.fallback_remaining_excluding("ap-northeast-2") == "ap-northeast-3")
core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-3,ap-northeast-2,ap-northeast-4", 3000, 5000, "AWS")
check("fallback keeps configured order", core.fallback_remaining_excluding("ap-northeast-2") == "ap-northeast-3,ap-northeast-4")
core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")
check("fallback returns nil when no candidate remains", core.fallback_remaining_excluding("ap-northeast-3") == nil)

-- decide_arrival: cookie 無 / 自stack宛 はローカル解決
check("arrival nil without cookie", core.decide_arrival(nil, "ap-northeast-1", STACKS, "example.com") == nil)
check("arrival nil for own stack", core.decide_arrival("ap-northeast-1_x", "ap-northeast-1", STACKS, "example.com") == nil)

-- decide_arrival: 別stack宛は所属stackの内部ALBへ転送
r = core.decide_arrival("ap-northeast-3_deadbeef", "ap-northeast-1", STACKS, "example.com")
check("arrival forwards foreign stack host", r.forward_host == "ap-northeast-3.example.com")
check("arrival forwards foreign stack owner", r.owner_stack == "ap-northeast-3")
check("arrival forwards foreign stack no exit", r.exit == nil)

-- decide_arrival: allowlist 外の prefix は 500 で遮断
r = core.decide_arrival("evilhost_x", "ap-northeast-1", STACKS, "example.com")
check("arrival rejects unknown stack exit", r.exit == 500)
check("arrival rejects unknown stack log", r.log ~= nil)

-- decide_arrival: fallback 転送中は既存 cookie の所属stackへ戻さずローカル broker を呼ぶ
check("arrival skips during own fallback", core.decide_arrival("ap-northeast-1_deadbeef", "ap-northeast-3", STACKS, "example.com", "ap-northeast-3") == nil)
r = core.decide_arrival("ap-northeast-1_deadbeef", "ap-northeast-3", STACKS, "example.com", "ap-northeast-1")
check("arrival rejects mismatched fallback stack exit", r.exit == 500)
check("arrival rejects mismatched fallback stack log", r.log ~= nil)

-- decide: 503 + X-Fallback-Stack は次stackの内部ALBへ転送し残り候補を中継
r = core.decide({ status = 503, header = {
    ["X-Fallback-Stack"]     = "ap-northeast-3",
    ["X-Fallback-Remaining"] = "ap-northeast-2",
} }, STACKS, "example.com")
check("fallback forwards next stack", r.forward_host == "ap-northeast-3.example.com")
check("fallback relays stack", r.fallback_stack == "ap-northeast-3")
check("fallback relays remaining", r.fallback_remaining == "ap-northeast-2")

-- decide: 残り候補が複数ならカンマ区切り文字列のまま中継
r = core.decide({ status = 503, header = {
    ["X-Fallback-Stack"]     = "ap-northeast-3",
    ["X-Fallback-Remaining"] = "ap-northeast-2,us-east-1",
} }, STACKS, "example.com")
check("fallback relays comma-separated remaining", r.fallback_remaining == "ap-northeast-2,us-east-1")

-- decide: 残り候補ヘッダが無ければ下流にも出さない
r = core.decide({ status = 503, header = { ["X-Fallback-Stack"] = "ap-northeast-3" } }, STACKS, "example.com")
check("fallback empty remaining host", r.forward_host == "ap-northeast-3.example.com")
check("fallback empty remaining value", r.fallback_remaining == nil)

-- validate_resolve_response: fallback header が複数値なら decide の前に遮断
r = core.validate_resolve_response({ status = 503, header = { ["X-Fallback-Stack"] = { "ap-northeast-3" } } })
check("fallback validates stack table exit", r.exit == 503)
check("fallback validates stack table log", r.log ~= nil)
r = core.validate_resolve_response({ status = 503, header = {
    ["X-Fallback-Stack"]     = "ap-northeast-3",
    ["X-Fallback-Remaining"] = { "ap-northeast-2" },
} })
check("fallback validates remaining table exit", r.exit == 503)
check("fallback validates remaining table log", r.log ~= nil)

-- decide: 不正な fallback stack は 503 で遮断 (転送しない)
r = core.decide({ status = 503, header = { ["X-Fallback-Stack"] = "EVIL" } }, STACKS, "example.com")
check("fallback rejects invalid stack exit", r.exit == 503)
check("fallback rejects invalid stack log", r.log ~= nil)
check("fallback rejects invalid stack no host", r.forward_host == nil)

-- decide: X-Fallback-Stack 無し/空 の 503 は終端 (転送しない)
r = core.decide({ status = 503, header = {} }, STACKS, "example.com")
check("fallback terminal without header exit", r.exit == 503)
check("fallback terminal without header no host", r.forward_host == nil)
r = core.decide({ status = 503, header = { ["X-Fallback-Stack"] = "" } }, STACKS, "example.com")
check("fallback terminal on empty header exit", r.exit == 503)
check("fallback terminal on empty header no host", r.forward_host == nil)

-- is_internal_host: <stack>.<internal_domain>の完全一致だけを内部ALBと認める
core.configure("ap-northeast-1", "internal.example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")
check("is_internal_host accepts own stack host", core.is_internal_host("ap-northeast-1.internal.example.com"))
check("is_internal_host accepts peer stack host", core.is_internal_host("ap-northeast-3.internal.example.com"))
check("is_internal_host rejects public host", not core.is_internal_host("app.example.com"))
check("is_internal_host rejects regex-matching public host", not core.is_internal_host("aaa111.ap-northeast-1.internal.example.com"))
check("is_internal_host rejects trailing garbage", not core.is_internal_host("ap-northeast-1.internal.example.com.evil"))
check("is_internal_host rejects unknown stack in known domain", not core.is_internal_host("ap-southeast-9.internal.example.com"))
check("is_internal_host rejects nil", not core.is_internal_host(nil))
check("is_internal_host rejects empty", not core.is_internal_host(""))
check("is_internal_host rejects leading dot", not core.is_internal_host(".internal.example.com"))
check("is_internal_host rejects double dot", not core.is_internal_host("ap-northeast-1..internal.example.com"))
check("is_internal_host rejects stack without domain", not core.is_internal_host("ap-northeast-1."))

-- relay_if_internal: 内部ALBのときのみヘッダを通し、公開経路では空文字を返す
check("relay_if_internal returns header when internal", core.relay_if_internal(true, "ap-northeast-3") == "ap-northeast-3")
check("relay_if_internal returns empty when public", core.relay_if_internal(false, "ap-northeast-3") == "")
check("relay_if_internal returns empty when internal but header nil", core.relay_if_internal(true, nil) == "")
check("relay_if_internal returns empty when public and header nil", core.relay_if_internal(false, nil) == "")

-- last_forwarded_for: ALBが末尾に足したIP:portだけを採り、client由来の先頭側は捨てる
check("last_forwarded_for takes the last entry", core.last_forwarded_for("1.1.1.1:1, 9.9.9.9:99") == "9.9.9.9:99")
check("last_forwarded_for takes the only entry", core.last_forwarded_for("9.9.9.9:99") == "9.9.9.9:99")
check("last_forwarded_for ignores trailing space", core.last_forwarded_for("1.1.1.1:1 , 9.9.9.9:99 ") == "9.9.9.9:99")
check("last_forwarded_for returns nil for empty", core.last_forwarded_for("") == nil)
check("last_forwarded_for returns nil for nil", core.last_forwarded_for(nil) == nil)

-- client_address: cloud=AWS は bunshin(内部転送)→X-Forwarded-For末尾→remote_addr:port。GCP LB でしか上書きできない edge_header は無視する
core.configure("ap-northeast-1", "example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")
check("client_address AWS internal picks bunshin header",
    core.client_address(true, "1.2.3.4:5678", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "1.2.3.4:5678")
check("client_address AWS internal falls to forwarded-for when bunshin empty",
    core.client_address(true, "", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "9.9.9.9:1")
check("client_address AWS takes only the ALB appended entry",
    core.client_address(false, "", "spoof:0, 9.9.9.9:1", "", "10.0.0.1", "12345") == "9.9.9.9:1")
check("client_address AWS internal ignores spoofed edge and falls to remote",
    core.client_address(true, "", "", "3.3.3.3:9", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address AWS internal falls to remote when all empty",
    core.client_address(true, "", "", "", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address AWS public ignores bunshin header",
    core.client_address(false, "1.2.3.4:5678", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "9.9.9.9:1")
check("client_address AWS public ignores spoofed edge and falls to remote",
    core.client_address(false, "spoof", "", "3.3.3.3:9", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address AWS public falls to remote when all headers nil",
    core.client_address(false, nil, nil, nil, "10.0.0.1", "12345") == "10.0.0.1:12345")

-- client_address: cloud=GOOGLE_CLOUD は bunshin(内部転送)→edge→remote_addr:port。AWS ALB でしか上書きできない forwarded_for は無視する
core.configure("asia-northeast1", "example.com", "asia-northeast1,asia-northeast2", 3000, 5000, "GOOGLE_CLOUD")
check("client_address GOOGLE_CLOUD internal picks bunshin header",
    core.client_address(true, "1.2.3.4:5678", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "1.2.3.4:5678")
check("client_address GOOGLE_CLOUD internal falls to edge when bunshin empty",
    core.client_address(true, "", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "3.3.3.3:9")
check("client_address GOOGLE_CLOUD internal ignores spoofed forwarded-for and falls to remote",
    core.client_address(true, "", "spoof:0", "", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address GOOGLE_CLOUD internal falls to remote when all empty",
    core.client_address(true, "", "", "", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address GOOGLE_CLOUD public ignores bunshin header",
    core.client_address(false, "1.2.3.4:5678", "9.9.9.9:1", "3.3.3.3:9", "10.0.0.1", "12345") == "3.3.3.3:9")
check("client_address GOOGLE_CLOUD public ignores spoofed forwarded-for and falls to remote",
    core.client_address(false, "spoof", "spoof:0", "", "10.0.0.1", "12345") == "10.0.0.1:12345")
check("client_address GOOGLE_CLOUD public falls to remote when all headers nil",
    core.client_address(false, nil, nil, nil, "10.0.0.1", "12345") == "10.0.0.1:12345")

-- parse_app_host: 32 hex label + 既知stack + internal_domain完全一致だけ通す
core.configure("ap-northeast-1", "internal.example.com", "ap-northeast-1,ap-northeast-3", 3000, 5000, "AWS")
local HEX = string.rep("a", 32)
r = core.parse_app_host(HEX .. ".ap-northeast-1.internal.example.com")
check("parse_app_host own stack hex", r ~= nil and r.hex == HEX and r.stack == "ap-northeast-1")
r = core.parse_app_host(HEX .. ".ap-northeast-3.internal.example.com")
check("parse_app_host peer stack", r ~= nil and r.stack == "ap-northeast-3")
check("parse_app_host rejects 31 hex", core.parse_app_host(string.rep("a", 31) .. ".ap-northeast-1.internal.example.com") == nil)
check("parse_app_host rejects 33 hex", core.parse_app_host(string.rep("a", 33) .. ".ap-northeast-1.internal.example.com") == nil)
check("parse_app_host rejects uppercase hex", core.parse_app_host(string.rep("A", 32) .. ".ap-northeast-1.internal.example.com") == nil)
check("parse_app_host rejects unknown stack", core.parse_app_host(HEX .. ".ap-southeast-9.internal.example.com") == nil)
check("parse_app_host rejects suffix mismatch", core.parse_app_host(HEX .. ".ap-northeast-1.evil.example.com") == nil)
check("parse_app_host rejects extra suffix", core.parse_app_host(HEX .. ".ap-northeast-1.internal.example.com.evil") == nil)
check("parse_app_host rejects nil", core.parse_app_host(nil) == nil)
check("parse_app_host rejects empty", core.parse_app_host("") == nil)

-- decide_app_arrival: 自stackのみhexを返し、他stack / 不正はすべて404
r = core.decide_app_arrival(HEX .. ".ap-northeast-1.internal.example.com")
check("app_arrival own stack returns hex", r.hex == HEX and r.exit == nil)
r = core.decide_app_arrival(HEX .. ".ap-northeast-3.internal.example.com")
check("app_arrival peer stack 404", r.exit == 404)
r = core.decide_app_arrival(HEX .. ".ap-southeast-9.internal.example.com")
check("app_arrival unknown stack 404", r.exit == 404)
r = core.decide_app_arrival("app.example.com")
check("app_arrival non-pf host 404", r.exit == 404)

-- decide_app_resolve: 404はsession不在として隠蔽、他non-200は透過、200 + 不正hostは500 + log
r = core.decide_app_resolve(200, { ["X-Runner-Host"] = "runner-1" })
check("app_resolve builds upstream with app_port", r.upstream == "http://runner-1:5000")
r = core.decide_app_resolve(200, { ["X-Runner-Host"] = "10.0.0.1" })
check("app_resolve accepts ipv4 host", r.upstream == "http://10.0.0.1:5000")
r = core.decide_app_resolve(404, {})
check("app_resolve 404 broker 404", r.exit == 404 and r.log == nil)
r = core.decide_app_resolve(500, {})
check("app_resolve 500 broker passes through", r.exit == 500 and r.log == nil)
r = core.decide_app_resolve(503, {})
check("app_resolve 503 broker passes through", r.exit == 503 and r.log == nil)
r = core.decide_app_resolve(200, {})
check("app_resolve missing runner host 500 + log", r.exit == 500 and r.log ~= nil)
r = core.decide_app_resolve(200, { ["X-Runner-Host"] = "runner/path" })
check("app_resolve invalid host with slash 500 + log", r.exit == 500 and r.log ~= nil)
r = core.decide_app_resolve(200, { ["X-Runner-Host"] = "runner:3000" })
check("app_resolve host with port 500 + log", r.exit == 500 and r.log ~= nil)
r = core.decide_app_resolve(200, { ["X-Runner-Host"] = { "runner-1", "runner-2" } })
check("app_resolve duplicate X-Runner-Host 500 + log", r.exit == 500 and r.log ~= nil)

if failed > 0 then
    io.stderr:write(string.format("resolve_core: %d check(s) failed\n", failed))
    os.exit(1)
end
print("resolve_core: all checks passed")
