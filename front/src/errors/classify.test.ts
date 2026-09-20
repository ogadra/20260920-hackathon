import { describe, test, expect } from "vitest";
import { AppError } from "./AppError";
import { classifyResponse, classifyThrown } from "./classify";
import type { MessageKey } from "../i18n";

const jsonResponse = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const rawResponse = (status: number, body: string): Response => new Response(body, { status });

describe("classifyResponse", () => {
  const cases: Array<{ name: string; res: Response; want: MessageKey }> = [
    {
      name: "broker 503 NO_IDLE_RUNNER",
      res: jsonResponse(503, { code: "NO_IDLE_RUNNER", message: "no idle runner available" }),
      want: "errorNoIdleRunner",
    },
    {
      name: "nginx 503 SERVICE_UNAVAILABLE (broker 503 through error_page)",
      res: jsonResponse(503, {
        code: "SERVICE_UNAVAILABLE",
        message: "Service temporarily unavailable.",
      }),
      want: "errorNoIdleRunner",
    },
    {
      name: "broker 404 SESSION_NOT_FOUND",
      res: jsonResponse(404, { code: "SESSION_NOT_FOUND", message: "session not found" }),
      want: "errorSessionLost",
    },
    {
      name: "nginx 504 GATEWAY_TIMEOUT",
      res: jsonResponse(504, { code: "GATEWAY_TIMEOUT", message: "Upstream timeout." }),
      want: "errorGatewayTimeout",
    },
    {
      name: "nginx 502 BAD_GATEWAY",
      res: jsonResponse(502, { code: "BAD_GATEWAY", message: "Upstream unavailable." }),
      want: "errorBadGateway",
    },
    {
      name: "runner 403 with the validator verdict",
      res: jsonResponse(403, { error: "safety probability 0.13 (threshold 0.80)" }),
      want: "errorCommandRejected",
    },
    {
      name: "runner 403 with an unusable validator response",
      res: jsonResponse(403, { error: "jev: response is not valid JSON" }),
      want: "errorCommandRejected",
    },
    {
      name: "runner 503 with the validator out of reach",
      res: jsonResponse(503, { error: "validation unavailable: 429 Too Many Requests" }),
      want: "errorValidationUnavailable",
    },
    {
      name: "nginx 403 FORBIDDEN folded to internal",
      res: jsonResponse(403, { code: "FORBIDDEN", message: "Session invalid." }),
      want: "errorInternal",
    },
    {
      name: "nginx 401 UNAUTHORIZED folded to internal",
      res: jsonResponse(401, { code: "UNAUTHORIZED", message: "Session cookie required." }),
      want: "errorInternal",
    },
    {
      name: "nginx 500 INTERNAL_ERROR folded to internal",
      res: jsonResponse(500, { code: "INTERNAL_ERROR", message: "Internal server error." }),
      want: "errorInternal",
    },
    {
      name: "504 without body falls back to gateway timeout by status",
      res: rawResponse(504, ""),
      want: "errorGatewayTimeout",
    },
    {
      name: "502 without body falls back to bad gateway by status",
      res: rawResponse(502, ""),
      want: "errorBadGateway",
    },
    {
      name: "503 without body falls back to no-idle-runner by status",
      res: rawResponse(503, ""),
      want: "errorNoIdleRunner",
    },
    {
      name: "nginx 413 over client_max_body_size, HTML body",
      res: rawResponse(413, "<html>413 Request Entity Too Large</html>"),
      want: "errorCommandTooLong",
    },
    {
      name: "unknown 418 folded to internal",
      res: rawResponse(418, ""),
      want: "errorInternal",
    },
    {
      name: "non-json body folded to internal by status",
      res: rawResponse(500, "<html>oops</html>"),
      want: "errorInternal",
    },
  ];

  test.each(cases)("$name → $want", async ({ res, want }) => {
    const err = await classifyResponse(res);
    expect(err).toBeInstanceOf(AppError);
    expect(err.key).toBe(want);
  });

  test("the validator verdict is carried as the detail", async () => {
    const err = await classifyResponse(
      jsonResponse(403, { error: "safety probability 0.13 (threshold 0.80)" }),
    );
    expect(err.detail).toBe("safety probability 0.13 (threshold 0.80)");
  });

  test("a response without a verdict carries no detail", async () => {
    const err = await classifyResponse(jsonResponse(503, { code: "NO_IDLE_RUNNER" }));
    expect(err.detail).toBeNull();
  });
});

describe("classifyThrown", () => {
  test("TypeError → errorNetwork", () => {
    const err = classifyThrown(new TypeError("Failed to fetch"));
    expect(err).toBeInstanceOf(AppError);
    expect(err.key).toBe("errorNetwork");
  });

  test("AppError passes through", () => {
    const original = new AppError("errorSessionLost");
    expect(classifyThrown(original)).toBe(original);
  });

  test("unknown throw → errorInternal", () => {
    expect(classifyThrown(new Error("boom")).key).toBe("errorInternal");
    expect(classifyThrown("bare string").key).toBe("errorInternal");
    expect(classifyThrown(undefined).key).toBe("errorInternal");
  });
});
