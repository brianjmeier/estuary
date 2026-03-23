{
  description = "Estuary";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        estuary = pkgs.buildGoModule {
          pname = "estuary";
          version = "pre-alpha";
          src = ./.;
          subPackages = [ "cmd/estuary" ];
          vendorHash = "sha256-uFl/NFP790G9k8RAv8/UbaV0cjnYtwIXKP+UnxFg4Wo=";

          meta = with pkgs.lib; {
            description = "Local TUI for Claude Code and Codex sessions";
            mainProgram = "estuary";
            homepage = "https://github.com/brianjmeier/estuary";
            license = licenses.mit;
            platforms = platforms.unix;
          };
        };
      in
      {
        packages = {
          estuary = estuary;
          default = estuary;
        };

        apps = {
          estuary = flake-utils.lib.mkApp {
            drv = estuary;
          };
          default = flake-utils.lib.mkApp {
            drv = estuary;
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            golangci-lint
            delve
            git
            sqlite
            gcc
          ];
        };
      });
}
