export const PRESET_COMMANDS = [
  "nix run nixpkgs#pokemonsay 'Nix'",
  "which pokemonsay",
  `nix develop --command sh -c "figlet 'Nix' | cowsay -n | lolcat -f"`,
] as const;

export interface PresetBar {
  setDisabled(disabled: boolean): void;
}

export const createPresetBar = (root: HTMLElement, run: (command: string) => void): PresetBar => {
  const buttons = PRESET_COMMANDS.map((command) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "preset";
    button.textContent = command;
    button.title = command;
    button.disabled = true;
    button.addEventListener("click", () => {
      run(command);
    });
    return button;
  });
  root.append(...buttons);

  return {
    setDisabled(disabled: boolean): void {
      for (const button of buttons) button.disabled = disabled;
    },
  };
};
