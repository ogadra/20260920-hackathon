import { defineConfig } from "@playwright/test";

const chromiumExecutablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;

export default defineConfig({
  testDir: "./e2e",
  testMatch: /.*\.e2e\.ts/,
  use: {
    baseURL: "http://localhost:4273",
    // Playwright 同梱ブラウザは NixOS で実行できないため、dev shell が指す chromium を使う
    ...(chromiumExecutablePath
      ? { launchOptions: { executablePath: chromiumExecutablePath } }
      : {}),
  },
  webServer: {
    // 毎回フルビルドが走るので、ローカルで反復するときは別シェルで pnpm preview を
    // 起動しておけば reuseExistingServer で再利用される
    command: "pnpm build && pnpm preview --port 4273 --strictPort",
    url: "http://localhost:4273",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    stdout: "pipe",
  },
});
