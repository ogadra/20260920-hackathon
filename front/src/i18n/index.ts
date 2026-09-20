export type Lang = "en" | "ja";

export const MessageKey = {
  errorNoIdleRunner: "errorNoIdleRunner",
  errorSessionLost: "errorSessionLost",
  errorGatewayTimeout: "errorGatewayTimeout",
  errorBadGateway: "errorBadGateway",
  errorCommandTooLong: "errorCommandTooLong",
  errorCommandRejected: "errorCommandRejected",
  errorValidationUnavailable: "errorValidationUnavailable",
  errorNetwork: "errorNetwork",
  errorInternal: "errorInternal",
  termConnecting: "termConnecting",
  termRetrying: "termRetrying",
  termSessionRecreated: "termSessionRecreated",
  stackInfoLabel: "stackInfoLabel",
  stackInfoRegion: "stackInfoRegion",
  stackInfoClose: "stackInfoClose",
  stackRegionTokyo: "stackRegionTokyo",
  stackRegionOsaka: "stackRegionOsaka",
} as const;

export type MessageKey = (typeof MessageKey)[keyof typeof MessageKey];

type Messages = Record<Lang, Record<MessageKey, string>>;

const MESSAGES: Messages = {
  en: {
    errorNoIdleRunner: "No available execution environment",
    errorSessionLost: "Previous execution environment was not found",
    errorGatewayTimeout: "Server response timed out",
    errorBadGateway: "Cannot reach the execution environment",
    errorCommandTooLong: "The command is too long",
    errorCommandRejected: "Blocked by the safety check",
    errorValidationUnavailable: "The safety check is unavailable, so nothing was run",
    errorNetwork: "Cannot connect to the server",
    errorInternal: "An internal server error occurred",
    termConnecting: "Connecting…",
    termRetrying: "Retrying…",
    termSessionRecreated: "Session recreated. Run the command again.",
    stackInfoLabel: "Connection",
    stackInfoRegion: "Region",
    stackInfoClose: "Close",
    stackRegionTokyo: "Tokyo",
    stackRegionOsaka: "Osaka",
  },
  ja: {
    errorNoIdleRunner: "実行環境に空きがありません",
    errorSessionLost: "以前実行した環境が見つかりません",
    errorGatewayTimeout: "サーバー応答がタイムアウトしました",
    errorBadGateway: "実行環境に接続できません",
    errorCommandTooLong: "コマンドが長すぎます",
    errorCommandRejected: "安全性チェックで拒否されました",
    errorValidationUnavailable: "安全性チェックを実行できないため、コマンドを実行していません",
    errorNetwork: "サーバーに接続できません",
    errorInternal: "サーバー内部エラーが発生しました",
    termConnecting: "接続中…",
    termRetrying: "再試行します…",
    termSessionRecreated: "セッションを作り直しました。もう一度実行してください。",
    stackInfoLabel: "接続先",
    stackInfoRegion: "リージョン",
    stackInfoClose: "閉じる",
    stackRegionTokyo: "東京",
    stackRegionOsaka: "大阪",
  },
};

export const detectLang = (navigatorLanguage: string | undefined): Lang =>
  navigatorLanguage?.toLowerCase().startsWith("ja") === true ? "ja" : "en";

export const translate = (lang: Lang, key: MessageKey): string => MESSAGES[lang][key];
