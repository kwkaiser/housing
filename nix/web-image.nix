{ lib
, dockerTools
, writeShellApplication
, caddy
, web
, name ? "housing-web"
, tag ? "dev"
, revision ? "unknown"
}:

let
  entrypoint = writeShellApplication {
    name = "housing-web-entrypoint";
    runtimeInputs = [ caddy ];
    text = ''
      export HOUSING_WEB_ROOT=${web}
      exec caddy run --adapter caddyfile --config ${./Caddyfile}
    '';
  };
in
dockerTools.streamLayeredImage {
  inherit name tag;

  contents = [
    dockerTools.fakeNss
    dockerTools.caCertificates
  ];

  extraCommands = ''
    mkdir -p tmp
    chmod 1777 tmp
  '';

  config = {
    Labels = {
      "org.opencontainers.image.title" = "housing-web";
      "org.opencontainers.image.description" = "housing web app served by Caddy";
      "org.opencontainers.image.source" = "https://git.kwkaiser.io/kwkaiser/housing";
      "org.opencontainers.image.version" = tag;
      "org.opencontainers.image.revision" = revision;
    };
    Entrypoint = [ (lib.getExe entrypoint) ];
    ExposedPorts = { "8080/tcp" = { }; };
    WorkingDir = "/";
    User = "nobody";
    Env = [
      "HOME=/tmp"
      "XDG_DATA_HOME=/tmp"
      "XDG_CONFIG_HOME=/tmp"
      "HOUSING_WEB_LISTEN_PORT=8080"
      "HOUSING_API_URL=http://housing:8080"
    ];
  };
}
