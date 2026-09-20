import { describe, test, expect } from "vitest";
import { Cloud, Region, classifyStack } from "./stackInfo";

describe("classifyStack", () => {
  test.each([
    ["asia-northeast1", Region.TOKYO, Cloud.GOOGLE_CLOUD],
    ["asia-northeast2", Region.OSAKA, Cloud.GOOGLE_CLOUD],
    ["ap-northeast-1", Region.TOKYO, Cloud.AWS],
    ["ap-northeast-3", Region.OSAKA, Cloud.AWS],
  ])("%s is served from %s on %s", (stack, region, cloud) => {
    expect(classifyStack(stack)).toEqual({ region, cloud });
  });

  test("an unknown stack name is rejected", () => {
    expect(() => classifyStack("us-east-1")).toThrow("unknown stack name: us-east-1");
  });
});
