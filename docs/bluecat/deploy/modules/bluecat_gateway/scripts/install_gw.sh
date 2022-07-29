#!/bin/bash

while getopts "b:u:" opt; do
  case ${opt} in
    b )
      BAM_IP=$OPTARG;;
    u )
      USER=$OPTARG;;
    * )
      exit;;
  esac
done
shift $((OPTIND -1))

echo "===> INSTALLING PREREQUISITES <==="
sudo apt-get update -q=2 >/dev/null 2>&1
sudo apt-get install -q=2 apt-transport-https ca-certificates curl gnupg-agent software-properties-common >/dev/null 2>&1
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -

echo "===> INSTALLING DOCKER CE <==="
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"
sudo apt-get update -q=2 >/dev/null 2>&1
sudo apt-get install -q=2 docker-ce docker-ce-cli containerd.io >/dev/null 2>&1

sudo usermod -a -G docker $USER

sudo mkdir -p /opt/bluecat/logs
sudo mkdir -p /opt/bluecat/data
sudo chmod -R 777 /opt/bluecat/
