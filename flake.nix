{
  description = "foxglove websocket protocol python library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
  };
  outputs = { self, nixpkgs }:
    let
      cloud_webserver_overlay = final: prev: {
        cloud_webserver_pkg = final.callPackage ./default.nix { };
      };
      my_overlays = [ cloud_webserver_overlay ];
      pkgs = import nixpkgs {
        system = "x86_64-linux";
        overlays = [ self.overlays.default ];
      };

      shared_shell = pkgs.mkShell rec {
        # Update the name to something that suites your project.
        name = "nix-devshell";
        packages = with pkgs; [
          # Development Tools
          mongodb
          mongosh
        ];

        # Setting up the environment variables you need during
        # development.
        shellHook =
          let
            icon = "f121";
          in
          ''
            export PS1="$(echo -e '\u${icon}') {\[$(tput sgr0)\]\[\033[38;5;228m\]\w\[$(tput sgr0)\]\[\033[38;5;15m\]} (${name}) \\$ \[$(tput sgr0)\]"
          '';
      };
      shared_pkgs = rec {
        cloud_webserver_pkg = pkgs.cloud_webserver_pkg;
        default = cloud_webserver_pkg;
      };
    in
    {
      overlays.default = nixpkgs.lib.composeManyExtensions my_overlays;

      packages.x86_64-linux = shared_pkgs;
      packages.aarch64-darwin = shared_pkgs;
      packages.x86_64-darwin = shared_pkgs;



      devShells.x86_64-linux.default = shared_shell;
      devShells.aarch64-darwin.default = shared_shell;
      devShells.x86_64-darwin.default = shared_shell;


    };
}
