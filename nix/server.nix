{ lib
, buildGoModule
, src
, version ? "dev"
}:

let
  depsKey = builtins.substring 0 16 (builtins.hashString "sha256"
    (builtins.readFile (src + "/go.mod") + builtins.readFile (src + "/go.sum")));
in
buildGoModule {
  pname = "housing";
  inherit version src;

  vendorHash = "sha256-b3O1dkEoq8+3UeSsCCu8YTRe2wqUgsw1vZGJrv3ZClI=";
  proxyVendor = true;
  overrideModAttrs = _: { name = "housing-go-modules-${depsKey}"; };

  subPackages = [ "cmd/housing" ];
  tags = [ "timetzdata" ];
  env.CGO_ENABLED = 0;

  ldflags = [
    "-s"
    "-w"
    "-X git.kwkaiser.io/kwkaiser/housing/internal/cli.version=${version}"
  ];

  doCheck = false;

  meta.mainProgram = "housing";
}
