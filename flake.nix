{
  description = "housing web app";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        linuxSystem = {
          "x86_64-darwin" = "x86_64-linux";
          "aarch64-darwin" = "aarch64-linux";
        }.${system} or system;

        linuxPkgs = nixpkgs.legacyPackages.${linuxSystem};

        envOr = name: fallback:
          let value = builtins.getEnv name; in if value == "" then fallback else value;

        version = envOr "IMAGE_TAG" (self.shortRev or self.dirtyShortRev or "dev");

        serverSrc = ./apps/server;
        webSrc = ./apps/web;
      in
      {
        packages.server = pkgs.callPackage ./nix/server.nix {
          src = serverSrc;
          inherit version;
        };

        packages.default = self.packages.${system}.server;

        packages.server-image = linuxPkgs.callPackage ./nix/server-image.nix {
          housing = linuxPkgs.callPackage ./nix/server.nix {
            src = serverSrc;
            inherit version;
          };
          name = envOr "IMAGE_NAME" "housing";
          tag = version;
        };

        packages.web = pkgs.callPackage ./nix/web.nix {
          src = webSrc;
          inherit version;
        };

        packages.web-image = linuxPkgs.callPackage ./nix/web-image.nix {
          web = linuxPkgs.callPackage ./nix/web.nix {
            src = webSrc;
            inherit version;
          };
          name = envOr "IMAGE_NAME" "housing-web";
          tag = version;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            go-task
            git
            skopeo
            nodejs
            pnpm
          ];
        };
      });
}
