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
      cloud_webserver_pkg = final.callPackage ./default.nix { };
    };

    yaml_overlay = final: prev: {
      yaml_pkg = final.callPackage ./package.nix { };
    };

    dockerService = nixpkgs.buildEnv {
      name = "docker-service";
      buildInputs = [ ./docker.nix ];
    };

    nixosConfigurations.ec2 = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      


      modules =
        [{
          imports = [ ./modules/docker.nix ];

          config = {
            virtualisation.docker.enable = true;
            nixpkgs.config.allowUnfree = true; 
            nixpkgs.overlays = [ self.cloud_webserver_overlay mcap-protobuf.overlays.default mcap.overlays.default self.yaml_overlay ];
            users.users.nixos.extraGroups = [ "docker" ];
            
            
            #this unused code gets rid of the The ‘fileSystems’ option does not specify your root file system error
            fileSystems."/" =
              {
                device = "/dev/xvda";
                fsType = "ext4";
              };
            #this unused code gets rid of this error:
            #You must set the option ‘boot.loader.grub.devices’ or 'boot.loader.grub.mirroredBoots' to make the system bootable.
            #https://discourse.nixos.org/t/configure-grub-on-efi-system/2926/2
            boot.loader.grub.devices = [ "nodev" ];
            #the following user code also makes errors go away...
            users.users.nixos.isSystemUser = false;
            users.users.nixos.isNormalUser = true;
            #users.groups.nixos = { };
            #users.users.nixos.group = "nixos";
            services.openssh = {
              enable = true;
              # require public key authentication for better security
              settings.PasswordAuthentication = false;
              settings.KbdInteractiveAuthentication = false;
              #settings.PermitRootLogin = "yes";
            };
            users.users.nixos.openssh.authorizedKeys.keys = [
              # your own SSH public key.
              # Jason
              "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC8NWlt5zLQzQbAQ/IYOAcSmwxBUCzwL+iE5yZBQQWNa43ny+aLTKyZsb8S4gywuRT9/pqJELOzGBEKVaoW08tSy48U2/8fkLbJEwRQKIdb3cmvc6xu9dridVHVeETMULwGq99YaFhRDtUAI/d+tsseqGupmDV+/XgVP9UKN3jpa21fmpCNUn3z3lbbPdjRLLbdvphS/PTHAIBsgFxHQ7GAlBO0WhC39pEyyoWrQD+ip7r8iFUg5Tjd/hTH0zJPojZ/kC8DSxjyEQfX3zsJ//PZzf7p7gtbUSAAFNeMe7CKC0Y3ThnBaM/CKTnjH9tqmqTI6BYMaiawnm6ZwsM3oAb3ZpIR7vnhkr45jKzpKZJogid5cVdNLnTBoPd/I9nB0z/F79L9CQ8vFeag7Py0TxQJUvdXcxAVlgpyFEfsMCnf3VJDW1afLfc/dxhSyUyVqqEU1qCdzB6oKUHNgKv7RI2yzNyKEuh9HOVVn/4b/xDua4MYacYVYwKD60IPA4hgXzc= home@lawn-128-61-50-84.lawn.gatech.edu"
            ];
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

        }];
    };

  };
}
