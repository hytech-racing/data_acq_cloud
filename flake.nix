{
  description = "foxglove websocket protocol python library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-darwin" "x86_64-darwin" ] (system:
      let
        cloud_webserver_overlay = final: prev: {
          cloud_webserver_pkg = final.callPackage ./default.nix { };
        };

        pkgs = import nixpkgs {
          inherit system;
          config = {
            allowUnfree = true;
            # Include any other global Nixpkgs configuration here
          };
          overlays = [ cloud_webserver_overlay ];
        };

        shared_shell = pkgs.mkShell rec {
          name = "nix-devshell";
          packages = with pkgs; [
            mongodb
            mongosh
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
        };

        devShells = {
          default = shared_shell;
        };

      });
}
