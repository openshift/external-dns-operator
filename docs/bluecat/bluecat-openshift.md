# Manual test of BlueCat DNS provider on AWS
This document aims at describing the process of the manual testing of the BlueCat DNS provider for ExternalDNS Operator. The BlueCat products publicly available in AWS Marketplace were used.

- [Architecture](#architecture)
    - [Gateway and Workflows](#gateway-and-workflows)
    - [BlueCat Address Manager (BAM)](#bluecat-address-manager-bam)
    - [BlueCat DNS/DHCP Server (BDDS)](#bluecat-dnsdhcp-server-bdds)
- [Provision infrastructure](#provision-infrastructure)
    - [Prerequisites](#prerequisites)
    - [Provision manually, almost](#provision-manually-almost)
    - [Provision with terraform](#provision-with-terraform)
- [BAM configuration](#bam-configuration)
- [ExternalDNS Operator configuration](#externaldns-operator-configuration)
- [Testing](#testing)
- [Links](#links)

## Architecture
BlueCat has a lot of products and it's not always clear which ones are needed by ExternalDNS Operator.
The entry point into the BlueCat world is [the ExternalDNS prerequisites chapter](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/bluecat.md#prerequisites).

### Gateway and Workflows
[The BlueCat Gateway](https://docs.bluecatnetworks.com/r/en-US/Gateway-Administration-Guide/21.11.2) is a web utility which helps to automate DDI (DNS, DHCP, IPAM) operations. The BlueCat gateway is not a DNS server on its own, it's a proxy which talks to the other BlueCat products which in turn may be considered as being more low level. The BlueCat Gateway allows users to import the preconfigured DDI operations called workflows. The workflows automate the tedious configuration changes making the standard DNS operations easy.     
The ExternalDNS expects [the public community workflows](https://github.com/bluecatlabs/gateway-workflows/tree/master/Community) to be added to the gateway before the BlueCat provider can be used.    
The BlueCat Gateway is shipped as a public image from quay.io registry: `quay.io/bluecat/gateway`.

### BlueCat Address Manager (BAM)
[The BlueCat Address Manager](https://docs.bluecatnetworks.com/r/Address-Manager-Administration-Guide/Managing-servers/9.4.0) is the central product which serves the administrative purpose. It provides the central management of DNS, DHCP and IPAM services via configurations. BAM has the administration WebUI which is exposed on HTTP/HTTPs ports. The same ports are used for BAM API.       
The BlueCat Gateway talks to BAM API to do all the workflow operations. BAM then [deploys](https://docs.bluecatnetworks.com/r/Address-Manager-Administration-Guide/Managing-deployment/9.4.0) the new configurations to the corresponding servers: DNS, DHCP.

### BlueCat DNS/DHCP Server (BDDS)
And finally the DNS server itself. BAM can manage different DNS servers including, of course, BlueCat's own DNS/DHCP Server.
All the heavy lifting configuration (zones, records) is done by BAM, The BlueCat DNS server is here to serve DNS requests.

## Provision infrastructure

### Prerequisites
Make sure your AWS account is subscribed to the following products:
- [BlueCat Address Manager](https://aws.amazon.com/marketplace/pp/prodview-d5jopwyvyqfmu)
- [BlueCat DNS](https://aws.amazon.com/marketplace/pp/prodview-uon2romr42qxs)

### Provision manually, almost

- Create VPC with public subnet to host BlueCat VMs:
    ```sh
    aws cloudformation create-stack --stack-name bluecat-test --template-body file://${PWD}/scripts/aws-resources.yaml --parameters ParameterKey=EnvironmentName,ParameterValue=bluecat-test
    ```
- Launch BAM instance:
    - Go to `AWS Marketplace/Subscriptions` and click on `Launch instance`
    - Choose the machine type from the list of recommended (enabled) ones
    - Choose the previously created VPC and subnet
    - Add the storage: default (preconfigured) is fine
    - Add security group: default (preconfigured) is fine
    - Use existing or create a new keypair
    - Launch the instance and wait for all the status checks to pass: try to connect to `HTTPS` WebUI using `admin/<ec2-instance-id>` as credenetials
- Launch BlueCat DNS instance:
    - Go to `AWS Marketplace/Subscriptions` and click on `Launch instance`
    - Choose the machine type from the list of recommended (enabled) ones
    - Choose the previously created VPC and subnet
    - Add the storage: default (preconfigured) is fine
    - Add security group: default(preconfigured) is fine
    - Use existing or create a new keypair
    - Launch the instance, wait for all the status checks to pass: try to ssh to the instance public IP using the keypair's private key and `admin` user, it'll get you to the admin terminal of DNS server
- Launch BlueCat Gateway instance:
    - Launch any EC2 instance in the VPC and subnet created before
    - Add the following instructions to the user data for the EC2 instance:
        - [Podman installation instructions](https://podman.io/getting-started/installation#linux-distributions)
        - Gateway container with the community workflows:
        ```sh
        mkdir -p /opt/bluecat/data
        mkdir -p /opt/bluecat/logs
        mkdir -p /opt/bluecat/data/workflows/Community
        git clone https://github.com/bluecatlabs/gateway-workflows.git /opt/bluecat/workflows
        cp -R /opt/bluecat/workflows/Community/* /opt/bluecat/data/workflows/Community
        chmod -R 777 /opt/bluecat/
        podman run -d -p 80:8000 -p 443:44300 -v /opt/bluecat/data:/bluecat_gateway/ -v /opt/bluecat/logs:/logs/ -e BAM_IP=${BAM_IP} -e SESSION_COOKIE_SECURE=False quay.io/bluecat/gateway:latest
        ```
        _Note_: use `:Z` for the volumes if the EC2 instance is Red Hat based

### Provision with Terraform
- Follow [README from deploy directory](./deploy/README.md), the output will have all the needed data (BAM IP, BAM password, etc.)

## BAM configuration
_Note_: All the configuration must be done in BAM WebUI

- It's highly recommeded to change the default (EC2 instance ID) password for `admin` user:
    - Top right corner, user picture
    - Profile menu
    - Choose `admin` user, `Change password` menu
    - Type the old and the new passwords
- By default no configuration exists in BAM, so you need to create a new one:
    - `Administration` tab, `Configurations`
    - `Add new configuration` button
    - Put a name and click on `Add`
- With the new configuration DNS,IPAM and other tabs got unlocked. Now you are ready to add the DNS server whose instance was created before:
    - Follow [this doc](https://docs.bluecatnetworks.com/r/Address-Manager-Administration-Guide/Adding-DNS/DHCP-Servers-to-Address-Manager/9.3.0). **Failed to add the DNS server as it needed a password which I was unable to reset using the admin terminal because of the missing BlueCat license(activation key)**.
- Gateway needs some BAM configuration to have user/password which can be used in the gateway authentication page:
    - Create user defined field (UDF) as described [here](https://docs.bluecatnetworks.com/r/Gateway-Installation-Guide/Creating-BlueCatGateway-UDF/21.5.1).
    - Create gateway user as described [here](https://docs.bluecatnetworks.com/r/Gateway-Installation-Guide/Creating-a-BlueCat-Gateway-user-in-Address-Manager/21.5.1).
- Follow [these instructions](https://docs.bluecatnetworks.com/r/Address-Manager-Administration-Guide/Adding-DNS-zones/9.4.0) to create a DNS zone
    - Use your OpenShift cluster's default router's domain name for the top and sub domains
    - Make sure the zone is `Deployable`
    - Add `Allow Dynamic Updates` DNS deployment options (`Deployment options` Configuration tab)

## ExternalDNS Operator configuration
- Follow [BlueCat usage instructions](https://github.com/openshift/external-dns-operator/blob/main/docs/usage.md#bluecat) to create a secret before the operator deployment. Note that you will need some things created before in this document:
    - IP/DNS of the host where the gateway is running
    - Gateway username and password created in BAM
    - DNS configuration is the BAM configuration which was newly created in this document
    - Root zone is the domain of the DNS zone created in BAM
- Deploy ExternalDNS Operator following the instrcutions from [README](../../README.md)

## Testing
- Create a [ExternalDNS CR](../../config/samples/bluecat/operator_v1alpha1_externaldns_detailed.yaml):
    ```sh
    cat <<EOF | oc create -f -
    apiVersion: externaldns.olm.openshift.io/v1alpha1
    kind: ExternalDNS
    metadata:
    name: sample-bluecat
    spec:
    domains:
        - filterType: Include
        matchType: Exact
        name: mybluecatrootzone.com
    provider:
        type: BlueCat
    source:
        type: OpenShiftRoute
        openshiftRouteOptions:
        routerName: default
        labelFilter:
        matchLabels:
            app: hello-openshift
    EOF
    ```
- Create the source for the DNS records:
    ```sh
    oc new-app --docker-image=openshift/hello-openshift -l app=hello-openshift
    oc expose service/hello-openshift -l app=hello-openshift
    ```
- Check the record for `hello-openshift` route:
    ```sh
    dig $(oc get route hello-openshift --template='{{range .status.ingress}}{{if eq "default" .routerName}}{{.host}}{{end}}{{end}}') @${PUBLIC_IP_BDDS}
    ```

## Links
- [BlueCat AWS Virtual Appliances guide](https://docs.bluecatnetworks.com/r/BlueCat-AWS-Virtual-Appliances/Requirements/9.4.0)
