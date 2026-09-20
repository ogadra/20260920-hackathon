// @vitest-environment happy-dom
import { describe, test, expect, beforeEach } from "vitest";
import { createPresetBar, PRESET_COMMANDS } from "./presets";

beforeEach(() => {
  document.body.innerHTML = "";
});

const setup = () => {
  const root = document.createElement("div");
  document.body.append(root);
  const run: string[] = [];
  const bar = createPresetBar(root, (command) => {
    run.push(command);
  });
  const buttons = (): HTMLButtonElement[] => [...root.querySelectorAll("button")];
  return { bar, buttons, run };
};

describe("createPresetBar", () => {
  test("the hands-on commands each get a button", () => {
    const { buttons } = setup();
    expect(buttons().map((button) => button.textContent)).toEqual([...PRESET_COMMANDS]);
  });

  test("a button runs its command in one tap", () => {
    const { bar, buttons, run } = setup();
    bar.setDisabled(false);

    buttons()[1].click();

    expect(run).toEqual(["which pokemonsay"]);
  });

  test("the buttons start disabled and follow setDisabled", () => {
    const { bar, buttons } = setup();
    expect(buttons().every((button) => button.disabled)).toBe(true);

    bar.setDisabled(false);
    expect(buttons().every((button) => button.disabled)).toBe(false);

    bar.setDisabled(true);
    expect(buttons().every((button) => button.disabled)).toBe(true);
  });
});
