import http from "k6/http";
import { check } from "k6";

export const BASE_URL = __ENV.BASE_URL;
if (!BASE_URL) {
  throw new Error("BASE_URL environment variable is required");
}

if (!__ENV.RUNNER_COUNT) {
  throw new Error("RUNNER_COUNT environment variable is required");
}
export const RUNNER_COUNT = Number(__ENV.RUNNER_COUNT);
if (!Number.isSafeInteger(RUNNER_COUNT) || RUNNER_COUNT <= 0) {
  throw new Error(
    `RUNNER_COUNT must be a positive integer, got: ${__ENV.RUNNER_COUNT}`,
  );
}

export function getCookie(res, name) {
  const cookies = res.cookies;
  if (cookies && cookies[name] && cookies[name].length > 0) {
    return cookies[name][0].value;
  }
  check(null, { [`cookie ${name} present`]: () => false });
  throw new Error(`cookie ${name} is missing`);
}

export function createSession() {
  const res = http.post(`${BASE_URL}/api/shell`, null, {
    redirects: 0,
  });
  check(res, {
    "POST /api/shell returns 204": (r) => r.status === 204,
  });
  const sessionId = getCookie(res, "session_id");
  const shellId = getCookie(res, "shell_id");
  return { sessionId, shellId };
}

export function cookieHeader(cookies) {
  return `session_id=${cookies.sessionId}; shell_id=${cookies.shellId}`;
}

export function deleteShell(cookies) {
  const res = http.del(`${BASE_URL}/api/shell`, null, {
    headers: { Cookie: cookieHeader(cookies) },
  });
  check(res, {
    "DELETE /api/shell returns 204": (r) => r.status === 204,
  });
}

export function splitSessionId(sessionId) {
  const sep = sessionId.indexOf("_");
  if (sep <= 0 || sep === sessionId.length - 1) {
    throw new Error(`session_id must be <stack>_<hex>, got: ${sessionId}`);
  }
  return { stack: sessionId.slice(0, sep), hex: sessionId.slice(sep + 1) };
}

export function getHandler(cookies) {
  const res = http.get(`${BASE_URL}/api/app/handler`, {
    headers: { Cookie: cookieHeader(cookies) },
  });
  check(res, {
    "GET /api/app/handler returns 200": (r) => r.status === 200,
  });
  return res.body;
}

export function putHandler(cookies, source) {
  const res = http.put(`${BASE_URL}/api/app/handler`, source, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      Cookie: cookieHeader(cookies),
    },
  });
  check(res, {
    "PUT /api/app/handler returns 204": (r) => r.status === 204,
  });
}

export function handlerModuleSource(marker) {
  return [
    "package DaiKichijoji;",
    "use strict;",
    `sub content { return qr/${marker}/; }`,
    "1;",
    "",
  ].join("\n");
}
