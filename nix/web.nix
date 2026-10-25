{ lib
, stdenvNoCC
, nodejs
, pnpm
, fetchPnpmDeps
, pnpmConfigHook
, src
, version ? "dev"
}:

stdenvNoCC.mkDerivation (finalAttrs: {
  pname = "housing-web";
  inherit version src;

  nativeBuildInputs = [
    nodejs
    pnpm
    pnpmConfigHook
  ];

  pnpmDeps = fetchPnpmDeps {
    inherit (finalAttrs) pname version src;
    fetcherVersion = 4;
    hash = "sha256-8lDzXU/lvQmjtwGVTmjjrMYMTbGmKyfqXcLOVs4el+0=";
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
