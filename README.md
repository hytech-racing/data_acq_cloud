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

## Setting Up EC2 Instance
In terminal run:
- export NIXPKGS_ALLOW_UNFREE=1
- nix shell nixpkgs#ec2_api_tools --impure
- ec2-run-instances -O AWS-ACCESS-KEY  -W AWS-SECRET-KEY --region us-east-1 ami-0c463a64 -k jason-key-pair -t t2.micro

If you want to change the AMI version, check here: https://github.com/NixOS/nixpkgs/blob/master/nixos/modules/virtualisation/amazon-ec2-amis.nix (for some reason 14.04 doesn’t work)
Make sure you change the security group of the EC2 instance so that it allows connections from any IP address
The AWS access keys can be found in IAM. Key pairs can be added through the EC2 dashboard on AWS. 


## To connect to the EC2 Instance

ssh -i "/path/to/your-key-pair.pem" root@ec2-44-204-90-124.compute-1.amazonaws.com

Make sure you’ve run: chmod 400 /path/to/your-key-pair.pem

If you get this error: sign_and_send_pubkey: no mutual signature supported, check out this stack overflow post: https://stackoverflow.com/a/74258486 

### data acquisition data flow
```mermaid

flowchart TD
subgraph file offload
    direction BT
    car[on car data] --mcap file upload over ubiquiti--> panda[base station]
    panda[base station] -.mcap file upload over internt.-> aws[(data acq cloud DB / file storage)]
end
subgraph data provision
    
    aws2[(data acq cloud DB / file storage)] -.HTTP protocol.-> website[query builder site]
    website --> file_serv
    website --> mat
    aws2 <-.user MAT query.-> mat[MAT file builder]
    mat -.-> file_serv
    aws2 <-.user MCAP query.-> file_serv[file download link]
    file_serv -.-> matlab
    file_serv -.-> python
    file_serv -.-> foxglove
end
```

### data acquisition overview
- data acquisition management website (built into [data_acq](https://github.com/RCMast3r/data_acq/))
    - [x] handles starting / stopping of recording
    - [ ] handles the entry and management of the metadata that gets written into each log
    - [ ] interfaces with the `base_station_service` for handling offloading of the data from the car
    - runs on the car itself

- `base_station_service` 
    - [ ] python service that runs on the panda / base station computer that handles the upload over an internet connection
    - [ ] communicates with the car to determine which logs havent been pulled off the car yet and pulls the ones that dont exist on the base station file system yet (data offload)
    - [ ] communicates with the cloud hosted database and determines which mcap files arent a part of the database yet and uploads the ones that dont exist on remote yet (database ingress)

- `cloud_webserver`
    - [ ] handles the creation of new records in the mongodb infrastructure
    - [ ] serves the query creation utility website that allows users to download selections of recorded data in multiple formats 
    - [ ] handles the conversion from the MCAP files into other data formats on-demand (for now will only support MAT file formats)
    - [ ] handles automated backup of mongodb database states and associated MCAP files
