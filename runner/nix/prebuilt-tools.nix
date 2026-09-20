let
  pkgs = import (import ./nixpkgs-pin.nix) {
    system = "x86_64-linux";
  };
in
pkgs.symlinkJoin {
  name = "runner-prebuilt-tools";
  paths = with pkgs; [
    fastfetch
    cowsay
    pokemonsay
    lolcat
    figlet
    iproute2
    gawk
  ];
}
