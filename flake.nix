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

        dockerService = nixpkgs.buildEnv {
          name = "docker-service";
          buildInputs = [ ./docker.nix ];
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
        };
        
        #used for nix develop?
        devShells = {
          default = shared_shell;
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
                
                networking.firewall.enable = false;

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
                users.users.nixos.extraGroups = [ "docker" "wheel" ];
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
                  # Sree
                  "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQDm9I3E71s3OjT/eqnBH66qkX09Wn9VQRVrSoKkebec2RLe1lAPde2ZZjRviM7yxKR3U46J9FQYgOy16qyyjsVYlgZoPS/04BEAche/89sjEmrPs3FLOfsJ37xjP7JHSGp30s7KCs0YLxlWmogT2qXNpEDkcZtKt2/YwCK/HUpIzdMihLT15BdpYB/iqany0OWaF+yA8g+fsS3Qlanfn0lOpngospvfLkQNnWl04lViL3UZkKGUWwvIZQQ/aK1AsBxDFnbaO85teA36RGBzAwbNKegZF4kcU0RdrGq1iqyM2OMG800kzCsX0VNPAbRoaJFGxN/NsYTcCBQ4fSDbY4/ZV+j9W+kDcBCYNKwoiceGPbFPS9gXXnNk9mBzT7jsTE1v/zJ62u9AMdQWKRY6VqfovOQnOiiplQGm7f7/99cmPEBxs7Y2TZzFDL+6xFhxTqmPe6F4y3anIWBWVMIL0l6uEO9MIcSpSQ5WaldMrN7Kx7L5+6FZgkxh1P7krnNEQNE= sreekara@SreekaraLaptop"
                  # Jason
                  "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC8NWlt5zLQzQbAQ/IYOAcSmwxBUCzwL+iE5yZBQQWNa43ny+aLTKyZsb8S4gywuRT9/pqJELOzGBEKVaoW08tSy48U2/8fkLbJEwRQKIdb3cmvc6xu9dridVHVeETMULwGq99YaFhRDtUAI/d+tsseqGupmDV+/XgVP9UKN3jpa21fmpCNUn3z3lbbPdjRLLbdvphS/PTHAIBsgFxHQ7GAlBO0WhC39pEyyoWrQD+ip7r8iFUg5Tjd/hTH0zJPojZ/kC8DSxjyEQfX3zsJ//PZzf7p7gtbUSAAFNeMe7CKC0Y3ThnBaM/CKTnjH9tqmqTI6BYMaiawnm6ZwsM3oAb3ZpIR7vnhkr45jKzpKZJogid5cVdNLnTBoPd/I9nB0z/F79L9CQ8vFeag7Py0TxQJUvdXcxAVlgpyFEfsMCnf3VJDW1afLfc/dxhSyUyVqqEU1qCdzB6oKUHNgKv7RI2yzNyKEuh9HOVVn/4b/xDua4MYacYVYwKD60IPA4hgXzc= home@lawn-128-61-50-84.lawn.gatech.edu"
                  # Jason (Mac Mini)
                  "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC6mUZ4OrUliWfoW48b8lNCSsZZVYjg2jqVIqjxnZdKgNav61sTaUJ76t2r+lI7MiBZN8tvS/LFBT9BlzRRmVWoDdzPbta/RI5Yd7N48MgzjeKsZAubaJzhv5iNjGTGe7+DlxxxVt0EmymCU7+eYxyUFaTB9lwY9DYGAy7GoOfDieo7u9FVN5MQAH8KVEoWzL+ruu1ia0wiREv13Z/4ECXiXqmgKF1Ul8nENmqCaFwBUka9Pu6E93MY1jHzVbRARsXCXwba0+E1HQcLMTj2EFmRa6NakZUctORL0+ybnyCIDH4hg00lNf/Y28O5jwxq/8rDPXIjZZfN09f1ZjW+3PV7s2CX8bT7t+Ojir+2bWjmjBdWJNl34kzm8CSQ4MibIuHgGY0QJz5YaVAn3WpalhcGxwH/+q2vLR7/WciQE/ohIkx9jj6h/TTy6ekzM0NkeY1Vhihna6l+ubPIo7Fzub7LoccN75jesa0LJYgXNkPn94qCtd3DAJuexsoaBJ0+9oE= home@r4-128-61-95-123.res.gatech.edu"
                ];

                #sudo does not need password
                security.sudo.wheelNeedsPassword = false;
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
      });
}

