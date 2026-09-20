import { translate, type Lang, type MessageKey } from "./i18n";
import { Region, classifyStack, type StackInfo } from "./stackInfo";

export interface StackInfoDialog {
  setStack(stack: string): void;
}

const REGION_KEY: Record<Region, MessageKey> = {
  [Region.TOKYO]: "stackRegionTokyo",
  [Region.OSAKA]: "stackRegionOsaka",
};

export const createStackInfoDialog = (
  button: HTMLButtonElement,
  dialog: HTMLDialogElement,
  lang: Lang,
): StackInfoDialog => {
  let current: StackInfo | null = null;

  const t = document.createElement("template");
  t.innerHTML = `
    <h2 data-role="title" id="stack-info-title" class="stack-info-title"></h2>
    <dl class="stack-info-list">
      <dt data-role="region-label"></dt><dd data-role="region-value"></dd>
    </dl>
    <form method="dialog" class="stack-info-actions">
      <button type="submit" data-role="close"></button>
    </form>
  `;
  dialog.replaceChildren(t.content.cloneNode(true));
  dialog.setAttribute("aria-labelledby", "stack-info-title");

  const pick = <T extends Element>(role: string): T => {
    const el = dialog.querySelector<T>(`[data-role="${role}"]`);
    if (el === null) throw new Error(`stack info dialog: missing element with data-role="${role}"`);
    return el;
  };
  const title = pick<HTMLElement>("title");
  const regionLabel = pick<HTMLElement>("region-label");
  const regionValue = pick<HTMLElement>("region-value");
  const closeButton = pick<HTMLButtonElement>("close");

  title.textContent = translate(lang, "stackInfoLabel");
  regionLabel.textContent = translate(lang, "stackInfoRegion");
  closeButton.textContent = translate(lang, "stackInfoClose");
  button.textContent = translate(lang, "stackInfoLabel");
  button.disabled = true;

  button.addEventListener("click", () => {
    if (current === null) return;
    regionValue.textContent = translate(lang, REGION_KEY[current.region]);
    dialog.showModal();
  });

  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) dialog.close();
  });

  return {
    setStack(stack: string): void {
      current = classifyStack(stack);
      button.disabled = false;
    },
  };
};
