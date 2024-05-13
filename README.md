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

### AWS Database Setup
## Starting Up EC2 Instance
In terminal run:
- export NIXPKGS_ALLOW_UNFREE=1
- nix shell nixpkgs#ec2_api_tools --impure
- ec2-run-instances -O AWS-ACCESS-KEY  -W AWS-SECRET-KEY --region us-east-1 ami-0c463a64 -k jason-key-pair -t t2.micro --block-device-mapping /dev/xvda=:32

- In the AWS console, change the ec2 security group to rds-security

If you want to change the AMI version, check here: https://github.com/NixOS/nixpkgs/blob/master/nixos/modules/virtualisation/amazon-ec2-amis.nix (for some reason 14.04 doesn’t work)
Make sure you change the security group of the EC2 instance so that it allows connections from any IP address
The AWS access keys can be found in IAM. Key pairs can be added through the EC2 dashboard on AWS. 
32 can be replaced with the desired size of the EBS volume in Gib

## To connect to the EC2 Instance

ssh -i "/path/to/your-key-pair.pem" ubuntu@ec2-107-20-116-116.compute-1.amazonaws.com

Make sure you’ve run: chmod 400 /path/to/your-key-pair.pem

If you get this error: sign_and_send_pubkey: no mutual signature supported, check out this stack overflow post: https://stackoverflow.com/a/74258486 

## Setting up EC2 Instance
In terminal run: 
- `sudo apt-get update && apt-get upgrade`
- `sudo apt install wireguard`
- `sudo mkdir /etc/wireguard/`
- `wg genkey | sudo tee /etc/wireguard/privatekey | wg pubkey | sudo tee /etc/wireguard/publickey`
- `sudo nano /etc/wireguard/wg0.conf`

In the configuration file, enter the settings:
```
[Interface]
Address = 10.0.0.1/24
SaveConfig = true
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
ListenPort = 51820
PrivateKey = [private_key]
```

You can get the private key with entering `cat /etc/wireguard/privatekey` in the terminal

Now run in the terminal:
- `sudo systemctl enable wg-quick@wg0`
- `sudo systemctl start wg-quick@wg0`
- `echo "net.ipv4.ip_forward = 1" | sudo tee -a /etc/sysctl.conf`
- `sudo sysctl -p`

To turn on the wireguard vpn, run `sudo wg-quick up wg0` and `sudo wg-quick save wg0`. \
To turn off the wireguard vpn, run `sudo wg-quick down wg0`

> **_NOTE:_** In order to actually have traffic flow through the vpn and port 51820, drop all security groups and set the new security group to `CloudWebServerSecurity`.


## Adding yourself to the wireguard vpn

ssh into the ec2 instance.

List the config with `sudo cat /etc/wireguard/wg0.conf`. The latest peer added is the peer at the end of the file. Look at the allowed-ips for that peer. 

Run in the terminal: `sudo wg set wg0 peer clientpublickey allowed-ips 10.0.0.[x]`. `[x]` is the latest peer's allowed-ips plus 1.

**On your computer for Linux (if on windows use WSL)**
Run in terminal:
- `sudo mkdir /etc/wireguard/`
- `wg genkey | sudo tee /etc/wireguard/privatekey | wg pubkey | sudo tee /etc/wireguard/publickey`
- `sudo nano /etc/wireguard/wg0.conf` (replace wg0 with another name if you already have a wg0. It doesn't matter too much does reflect the change in the next commands).

In the file, enter:
```
[Interface]
PrivateKey = [your_privatekey] # Private key of your computer, can be found with sudo cat /etc/wireguard/privatekey
Address = 10.0.0.[x]/24 # Whatever allowed-ips you set on the server

[Peer]
PublicKey = [publickey_server] # Public key of the server, can be found with sudo cat /etc/wireguard/publickey on the server
Endpoint = 107.20.116.116:51820
AllowedIPs = 0.0.0.0/0
```
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
