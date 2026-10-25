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
        revision = self.rev or self.dirtyRev or "unknown";

        serverSrc = ./apps/server;
        webSrc = ./apps/web;

        androidPkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
            android_sdk.accept_license = true;
          };
        };

        androidBuildTools = "35.0.0";

        androidSdk = (androidPkgs.androidenv.composeAndroidPackages {
          platformVersions = [ "36" ];
          buildToolsVersions = [ androidBuildTools ];
          includeEmulator = false;
          includeNDK = false;
          includeSources = false;
          includeSystemImages = false;
        }).androidsdk;
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
          inherit revision;
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
          inherit revision;
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

        devShells.android = pkgs.mkShell {
          inputsFrom = [ self.devShells.${system}.default ];
          packages = [
            androidSdk
            pkgs.jdk21
            pkgs.curl
            pkgs.jq
          ];
          ANDROID_HOME = "${androidSdk}/libexec/android-sdk";
          ANDROID_SDK_ROOT = "${androidSdk}/libexec/android-sdk";
          JAVA_HOME = pkgs.jdk21.home;
          shellHook = ''
            export PATH="$ANDROID_HOME/build-tools/${androidBuildTools}:$PATH"
          '';
        };
      });
}
