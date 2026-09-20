import type { MessageKey } from "../i18n";

export class AppError extends Error {
  constructor(public readonly key: MessageKey) {
    super(key);
    this.name = "AppError";
  }
}
