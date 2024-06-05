# Data Acquisition Cloud Webserver

## Setup Packages (Optional)
> [!TIP]
> You don't have to do this, but you may want to so you get autocomplete with packages in whatever IDE you are using.

Make sure you are in root of `cloud_webserver`.

Install virtualenv via pip, `pip install virtualenv`.
Create your python virtual environment with `python -m venv <virtual-environment-name>`. 
If on windows, activate the virtual environment with `env/Scripts/activate.bat` if using cmd and `env/Scripts/Activate.ps1` if using powershell.

If on linux/mac, use `source env/bin/activate`.

Next, install the packages with `pip install -r requirements.txt`. Everytime you add a package to the environment, make sure to type `pip freeze > requirements.txt` to update the requirements file.

## Running Locally and Deploying

Source the two files `data_acq_cloud/cloud_webserver/bin/docker_up.sh` and `data_acq_cloud/cloud_webserver/bin/docker_down.sh`.

To start the webserver and database, run `docker_up.sh dev`. This will run the local docker database and the dockerized code. If you are running this for the first time, it will take a while because the script is creating a local database for you and populating it with mcap data you can use to test your code.

To stop the webserver and database, run `docker_down.sh dev`. This will shutdown the database as well.

If you just want to start your local database run `docker start local_hytechdb`.

You can enter the docker container with `docker exec -it local_hytechdb /bin/bash` and can then drop to the `mongosh` cli with `mongosh mongodb://username:password@localhost:27017/`.

To deploy your code, just push your changes to the github and merge to main.

## Run the Webserver on the EC2 instance

First, ssh into the EC2 instance and navigate to the `data_acq_cloud` directory.

Make sure you are on the main branch and pull the latest changes. Start the webserver and database with `./cloud_webserver/bin/docker_up.sh prod`. Stop the webserver and database with `./cloud_webserver/bin/docker_down.sh prod`. 

On your local computer, connect to the wireguard vpn. You can send requests to the server with the address: `10.0.0.1/{endpoint}`.

