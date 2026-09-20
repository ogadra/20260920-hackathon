import { describe, test, expect, vi, beforeEach } from "vitest";
import { createShell, deleteShell, startExecute, SseEventType } from "./client";
import { AppError } from "./errors/AppError";
import { SessionReassignedError } from "./errors/SessionReassignedError";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

beforeEach(() => {
  mockFetch.mockReset();
});

const textEncoder = new TextEncoder();

const STACK = "ap-northeast-1";

const responseHeaders = (values: Record<string, string> = {}) => ({
  get: (name: string) => values[name] ?? null,
});

const stackHeaders = (values: Record<string, string> = {}) =>
  responseHeaders({ "X-Stack-Name": STACK, ...values });

const sseBody = (lines: string[]) => {
  const encoded = textEncoder.encode(lines.join("\n") + "\n");
  let read = false;
  return {
    headers: stackHeaders(),
    body: {
      getReader: () => ({
        read: async () => {
          if (read) return { done: true, value: undefined };
          read = true;
          return { done: false, value: encoded };
        },
        cancel: vi.fn(),
      }),
    },
  };
};

const jsonErrorResponse = (status: number, body: unknown) => ({
  ok: false,
  status,
  headers: responseHeaders(),
  clone() {
    return { json: async () => body };
  },
});

const collect = async (lines: string[]) => {
  mockFetch.mockResolvedValue({ ok: true, ...sseBody(lines) });
  const { events } = await startExecute("cmd");
  const collected = [];
  for await (const event of events) {
    collected.push(event);
  }
  return collected;
};

describe("createShell", () => {
  test("POST /api/shell", async () => {
    mockFetch.mockResolvedValue({ ok: true, headers: stackHeaders() });
    await createShell();
    expect(mockFetch).toHaveBeenCalledWith("/api/shell", {
      method: "POST",
      signal: undefined,
    });
  });

  test("returns the stack name the response was served from", async () => {
    mockFetch.mockResolvedValue({ ok: true, headers: stackHeaders() });
    await expect(createShell()).resolves.toEqual({ stackName: STACK });
  });

  test("a missing stack name is an internal error", async () => {
    mockFetch.mockResolvedValue({ ok: true, headers: responseHeaders() });
    const err = await createShell().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AppError);
    expect((err as AppError).key).toBe("errorInternal");
  });

  test("classifies a non-ok response", async () => {
    mockFetch.mockResolvedValue(
      jsonErrorResponse(503, { code: "NO_IDLE_RUNNER", message: "no idle runner available" }),
    );
    const err = await createShell().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AppError);
    expect((err as AppError).key).toBe("errorNoIdleRunner");
  });
});

describe("deleteShell", () => {
  test("DELETE /api/shell with keepalive", () => {
    mockFetch.mockResolvedValue({ ok: true });
    deleteShell();
    expect(mockFetch).toHaveBeenCalledWith("/api/shell", {
      method: "DELETE",
      keepalive: true,
    });
  });
});

describe("startExecute", () => {
  test("yields stdout and stderr events", async () => {
    const events = await collect([
      'data: {"type":"stdout","data":"hello"}',
      'data: {"type":"stderr","data":"warn"}',
      'data: {"type":"complete","exitCode":0}',
    ]);

    expect(events).toEqual([
      { type: SseEventType.STDOUT, data: "hello" },
      { type: SseEventType.STDERR, data: "warn" },
      { type: SseEventType.COMPLETE, exitCode: 0 },
    ]);
  });

  test("reports the stack name before the events are read", async () => {
    mockFetch.mockResolvedValue({ ok: true, ...sseBody([]) });
    const { stackName } = await startExecute("date");
    expect(stackName).toBe(STACK);
  });

  test("a missing stack name is an internal error", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      ...sseBody([]),
      headers: responseHeaders(),
    });
    const err = await startExecute("date").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AppError);
    expect((err as AppError).key).toBe("errorInternal");
  });

  test("classifies a non-ok response", async () => {
    mockFetch.mockResolvedValue(jsonErrorResponse(504, { code: "GATEWAY_TIMEOUT" }));
    const err = await startExecute("ls").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(AppError);
    expect((err as AppError).key).toBe("errorGatewayTimeout");
  });

  test("throws reassigned error when response has reassigned header", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 400,
      headers: stackHeaders({ "X-Session-Reassigned": "true" }),
    });
    await expect(startExecute("ls")).rejects.toBeInstanceOf(SessionReassignedError);
  });

  test("throws on missing body", async () => {
    mockFetch.mockResolvedValue({ ok: true, headers: stackHeaders(), body: null });
    await expect(startExecute("ls")).rejects.toThrow("No response body");
  });

  test("parses data: without trailing space", async () => {
    const events = await collect([
      'data:{"type":"stdout","data":"no-space"}',
      'data: {"type":"stdout","data":"with-space"}',
    ]);

    expect(events).toEqual([
      { type: SseEventType.STDOUT, data: "no-space" },
      { type: SseEventType.STDOUT, data: "with-space" },
    ]);
  });

  test("skips empty data: lines", async () => {
    const events = await collect(["data:", 'data: {"type":"stdout","data":"ok"}']);
    expect(events).toEqual([{ type: SseEventType.STDOUT, data: "ok" }]);
  });

  test("skips non-data lines", async () => {
    const events = await collect(["event: ping", "", 'data: {"type":"stdout","data":"ok"}']);
    expect(events).toEqual([{ type: SseEventType.STDOUT, data: "ok" }]);
  });

  test("cancels reader on early break", async () => {
    const cancelFn = vi.fn();
    const encoded = textEncoder.encode(
      'data: {"type":"stdout","data":"a"}\ndata: {"type":"stdout","data":"b"}\n',
    );
    mockFetch.mockResolvedValue({
      ok: true,
      headers: stackHeaders(),
      body: {
        getReader: () => ({
          read: async () => ({ done: false, value: encoded }),
          cancel: cancelFn,
        }),
      },
    });

    const { events } = await startExecute("cmd");
    for await (const _ of events) {
      break;
    }

    expect(cancelFn).toHaveBeenCalled();
  });
});
