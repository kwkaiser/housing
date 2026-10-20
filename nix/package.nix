{ lib
, buildGoModule
, src
, version ? "dev"
}:

buildGoModule {
  pname = "housing";
  inherit version src;

  vendorHash = "sha256-e9etEum8Hrw9Pj9iM+bruDPPo2ArUELtvePE+hhtS6E=";

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
