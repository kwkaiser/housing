{ lib
, stdenvNoCC
, nodejs
, pnpm
, fetchPnpmDeps
, pnpmConfigHook
, src
, version ? "dev"
}:

let
  depsKey = builtins.substring 0 16 (builtins.hashString "sha256"
    (lib.concatMapStrings (f: builtins.readFile (src + "/${f}"))
      [ "package.json" "pnpm-lock.yaml" "pnpm-workspace.yaml" ]));
in
stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "housing-web";
  inherit version src;

  nativeBuildInputs = [
    nodejs
    pnpm
    pnpmConfigHook
  ];

  pnpmDeps = fetchPnpmDeps {
    inherit (finalAttrs) version src;
    pname = "housing-web-${depsKey}";
    fetcherVersion = 4;
    hash = "sha256-fkkeVC1Ox9VbfT62LJyGTEInO5IKNuOLwagiep6qNhg=";
    postInstall = ''
      rm -rf "$storePath"/v11/links
    '';
  };

  env.HOUSING_VERSION = version;

  buildPhase = ''
    runHook preBuild
    pnpm build
    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall
    cp -r build $out
    runHook postInstall
  '';
})
