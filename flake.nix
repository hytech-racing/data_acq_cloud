{
  description = "foxglove websocket protocol python library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
    mcap-protobuf.url = "github:RCMast3r/mcap-protobuf-support-flake";
    mcap.url = "github:RCMast3r/py_mcap_nix";
  };

  outputs = { self, nixpkgs, flake-utils, mcap-protobuf, mcap, ... }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-darwin" "x86_64-darwin" ] (system:
      let
        cloud_webserver_overlay = final: prev: {
          cloud_webserver_pkg = final.callPackage ./default.nix { };
        };

        yaml_overlay = final: prev: {
          yaml_pkg = final.callPackage ./package.nix { };
        };

        pkgs = import nixpkgs {
          inherit system;
          overlays = [ cloud_webserver_overlay mcap-protobuf.overlays.default mcap.overlays.default yaml_overlay ];
        };

        # Add Docker to the system configuration
        config = {
          users.users.nixos.extraGroups = [ "docker" ];
          services.docker = {
            enable = true;
            rootless = {
              enable = true;
              setSocketVariable = true;
            };     
          };
          modules = [
            ./modules/docker.nix
          ];
        };

        shared_shell = pkgs.mkShell rec {
          name = "nix-devshell";
          packages = with pkgs; [
            cloud_webserver_pkg
            yaml_pkg
          ];

          shellHook = ''
            export PS1="\u${"f121"} \w (${name}) \$ "
          '';
        };

      in
      {
        packages = rec {
          cloud_webserver_pkg = pkgs.cloud_webserver_pkg;
          default = cloud_webserver_pkg;

          yaml_pkg = pkgs.yaml_pkg;

        };

        devShells = {
          default = shared_shell;
        };

      });
}
