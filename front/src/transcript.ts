import type { OutputBlock } from "./outputBlock";

const PROMPT = "❯";

export interface Transcript {
  begin(command: string): OutputBlock;
}

export const createTranscript = (
  root: HTMLElement,
  createBlock: (container: HTMLElement) => OutputBlock,
): Transcript => {
  const scrollToEnd = (): void => {
    root.scrollTop = root.scrollHeight;
  };

  return {
    begin(command: string): OutputBlock {
      const cell = document.createElement("section");
      cell.className = "cell";
      cell.dataset.running = "true";

      const commandLine = document.createElement("div");
      commandLine.className = "cell-command";
      const prompt = document.createElement("span");
      prompt.className = "cell-prompt";
      prompt.ariaHidden = "true";
      prompt.textContent = PROMPT;
      const text = document.createElement("code");
      text.textContent = command;
      commandLine.append(prompt, text);

      const output = document.createElement("div");
      output.className = "cell-output";

      cell.append(commandLine, output);
      root.append(cell);
      scrollToEnd();

      const block = createBlock(output);
      return {
        write(data: string): void {
          block.write(data);
          scrollToEnd();
        },
        finish(): void {
          block.finish();
          delete cell.dataset.running;
          scrollToEnd();
        },
      };
    },
  };
};
