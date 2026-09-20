// @vitest-environment happy-dom
import { describe, test, expect, vi, beforeEach } from "vitest";
import type { OutputBlock } from "./outputBlock";
import { createTranscript } from "./transcript";

beforeEach(() => {
  document.body.innerHTML = "";
});

const setup = () => {
  const root = document.createElement("div");
  document.body.append(root);
  const containers: HTMLElement[] = [];
  const written: string[] = [];
  const finished = vi.fn();
  const createBlock = (container: HTMLElement): OutputBlock => {
    containers.push(container);
    return {
      write(data: string): void {
        written.push(data);
      },
      finish: finished,
    };
  };
  return { root, containers, written, finished, transcript: createTranscript(root, createBlock) };
};

describe("createTranscript", () => {
  test("a cell holds the command and an output container", () => {
    const { root, containers, transcript } = setup();

    transcript.begin("which pokemonsay");

    const cell = root.querySelector(".cell");
    expect(cell?.querySelector("code")?.textContent).toBe("which pokemonsay");
    expect(containers[0]).toBe(cell?.querySelector(".cell-output"));
  });

  test("cells stack in the order they were run", () => {
    const { root, transcript } = setup();

    transcript.begin("date");
    transcript.begin("whoami");

    const commands = [...root.querySelectorAll(".cell code")].map((el) => el.textContent);
    expect(commands).toEqual(["date", "whoami"]);
  });

  test("a running cell is marked until it finishes", () => {
    const { root, transcript } = setup();

    const block = transcript.begin("date");
    const cell = root.querySelector(".cell") as HTMLElement;
    expect(cell.dataset.running).toBe("true");

    block.finish();
    expect(cell.dataset.running).toBeUndefined();
  });

  test("writes and the finish reach the output block", () => {
    const { written, finished, transcript } = setup();

    const block = transcript.begin("date");
    block.write("2026-09-12\n");
    block.finish();

    expect(written).toEqual(["2026-09-12\n"]);
    expect(finished).toHaveBeenCalled();
  });
});
