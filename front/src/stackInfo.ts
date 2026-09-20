export const Region = {
  TOKYO: "tokyo",
  OSAKA: "osaka",
} as const;
export type Region = (typeof Region)[keyof typeof Region];

export const Cloud = {
  GOOGLE_CLOUD: "google-cloud",
  AWS: "aws",
} as const;
export type Cloud = (typeof Cloud)[keyof typeof Cloud];

export type StackInfo = {
  region: Region;
  cloud: Cloud;
};

const STACK_TABLE: Record<string, StackInfo> = {
  "asia-northeast1": { region: Region.TOKYO, cloud: Cloud.GOOGLE_CLOUD },
  "asia-northeast2": { region: Region.OSAKA, cloud: Cloud.GOOGLE_CLOUD },
  "ap-northeast-1": { region: Region.TOKYO, cloud: Cloud.AWS },
  "ap-northeast-3": { region: Region.OSAKA, cloud: Cloud.AWS },
};

export const classifyStack = (stack: string): StackInfo => {
  const known = STACK_TABLE[stack];
  if (known === undefined) throw new Error(`unknown stack name: ${stack}`);
  return known;
};
