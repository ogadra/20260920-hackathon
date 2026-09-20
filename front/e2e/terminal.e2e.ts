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

test("no editor UI is present", async ({ page }) => {
  await stubRunner(page, []);
  await page.goto("/");

  const dom = await page.evaluate(() => ({
    editor: document.getElementById("editor"),
    preview: document.getElementById("preview"),
  }));
  expect(dom.editor).toBeNull();
  expect(dom.preview).toBeNull();
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

  await run(page, "cowsay");

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
    { type: "stdout", data: "[38;5;198mNix[39m\n" },
    { type: "complete", exitCode: 0 },
  ]);
  await page.goto("/");
  await expect(command(page)).toBeEnabled();

  await run(page, "lolcat");

  // lolcat と pokemonsay は 256 色を使う。
  // xterm が色付きの span を起こすことを確認する
  await expect(lastCell(page).locator("span.xterm-fg-198").first()).toHaveText("Nix");
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
  const NIX_DEVELOP = `nix develop --command sh -c "figlet 'Nix' | cowsay -n | lolcat -f"`;

  test("one tap runs the hands-on command", async ({ page }) => {
    const stub = await stubRunner(page, [
      { type: "stdout", data: "/home/app/.nix-profile/bin/pokemonsay\n" },
      { type: "complete", exitCode: 0 },
    ]);
    await page.goto("/");
    await expect(command(page)).toBeEnabled();

    await page.locator("#presets button", { hasText: "which pokemonsay" }).click();

    expect(stub.commands).toEqual(["which pokemonsay"]);
    await expect(lastCell(page).locator("code")).toHaveText("which pokemonsay");
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

    await buttons.filter({ hasText: "which pokemonsay" }).click();
    await expect(buttons.first()).toBeDisabled();

    finish();

    await expect(buttons.first()).toBeEnabled();
  });

  test("every hands-on command has a button", async ({ page }) => {
    await stubRunner(page, []);
    await page.goto("/");

    await expect(page.locator("#presets button")).toHaveText([
      "nix run nixpkgs#pokemonsay 'Nix'",
      "which pokemonsay",
      NIX_DEVELOP,
    ]);
  });
});

test.describe("connection info", () => {
  test.use({ locale: "ja" });

  test("the button in the corner names the region and the cloud", async ({ page }) => {
    await stubRunner(page, []);
    await page.goto("/");

    const button = page.locator("#stack-info-button");
    await expect(button).toBeEnabled();
    await expect(button).toHaveText("接続先");

    await button.click();

    const dialog = page.locator("#stack-info-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText("東京");
    await expect(dialog).toContainText("AWS");
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
