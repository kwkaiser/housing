{ lib
, buildGoModule
, src
, version ? "dev"
}:

buildGoModule {
  pname = "housing";
  inherit version src;

  vendorHash = "sha256-B7TITji2l7Yfw/CWMhxVtoAOpZr8JjO4Vq33iT2gLXA=";

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
