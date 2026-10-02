{ lib
, dockerTools
, writeShellApplication
, housing
, name ? "housing"
, tag ? "dev"
}:

let
  entrypoint = writeShellApplication {
    name = "housing-entrypoint";
    text = ''
      data_dir="''${HOUSING_DATA_DIR:-/data}"
      addr="''${HOUSING_ADDR:-0.0.0.0:8080}"

      if ! ${lib.getExe housing} migrate --data-dir "$data_dir"; then
        echo "housing: migrations failed" >&2
        exit 1
      fi

      exec ${lib.getExe housing} server --data-dir "$data_dir" --addr "$addr" "$@"
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
    mkdir -p tmp data
    chmod 1777 tmp
    chmod 0777 data
  '';

  config = {
    Entrypoint = [ (lib.getExe entrypoint) ];
    ExposedPorts = { "8080/tcp" = { }; };
    Volumes = { "/data" = { }; };
    WorkingDir = "/";
    User = "nobody";
    Env = [
      "HOUSING_DATA_DIR=/data"
      "HOUSING_ADDR=0.0.0.0:8080"
    ];
  };
}
