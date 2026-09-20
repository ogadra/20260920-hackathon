import { AppError } from "./errors/AppError";
import { classifyResponse } from "./errors/classify";
import { SessionReassignedError } from "./errors/SessionReassignedError";

// compose interpolationでSTACK_NAMEを焼き込むと、
// fallbackで別stackへ移ったセッションを追えない
const stackNameHeader = "X-Stack-Name";

const sessionReassignedHeader = "X-Session-Reassigned";

export const SseEventType = {
  STDOUT: "stdout",
  STDERR: "stderr",
  COMPLETE: "complete",
} as const;

export type SseEvent =
  | { type: typeof SseEventType.STDOUT; data: string }
  | { type: typeof SseEventType.STDERR; data: string }
  | { type: typeof SseEventType.COMPLETE; exitCode: number };

export interface Execution {
  stackName: string;
  events: AsyncGenerator<SseEvent>;
}

const requireHeader = (res: Response, name: string): string => {
  const value = res.headers.get(name);
  if (value === null) {
    console.error(`missing required header: ${name}`);
    throw new AppError("errorInternal");
  }
  return value;
};

export const createShell = async (signal?: AbortSignal): Promise<{ stackName: string }> => {
  const res = await fetch("/api/shell", { method: "POST", signal });
  if (!res.ok) throw await classifyResponse(res);
  return { stackName: requireHeader(res, stackNameHeader) };
};

export const deleteShell = (): void => {
  void fetch("/api/shell", { method: "DELETE", keepalive: true }).catch((err: unknown) => {
    console.error("Failed to delete shell", err);
  });
};

async function* readEvents(body: ReadableStream<Uint8Array>): AsyncGenerator<SseEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  const chunks: string[] = [];
  let completed = false;

  try {
    for (;;) {
      const { done, value } = await reader.read();
      chunks.push(done ? decoder.decode() : decoder.decode(value, { stream: true }));

      const lines = chunks.join("").split("\n");
      chunks.length = 0;
      if (!done) chunks.push(lines.pop()!);

      for (const line of lines) {
        if (!line.startsWith("data:")) continue;
        const payload = line.slice(5).trimStart();
        if (!payload) continue;
        yield JSON.parse(payload) as SseEvent;
      }

      if (done) {
        completed = true;
        break;
      }
    }
  } finally {
    if (!completed) {
      await reader.cancel();
    }
  }
}

// bodyを読み切ってから返すと、接続先の表示がstreamの終わりまで出ない
export const startExecute = async (command: string, signal?: AbortSignal): Promise<Execution> => {
  const res = await fetch("/api/execute", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ command }),
    signal,
  });
  if (res.headers.get(sessionReassignedHeader) === "true") {
    throw new SessionReassignedError();
  }
  if (!res.ok) throw await classifyResponse(res);
  if (!res.body) throw new Error("No response body");

  return { stackName: requireHeader(res, stackNameHeader), events: readEvents(res.body) };
};
