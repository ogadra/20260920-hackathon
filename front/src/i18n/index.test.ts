import { describe, test, expect } from "vitest";
import { detectLang, MessageKey, translate, type Lang } from "./index";

describe("detectLang", () => {
  test.each([
    ["ja", "ja"],
    ["ja-JP", "ja"],
    ["JA-jp", "ja"],
    ["en", "en"],
    ["en-US", "en"],
    ["fr", "en"],
    ["", "en"],
    [undefined, "en"],
  ] satisfies Array<[string | undefined, Lang]>)("navigator.language %s → %s", (input, want) => {
    expect(detectLang(input)).toBe(want);
  });
});

describe("translate", () => {
  const EXPECTED: Record<MessageKey, { en: string; ja: string }> = {
    errorNoIdleRunner: {
      en: "No available execution environment",
      ja: "実行環境に空きがありません",
    },
    errorSessionLost: {
      en: "Previous execution environment was not found",
      ja: "以前実行した環境が見つかりません",
    },
    errorGatewayTimeout: {
      en: "Server response timed out",
      ja: "サーバー応答がタイムアウトしました",
    },
    errorBadGateway: {
      en: "Cannot reach the execution environment",
      ja: "実行環境に接続できません",
    },
    errorCommandTooLong: { en: "The command is too long", ja: "コマンドが長すぎます" },
    errorCommandRejected: {
      en: "Blocked by the safety check",
      ja: "安全性チェックで拒否されました",
    },
    errorValidationUnavailable: {
      en: "The safety check is unavailable, so nothing was run",
      ja: "安全性チェックを実行できないため、コマンドを実行していません",
    },
    errorNetwork: { en: "Cannot connect to the server", ja: "サーバーに接続できません" },
    errorInternal: {
      en: "An internal server error occurred",
      ja: "サーバー内部エラーが発生しました",
    },
    termConnecting: { en: "Connecting…", ja: "接続中…" },
    termRetrying: { en: "Retrying…", ja: "再試行します…" },
    termSessionRecreated: {
      en: "Session recreated. Run the command again.",
      ja: "セッションを作り直しました。もう一度実行してください。",
    },
    stackInfoLabel: { en: "Connection", ja: "接続先" },
    stackInfoRegion: { en: "Region", ja: "リージョン" },
    stackInfoClose: { en: "Close", ja: "閉じる" },
    stackRegionTokyo: { en: "Tokyo", ja: "東京" },
    stackRegionOsaka: { en: "Osaka", ja: "大阪" },
  };

  test.each(Object.values(MessageKey))("%s resolves to expected en/ja", (key) => {
    expect(translate("en", key)).toBe(EXPECTED[key].en);
    expect(translate("ja", key)).toBe(EXPECTED[key].ja);
  });
});
