{ lib
, buildGoModule
, src
, version ? "dev"
}:

buildGoModule {
  pname = "housing";
  inherit version src;

  vendorHash = "sha256-6Clp6flWtoHPIqYaSvLr0cv6c+MCHaWZsQG0LMkCkk4=";

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
