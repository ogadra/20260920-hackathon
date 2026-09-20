# The master commit closest to the pinned nixpkgs.
# A far-apart pair re-derives the generation at runtime and fails offline.
builtins.fetchTarball {
  url = "https://github.com/nix-community/home-manager/archive/5d320ab301cfaaca7d32514f13815d19d109f5f4.tar.gz";
  sha256 = "sha256-qPNd6lUohHP5gcJhqQ7rLV87RwIx0xYR2A4Frb9Zjc4=";
}
