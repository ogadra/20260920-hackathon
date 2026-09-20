import { check } from "k6";
import {
  RUNNER_COUNT,
  createSession,
  deleteShell,
  getHandler,
  handlerModuleSource,
  putHandler,
  splitSessionId,
} from "./common.js";

export const options = {
  scenarios: {
    concurrent_edit: {
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

export default function () {
  const cookies = createSession();
  const { hex } = splitSessionId(cookies.sessionId);

  const original = getHandler(cookies);
  const source = handlerModuleSource(`k6-edit-${hex}`);
  putHandler(cookies, source);
  const stored = getHandler(cookies);
  check(stored, {
    "GET /api/app/handler returns the updated source": (s) => s === source,
  });

  putHandler(cookies, original);
  deleteShell(cookies);
}
