// @vitest-environment happy-dom
import { describe, expect, test } from "vitest";
import { requiredRows } from "./outputBlock";

const colour = (text: string): string =>
  [...text].map((ch, i) => `[38;5;${String(196 + (i % 6))}m${ch}`).join("");

describe("requiredRows", () => {
  test("counts every visible character as full width", () => {
    expect(requiredRows("0123456789", 10)).toBe(3);
  });

  test("adds a spare row to the wrapped width", () => {
    expect(requiredRows("ok", 80)).toBe(2);
  });

  test("asks for a single row when there is nothing to write", () => {
    expect(requiredRows("", 80)).toBe(1);
  });

  test("claims a row for every line in the chunk", () => {
    expect(requiredRows("a\nb\nc", 80)).toBe(4);
  });

  test("claims a row for a blank line", () => {
    expect(requiredRows("a\n\nb", 80)).toBe(4);
  });

  test("adds the wrapping of each line on top of its own row", () => {
    expect(requiredRows("0123456789\nok", 10)).toBe(4);
  });

  test("drops the escapes lolcat wraps around each character", () => {
    expect(requiredRows(colour("0123456789"), 10)).toBe(requiredRows("0123456789", 10));
  });

  test("keeps counting the characters that follow a colour change", () => {
    expect(requiredRows(`[38;5;198mNix[39m`, 80)).toBe(requiredRows("Nix", 80));
  });
});
