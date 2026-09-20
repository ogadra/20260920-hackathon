import type { MessageKey } from "../i18n";
import { AppError } from "./AppError";

type ErrorBody = { code?: unknown; error?: unknown };

const CODE_MAP: Record<string, MessageKey> = {
  NO_IDLE_RUNNER: "errorNoIdleRunner",
  SERVICE_UNAVAILABLE: "errorNoIdleRunner",
  SESSION_NOT_FOUND: "errorSessionLost",
  GATEWAY_TIMEOUT: "errorGatewayTimeout",
  BAD_GATEWAY: "errorBadGateway",
};

// 拒否と検証不能はどちらもrunnerのvalidatorだけがerror本文で返す。
// codeを返すbroker/nginxの同じstatusとは本文の形で分かれる
const VALIDATION_MAP: Record<number, MessageKey> = {
  403: "errorCommandRejected",
  503: "errorValidationUnavailable",
};

const parseBody = async (res: Response): Promise<ErrorBody> => {
  try {
    return (await res.clone().json()) as ErrorBody;
  } catch {
    return {};
  }
};

const keyFromStatus = (status: number): MessageKey => {
  // nginx が client_max_body_size を超えた body を弾いた場合、JSON ではなく既定の HTML を返す
  if (status === 413) return "errorCommandTooLong";
  if (status === 504) return "errorGatewayTimeout";
  if (status === 502) return "errorBadGateway";
  if (status === 503) return "errorNoIdleRunner";
  return "errorInternal";
};

export const classifyResponse = async (res: Response): Promise<AppError> => {
  const body = await parseBody(res);
  const code = typeof body.code === "string" ? body.code : "";
  const reason = typeof body.error === "string" ? body.error : "";
  console.error("classifyResponse", { status: res.status, body });
  if (code !== "" && CODE_MAP[code] !== undefined) {
    return new AppError(CODE_MAP[code]);
  }
  const validationKey = VALIDATION_MAP[res.status];
  if (reason !== "" && validationKey !== undefined) {
    return new AppError(validationKey, reason);
  }
  return new AppError(keyFromStatus(res.status));
};

// user視点で区別する意味がないため、TypeError以外はinternalに丸める。
export const classifyThrown = (err: unknown): AppError => {
  if (err instanceof AppError) return err;
  console.error("classifyThrown", err);
  if (err instanceof TypeError) return new AppError("errorNetwork");
  return new AppError("errorInternal");
};
