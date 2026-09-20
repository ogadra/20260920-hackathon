# The rev comes from app/flake.lock.
# A second copy would split the store paths.
# The devShell and the prebuilt tools would stop sharing them.
let
  lock = builtins.fromJSON (builtins.readFile ./../app/flake.lock);
  nixpkgs = lock.nodes.nixpkgs.locked;
in
builtins.fetchTarball {
  url = "https://github.com/${nixpkgs.owner}/${nixpkgs.repo}/archive/${nixpkgs.rev}.tar.gz";
  sha256 = nixpkgs.narHash;
}
