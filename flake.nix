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
      in
      {
        packages.default = pkgs.callPackage ./nix/package.nix {
          src = self;
          inherit version;
        };

        packages.image = linuxPkgs.callPackage ./nix/image.nix {
          housing = linuxPkgs.callPackage ./nix/package.nix {
            src = self;
            inherit version;
          };
          name = envOr "IMAGE_NAME" "housing";
          tag = version;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            go-task
            git
            skopeo
          ];
        };
      });
}
