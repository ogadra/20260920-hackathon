export const Region = {
  TOKYO: "tokyo",
  OSAKA: "osaka",
} as const;
export type Region = (typeof Region)[keyof typeof Region];

export type StackInfo = {
  region: Region;
};

const STACK_TABLE: Record<string, StackInfo> = {
  "ap-northeast-1": { region: Region.TOKYO },
  "ap-northeast-3": { region: Region.OSAKA },
};

export const classifyStack = (stack: string): StackInfo => {
  const known = STACK_TABLE[stack];
  if (known === undefined) throw new Error(`unknown stack name: ${stack}`);
  return known;
};
