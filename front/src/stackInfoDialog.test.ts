// @vitest-environment happy-dom
import { describe, test, expect, vi, beforeEach } from "vitest";
import { createStackInfoDialog } from "./stackInfoDialog";

beforeEach(() => {
  document.body.innerHTML = "";
});

const setup = (lang: "en" | "ja") => {
  const button = document.createElement("button");
  const dialog = document.createElement("dialog");
  // happy-domはshowModalもcloseも持たない
  const showModal = vi.fn();
  const close = vi.fn();
  Object.assign(dialog, { showModal, close });
  document.body.append(button, dialog);
  return {
    button,
    dialog,
    showModal,
    close,
    controller: createStackInfoDialog(button, dialog, lang),
  };
};

const valueOf = (dialog: HTMLDialogElement, role: string): string | null =>
  dialog.querySelector(`[data-role="${role}"]`)?.textContent ?? null;

describe("createStackInfoDialog", () => {
  test("the button is labelled and stays disabled until a stack is known", () => {
    const { button } = setup("ja");
    expect(button.textContent).toBe("接続先");
    expect(button.disabled).toBe(true);
  });

  test("a known stack enables the button and fills the dialog", () => {
    const { button, dialog, showModal, controller } = setup("ja");

    controller.setStack("ap-northeast-1");
    expect(button.disabled).toBe(false);

    button.click();
    expect(showModal).toHaveBeenCalled();
    expect(valueOf(dialog, "region-value")).toBe("東京");
  });

  test("the dialog follows the stack the session moved to", () => {
    const { button, dialog, controller } = setup("en");

    controller.setStack("ap-northeast-1");
    controller.setStack("ap-northeast-3");
    button.click();

    expect(valueOf(dialog, "region-value")).toBe("Osaka");
  });

  test("a click on the backdrop closes the dialog", () => {
    const { dialog, close } = setup("en");

    dialog.click();

    expect(close).toHaveBeenCalled();
  });

  test("a click inside the dialog leaves it open", () => {
    const { dialog, close } = setup("en");

    dialog.querySelector("dl")?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

    expect(close).not.toHaveBeenCalled();
  });

  test("an unknown stack name is rejected", () => {
    const { controller } = setup("en");
    expect(() => {
      controller.setStack("us-east-1");
    }).toThrow("unknown stack name: us-east-1");
  });
});
