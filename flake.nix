/*{
  description = "foxglove websocket protocol python library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
    mcap-protobuf.url = "github:RCMast3r/mcap-protobuf-support-flake";
    mcap.url = "github:RCMast3r/py_mcap_nix";

    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, mcap-protobuf, mcap, nixos-generators, ... }:
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
          users.users.nixos.extraGroups = [ "docker" ];#
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

          ami = nixos-generators.nixosGenerate {
        system = "x86_64-linux";
        format = "amazon";
        
      };
      
        };
        

        devShells = {
          default = shared_shell;
        };        
      });
}
*/
/*
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

*/

{
  description = "foxglove websocket protocol python library";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
    mcap-protobuf.url = "github:RCMast3r/mcap-protobuf-support-flake";
    mcap.url = "github:RCMast3r/py_mcap_nix";
  };

  outputs = { self, nixpkgs, flake-utils, mcap-protobuf, mcap, ... }: {
    cloud_webserver_overlay = final: prev: {
          cloud_webserver_pkg = final.callPackage ./default.nix {};
        };

        yaml_overlay = final: prev: {
          yaml_pkg = final.callPackage ./package.nix {};
        };
        
      nixosConfigurations.default = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
      
       
       modules =
        [{ 
          
          imports = [./modules/docker.nix];

          config = {
              nixpkgs.overlays = [ self.cloud_webserver_overlay mcap-protobuf.overlays.default mcap.overlays.default self.yaml_overlay];
          users.users.nixos.extraGroups = [ "docker" ];

          #this unused code gets rid of the The ‘fileSystems’ option does not specify your root file system error
          fileSystems."/" =
    { device = "/dev/disk/by-uuid/44444444-4444-4444-8888-888888888888";
      fsType = "ext4";
    };
    #this unused code gets rid of this error:
    #You must set the option ‘boot.loader.grub.devices’ or 'boot.loader.grub.mirroredBoots' to make the system bootable.
    #https://discourse.nixos.org/t/configure-grub-on-efi-system/2926/2
   boot.loader.grub.devices = [ "nodev" ];
   #the following user code also makes errors go away...
   users.users.nixos.isSystemUser = true;
    users.groups.nixos = {};
   users.users.nixos.group = "nixos";
          };
          
          
          options = {
             services.docker = {
            enable = true;
            rootless = {
              enable = true;
              setSocketVariable = true;
            };     
          };
          
          };
          
        }
        ];
    };

  };
}