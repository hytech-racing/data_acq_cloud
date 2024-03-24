{ lib, pkgs, config, ... }:
with lib;
let
  # Shorter name to access final settings a 
  # user of hello.nix module HAS ACTUALLY SET.
  # cfg is a typical convention.
  cfg = config.services.data_writer;
in {
  
  config = {
    # https://nixos.org/manual/nixos/stable/options.html search for systemd.services.<name>. to get list of all of the options for 
    # new systemd services
    systemd.services.data_writer = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig.After = [ "network.target" ];
      # https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html serviceconfig
      serviceConfig.ExecStart = "${pkgs.docker-compose}/bin/docker-compose -f ${pkgs.yaml_pkg}/docker-compose.yml up";
      #serviceConfig.ExecStart = "${pkgs.dockerComposeUp} ${pkgs.yaml_pkg}/docker-compose.yml";
      serviceConfig.ExecStop = "/bin/kill -SIGINT $MAINPID";
      serviceConfig.Restart = "on-failure";
    };
  };
  
}


