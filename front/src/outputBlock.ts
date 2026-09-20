import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";

const FONT_SIZE = 22;

const ESCAPE = "\u001b";

export interface OutputBlock {
  write(data: string): void;
  finish(): void;
}

/**
 * Upper bound on the rows the data needs, counting every visible character as full width.
 * lolcat colours a single character with a whole escape sequence, so the escapes are dropped
 * rather than counted.
 *
 * Each newline claims a row of its own. A run counted as one long line reserves far too
 * little, and the lines beyond the scrollback limit leave the buffer for good.
 */
export const requiredRows = (data: string, cols: number): number => {
  let rows = 1;
  let width = 0;
  let escaped = false;
  for (const char of data) {
    if (char === ESCAPE) {
      escaped = true;
      continue;
    }
    if (escaped) {
      escaped = char < "@" || char > "~" || char === "[";
      continue;
    }
    if (char === "\n") {
      rows += Math.max(Math.ceil((width * 2) / cols), 1);
      width = 0;
      continue;
    }
    width += 1;
  }
  return rows + Math.ceil((width * 2) / cols);
};

export const createOutputBlock = (container: HTMLElement): OutputBlock => {
  // runnerのstdoutは\nだけを返す。
  // convertEolで行頭復帰込みに解釈させる
  const term = new Terminal({ convertEol: true, fontSize: FONT_SIZE, rows: 1 });
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(container);

  // 末尾の改行をそのまま書くとカーソルが空行へ進む。
  // その空行に合わせてrowsを詰めると、xtermは先頭の行をscrollbackへ捨てる
  let trailingNewlines = "";
  let content = "";

  const columns = (): number => {
    const proposed = fit.proposeDimensions();
    if (proposed === undefined) {
      throw new Error("output block: the container width is not measurable");
    }
    return proposed.cols;
  };

  // 高さはFitAddonに決めさせない。
  // ブロックはページと一緒に縦へ伸び、内側にスクロールを持たない
  const fitToContent = (): void => {
    const buffer = term.buffer.active;
    term.resize(columns(), Math.max(buffer.baseY + buffer.cursorY + 1, 1));
  };

  const push = (data: string): void => {
    const cols = columns();
    // rowsが足りないとxtermは溢れた行をscrollbackへ送る。
    // 送られた行はrowsを戻しても表示に返らないので、書く前に確保する
    term.resize(cols, term.rows + requiredRows(data, cols));
    term.write(data, fitToContent);
  };

  let width = container.clientWidth;
  const observer = new ResizeObserver(() => {
    // 行数を変えるとcontainerの高さも動く。
    // 幅が変わったときだけ折り返しを取り直す
    if (container.clientWidth === width) return;
    width = container.clientWidth;
    // 折り返しが増えるとxtermは溢れた行をscrollbackへ送る。
    // 幅に合わせた行数を確保しなおすため、空の端末に書き直す
    term.reset();
    term.resize(columns(), 1);
    if (content !== "") push(content);
  });
  observer.observe(container);

  return {
    write(data: string): void {
      const buffered = trailingNewlines + data;
      const body = buffered.replace(/\n+$/, "");
      trailingNewlines = buffered.slice(body.length);
      if (body === "") return;

      content += body;
      push(body);
    },
    finish(): void {
      term.write("", () => {
        if (content === "") {
          container.hidden = true;
          observer.disconnect();
          return;
        }
        fitToContent();
      });
    },
  };
};
