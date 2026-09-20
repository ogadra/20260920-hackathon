import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";
import {
  BASE_URL,
  RUNNER_COUNT,
  deleteShell,
  getCookie,
} from "./common.js";

const sessionsAllocated = new Counter("sessions_allocated");

export const options = {
  scenarios: {
    session_uniqueness: {
      executor: "shared-iterations",
      vus: RUNNER_COUNT + 10,
      iterations: RUNNER_COUNT + 10,
      maxDuration: "120s",
      gracefulStop: "10s",
    },
  },
  thresholds: {
    checks: ["rate==1.0"],
    http_req_duration: ["p(95)<10000"],
    sessions_allocated: [`count==${RUNNER_COUNT}`],
  },
};

export default function () {
  const res = http.post(`${BASE_URL}/api/shell`, null, {
    redirects: 0,
  });

  check(res, {
    "status is 204 (allocated) or 503 (no idle runner)": (r) =>
      r.status === 204 || r.status === 503,
  });

  if (res.status !== 204) return;

  sessionsAllocated.add(1);
  const sessionId = getCookie(res, "session_id");
  const shellId = getCookie(res, "shell_id");
  console.log(`SESSION_ID:${sessionId}`);
  deleteShell({ sessionId, shellId });
}
