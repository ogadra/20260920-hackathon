import { detectLang } from "./i18n";
import { createOutputBlock } from "./outputBlock";
import { createPresetBar } from "./presets";
import { createStackInfoDialog } from "./stackInfoDialog";
import { initTerminal } from "./terminal";
import { createTranscript } from "./transcript";
import "./style.css";

const lang = detectLang(navigator.language);
const form = document.getElementById("input-bar") as HTMLFormElement;

const transcript = createTranscript(
  document.getElementById("transcript") as HTMLElement,
  createOutputBlock,
);

const stackInfo = createStackInfoDialog(
  document.getElementById("stack-info-button") as HTMLButtonElement,
  document.getElementById("stack-info-dialog") as HTMLDialogElement,
  lang,
);

const terminal = initTerminal(
  transcript,
  {
    form,
    input: document.getElementById("command") as HTMLInputElement,
    button: form.querySelector("button") as HTMLButtonElement,
    status: document.getElementById("status") as HTMLElement,
  },
  lang,
  (stackName) => {
    stackInfo.setStack(stackName);
  },
);

const presets = createPresetBar(document.getElementById("presets") as HTMLElement, (command) => {
  terminal.run(command);
});
terminal.setBusyListener((busy) => {
  presets.setDisabled(busy);
});
