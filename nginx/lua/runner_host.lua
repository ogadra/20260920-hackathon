-- brokerが返すX-Runner-Hostのhost labelを検証する。
-- SSRF / proxy_passヘッダーinjectionを防ぐため、
-- 英数字 / ドット / ハイフンのみのhost labelに限定する。
local _M = {}

function _M.is_valid(host)
    if type(host) ~= "string" then
        return false
    end
    return host:match("^[%w%.%-]+$") ~= nil
end

return _M
