import { describe, test, expect } from "vitest";
import { Region, classifyStack } from "./stackInfo";

describe("classifyStack", () => {
  test.each([
    ["ap-northeast-1", Region.TOKYO],
    ["ap-northeast-3", Region.OSAKA],
  ])("%s is served from %s", (stack, region) => {
    expect(classifyStack(stack)).toEqual({ region });
  });

  test("an unknown stack name is rejected", () => {
    expect(() => classifyStack("us-east-1")).toThrow("unknown stack name: us-east-1");
  });
});
