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

## Development Guide:

- to run the local cloud webserver and database, run `./cloud_webserver/bin/docker_up.sh dev`.
  - If you are running this for the first time, it will take a while because it is creating and populating a local database for you to use.
- to shutdown the server and database containers, run `./cloud_webserver/bin/docker_down.sh dev`

- to enter the docker container and run commands to interact with the database using `mongosh`: 
```
docker exec -it local_hytechdb /bin/bash
```

- to drop to `mongosh` CLI for looking at contents of database:
```
mongosh mongodb://username:password@localhost:27017/
```
- in `mongosh` shell you can use the following commands

    - `show databases` to list the databases that exist in the docker container
    - `use hytechDB` to switch to database that the script is writing to
    - `show collections` to see the collections that have been written to
    - `db.<insert-collection-name-here>.find()` to list all data in specific collection

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

> [!NOTE]
> In order to actually have traffic flow through the vpn and port 51820, drop all security groups and set the new security group to `CloudWebServerSecurity`.


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
## Deploying on the EC2 instance

Navigate to the `data_acq_cloud/cloud_webserver` directory and run `./bin/docker_up.sh prod` to start the webserver and the database.

Navigate to the `data_acq_cloud/frontend/visualize` directory and run `nohup serve -s build &`. This will run the website asynchronously with a process id.
- To stop the frontend, run `ps aux | grep "serve -s build"` to find the process id of the frontend. Stop the frontend with `kill <process id>`

> [!NOTE]
> This is a pretty obnoxious and bad way to run the frontend. We can probably just dockerize it like the webserver, but until then this works.
> Also, we aren't building and running the frontend along with the backend in the docker compose yml file because the aws free tier doesn't have enough memory to do so.

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
