{ lib
, buildGoModule
, src
, version ? "dev"
}:

buildGoModule {
  pname = "housing";
  inherit version src;

  vendorHash = "sha256-j19AVzMCzD88lkB2Lza51sG4esd9/6TaKeVlkDmawrg=";

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
