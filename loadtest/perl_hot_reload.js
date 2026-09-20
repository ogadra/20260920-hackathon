import http from "k6/http";
import { check, sleep } from "k6";
import { Trend } from "k6/metrics";
import {
  RUNNER_COUNT,
  createSession,
  deleteShell,
  getHandler,
  handlerModuleSource,
  putHandler,
  splitSessionId,
} from "./common.js";

const PREVIEW_ORIGIN_TEMPLATE = __ENV.PREVIEW_ORIGIN_TEMPLATE;
if (!PREVIEW_ORIGIN_TEMPLATE) {
  throw new Error(
    "PREVIEW_ORIGIN_TEMPLATE environment variable is required (e.g. https://{hex}.{stack}.example.com)",
  );
}
if (
  !PREVIEW_ORIGIN_TEMPLATE.includes("{hex}") ||
  !PREVIEW_ORIGIN_TEMPLATE.includes("{stack}")
) {
  throw new Error(
    "PREVIEW_ORIGIN_TEMPLATE must contain {hex} and {stack} placeholders",
  );
}

const hotReloadDuration = new Trend("hot_reload_duration", true);

const RELOAD_DEADLINE_MS = 30000;

export const options = {
  scenarios: {
    perl_hot_reload: {
      executor: "shared-iterations",
      vus: RUNNER_COUNT,
      iterations: RUNNER_COUNT,
      maxDuration: "300s",
      gracefulStop: "10s",
    },
  },
  thresholds: {
    checks: ["rate==1.0"],
    hot_reload_duration: ["p(95)<10000"],
  },
};

export default function () {
  const cookies = createSession();
  const { stack, hex } = splitSessionId(cookies.sessionId);
  const previewUrl =
    PREVIEW_ORIGIN_TEMPLATE.replace("{hex}", hex).replace("{stack}", stack) +
    "/";

  const original = getHandler(cookies);
  const marker = `k6-hot-reload-${hex}`;
  putHandler(cookies, handlerModuleSource(marker));
  const putAt = Date.now();

  let reloaded = false;
  while (Date.now() - putAt < RELOAD_DEADLINE_MS) {
    const res = http.get(previewUrl);
    if (res.status === 200 && res.body.includes(marker)) {
      reloaded = true;
      break;
    }
    sleep(0.5);
  }
  check(reloaded, {
    "preview HTML shows updated content within deadline": (v) => v,
  });
  if (reloaded) {
    hotReloadDuration.add(Date.now() - putAt);
  }

  // Module::Refreshはmtime秒精度である。
  // マーカーPUTと同秒に書くとリロードされない。
  // 復元PUTは秒境界を跨いでから行う。
  const sinceMarkerPut = Date.now() - putAt;
  if (sinceMarkerPut < 1100) {
    sleep((1100 - sinceMarkerPut) / 1000);
  }
  putHandler(cookies, original);

  deleteShell(cookies);
}
