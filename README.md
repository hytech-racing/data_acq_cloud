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

- to enter the docker container and run commands to interact with the database using `mongosh`: 
```
docker exec -it my_mongo /bin/bash
```

- to drop to `mongosh` CLI for looking at contents of database:
```
mongosh mongodb://admin:password@localhost:27017/
```
- in `mongosh` shell you can use the following commands

    - `show databases` to list the databases that exist in the docker container
    - `use HyTech_database` to switch to database that the script is writing to (can be seen on line 62 of the `write_and_read_metas.py` script)
    - `show collections` to see the collections that have been written to
    - `db.<insert-collection-name-here>.find()` to list all data in specific collection

### data acquisition data flow
```mermaid
flowchart TD
subgraph file offload
    direction BT
    car[on car data] --mcap file upload over ubiquiti.--> panda[base station]
    panda[base station] -.mcap file upload over internt.-> aws[(data acq cloud DB / file storage)]
end
subgraph data provision
    
    aws2[(data acq cloud DB / file storage)] -.HTTP protocol.-> website[query builder site]
    website --> file_serv
    website --> mat
    aws2 <--user MAT query.-> mat[MAT file builder]
    mat -.-> file_serv
    aws2 <-.user MCAP query.-> file_serv[file download link]
    file_serv -.-> matlab
    file_serv -.-> python
    file_serv -.-> foxglove
end
```