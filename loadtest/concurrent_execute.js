import http from "k6/http";
import { check } from "k6";
import {
  BASE_URL,
  RUNNER_COUNT,
  cookieHeader,
  createSession,
  deleteShell,
} from "./common.js";

export const options = {
  scenarios: {
    concurrent_execute: {
      executor: "shared-iterations",
      vus: RUNNER_COUNT,
      iterations: RUNNER_COUNT,
      maxDuration: "120s",
      gracefulStop: "10s",
    },
  },
  thresholds: {
    checks: ["rate==1.0"],
    http_req_failed: ["rate==0.0"],
    http_req_duration: ["p(95)<10000"],
  },
};

function executeCommand(cookie, command) {
  const res = http.post(
    `${BASE_URL}/api/execute`,
    JSON.stringify({ command }),
    {
      headers: {
        "Content-Type": "application/json",
        Cookie: cookie,
      },
      timeout: "30s",
    },
  );
  check(res, {
    [`POST /api/execute (${command}) returns 200`]: (r) => r.status === 200,
    [`${command} response contains complete event`]: (r) =>
      /"type"\s*:\s*"complete"/.test(r.body),
    [`${command} response contains exitCode 0`]: (r) =>
      /"exitCode"\s*:\s*0\b/.test(r.body),
  });
}

export default function () {
  const cookies = createSession();
  const cookie = cookieHeader(cookies);

  executeCommand(cookie, "ls");
  executeCommand(cookie, "echo hello");

  deleteShell(cookies);
}
