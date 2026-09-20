import type { MessageKey } from "../i18n";

export class AppError extends Error {
  readonly key: MessageKey;
  /** Text from the server that the message alone cannot carry, such as the validator verdict. */
  readonly detail: string | null;

  constructor(key: MessageKey, detail: string | null = null) {
    super(detail === null ? key : `${key}: ${detail}`);
    this.name = "AppError";
    this.key = key;
    this.detail = detail;
  }
}
