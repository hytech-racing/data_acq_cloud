# Data Acquisition Cloud Webserver

## Setup Packages

Make sure you are in root of cloud_webserver.

Install virtualenv via pip, `pip install virtualenv`.
Create your python virtual environment with `python -m venv <virtual-environment-name>`. 
If on windows, activate the virtual environment with `env/Scripts/activate.bat` if usingcmd and `env/Scripts/Activate.ps1` if using powershell.

If on linux/mac, use `source env/bin/activate`.

Next, install the packages with `pip install -r requirements.txt`. Everytime you add a package to the environment, make sure to type `pip freeze > requirements.txt` to update the requirements file.

## Running Locally and Deploying

Run the server with `waitress-serve --port=8080 --call app:create_app`. 

To create the docker image, run `docker build -t cloud-webserver:<tag> .`. You can check if the docker runs with `docker run -p 8080:8080 cloud-webserver`. 


Tag the local image with `docker tag cloud-webserver:<tag> <hytech_username>/cloud-webserver:<tag>`

To push a docker image, setup login with docker. Create a text file and put your token there. Login to docker with `cat <path-to-token> | docker login --username <hytech_username> --password-stdin`.

Push the docker image with `docker push <hytech_username>/cloud-webserver:<tag>`.

## Run the Webserver on the EC2 instance

First, ssh into the EC2 instance. Then, login to docker the same way you do on your local machine (there should already be a text file with the token, so use that). Pull the docker image with `docker pull <hytech_username>/cloud-webserver:<tag>`. You can check it exists with `docker images`. 

Run the docker image with `docker run -d -p 8080:8080 <image_name>`. You can stop it for whatever reason by running `docker stop <image_name>`
