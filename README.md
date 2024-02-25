## hytech data acquisition cloud

this repo contains the infrastructure that will be running in the cloud for ease of deployment

requirements:
0. linux or MacOs environment (WSL works for windows)

1. [docker engine installed](https://docs.docker.com/engine/install/) 
    - NOTE: dont install docker desktop

2. nix installed and enable flakes:

    a. [install nix](https://nixos.org/download)

    b. enable flakes:

    - within `~/.config/nix/nix.conf` or `/etc/nix/nix.conf` add the line:
        
        ```
        experimental-features = nix-command flakes
        ```

## development guide:

- to bring up the database development container simply run: `./docker_bringup.sh`
- to shutdown the database dev container: `docker stop my_mongo`