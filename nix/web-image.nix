{ lib
, dockerTools
, writeShellApplication
, caddy
, web
, name ? "housing-web"
, tag ? "dev"
}:

let
  entrypoint = writeShellApplication {
    name = "housing-web-entrypoint";
    runtimeInputs = [ caddy ];
    text = ''
      export HOUSING_WEB_ROOT=${web}
      if [ -n "''${HOUSING_API_KEY:-}" ]; then
        export HOUSING_API_AUTHORIZATION="Bearer $HOUSING_API_KEY"
      else
        export HOUSING_API_AUTHORIZATION=""
      fi
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
    Entrypoint = [ (lib.getExe entrypoint) ];
    ExposedPorts = { "8080/tcp" = { }; };
    WorkingDir = "/";
    User = "nobody";
    Env = [
      "HOME=/tmp"
      "XDG_DATA_HOME=/tmp"
      "XDG_CONFIG_HOME=/tmp"
      "HOUSING_WEB_PORT=8080"
      "HOUSING_API_URL=http://housing:8080"
    ];
  };
}
