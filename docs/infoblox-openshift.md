# Use ExternalDNS Operator on Openshift with Infoblox provider

- [Steps](#steps)
- [Provision Infoblox on AWS](#provision-infoblox-on-aws)
    - [Prerequisites](#prerequisites)
    - [Provision manually](#provision-manually)
    - [Infoblox configuration](#infoblox-configuration)
- [Links](#links)

## Steps

**Note**: This guide assumes that Infoblox is setup and ready to be used for the DNS queries. You can follow the instructions from [Provision Infoblox on AWS chapter](#provision-infoblox-on-aws) if you want to setup Infoblox on AWS using the marketplace products. ExternalDNS Operator though can be deployed on any environment (AWS, Azure, GCP, locally)

1. _Optional_: In case Infoblox uses a self signed certificate, add its CA as trusted to ExternalDNS Operator:
```sh
oc -n external-dns-operator create configmap trusted-ca-infoblox --from-file=ca-bundle.crt=/path/to/pem/encoded/infoblox/ca
oc -n external-dns-operator patch subscription external-dns-operator --type='json' -p='[{"op": "add", "path": "/spec/config", "value":{"env":[{"name":"TRUSTED_CA_CONFIGMAP_NAME","value":"trusted-ca-infoblox"}]}}]'
```

2. Create a secret with Infoblox credentials:
```sh
oc -n external-dns-operator create secret generic infoblox-credentials --from-literal=EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME=${INFOBLOX_USERNAME} --from-literal=EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD=${INFOBLOX_PASSWORD}
```

3. Get the routes to check your cluster's domain (everything after `apps.`):
```sh
$ oc get routes --all-namespaces | grep console
openshift-console          console             console-openshift-console.apps.aws.devcluster.openshift.com                       console             https   reencrypt/Redirect     None
openshift-console          downloads           downloads-openshift-console.apps.aws.devcluster.openshift.com                     downloads           http    edge/Redirect          None
```

4. Add an authoritative DNS zone for your cluster's domain to Infoblox using its WebUI:
    - `Data Management` top tab -> `DNS` subtab -> `Add` right panel -> `Zone` -> `Authoritative Zone` -> `Forward mapping`
    - Put cluster's domain as zone name (e.g. `aws.devcluster.openshift.com`)
    - Add Grid Primary as nameserver (`+` button on top of the table)
    - `Save & Close`

5. Create a [ExternalDNS CR](../../config/samples/infoblox/operator_v1beta1_infoblox_openshift.yaml) as follows:
    ```sh
    cat <<EOF | oc create -f -
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
      name: sample-infoblox
    spec:
      provider:
        type: Infoblox
        infoblox:
          credentials:
            name: infoblox-credentials
          gridHost: ${INFOBLOX_GRID_PUBLIC_IP}
          wapiPort: 443
          wapiVersion: "2.12.2"
      domains:
      - filterType: Include
        matchType: Exact
        name: aws.devcluster.openshift.com
      source:
        type: OpenShiftRoute
        openshiftRouteOptions:
          routerName: default
    EOF
    ```

6. Check the records created for `console` routes:
    ```sh
    $ dig @${INFOBLOX_GRID_PUBLIC_IP} $(oc -n openshift-console get route console --template='{{range .status.ingress}}{{if eq "default" .routerName}}{{.host}}{{end}}{{end}}') +short
    router-default.apps.aws.devcluster.openshift.com
    $ dig @${INFOBLOX_GRID_PUBLIC_IP} $(oc -n openshift-console get route downloads --template='{{range .status.ingress}}{{if eq "default" .routerName}}{{.host}}{{end}}{{end}}') +short
    router-default.apps.aws.devcluster.openshift.com
    ```

## Provision Infoblox on AWS

### Prerequisites
Make sure your AWS account is subscribed to the following product:
- [Infoblox vNIOS for DDI](https://aws.amazon.com/marketplace/pp/prodview-opxe3p2cgudwe)

### Provision manually
**Note**: all the steps described in this chapter are well detailed in Infoblox guide which you can find in the [links](#links)

- Create VPC, public subnets and NIOS VM:
    ```sh
    export AWS_PROFILE=myprofile
    # EC2 -> Network & Security -> Key Pairs
    export AWS_KEYPAIR=mykey
    export GRID_ADMIN_PASSWORD=MyComplexPassword
    aws cloudformation create-stack --stack-name infoblox --template-body file://${PWD}/scripts/cloud-formation-infoblox.yaml --parameters ParameterKey=EnvironmentName,ParameterValue=infoblox ParameterKey=NiosKeyPair,ParameterValue=${AWS_KEYPAIR} ParameterKey=GridAdminPassword,ParameterValue=${GRID_ADMIN_PASSWORD}
    ```
- You can always check the status and the output of CloudFormation script:
    ```sh
    aws cloudformation describe-stacks --stack-name infoblox
    ```
    _Note_: NIOS instance takes around 10 minutes to start
- Once the EC2 instance passed all the checks you can try to connect to the Grid Manager WebUI using the Elastic IP and `admin/${GRID_ADMIN_PASSWORD}` as credentials. Use HTTPS scheme and accept the self-signed certificate.
- You may want to set up DNS for the newly created public IP. You will need the FQDN later during the setup.

### Infoblox configuration
- Setup a new grid. At the first start a wizard would pop up and propose to do so, you can follow `Use vNIOS Instance for New grid` chapter from the guide in the [links](#links) or follow these notes:
    - Most of the values can be kept as set by default except for these:
        - Set the shared secret to the admin password (note that it can be changed later in the setup process)
        - Set the hostname to the FQDN of your vNIOS instance, if you have one
        - Admin password is better to be reset if you plan to use this Infoblox for a long time
        - Enable NTP if you plan to use this Infoblox for a long time
    - Restart will be needed at the end of the setup of the new grid
- Add name server group with the grid server: `Data Management` top tab -> `DNS` subtab -> `Add` dropdown button on the right panel -> `Group` -> `Authorative` -> Put a name (e.g. `infoblox`) -> `+` button -> `Add Grid Primary` pannel -> `Select` button -> `Add` button -> `Save & Close`
- Start DNS service: `Grid` top tab -> `Grid manager` subtab -> `DNS` check button -> `Services` subsubtab -> Select `infoblox.localdomain` (or your instance's FQDN) -> Button `Play` (Start) -> Confirm the start
- The default self-signed certificate uses the private IP. Therefore, you will need to regenerate it with the Elastic IP (or FQDN if any):
    - `Grid` top tab -> `Grid Manager` subtab -> `DNS` check button -> `Members` subsubtab -> Click on your server group -> `Certificates` dropdown button on the right panel -> `HTTPS Cert` -> `Generate Self Signed Certificate` -> Fill `Days Valid` -> Add Elastic IP (or FQDN if any) in `Subject Alternative name` -> Accept the restart of the service -> Close the popup window

### Set licenses

You can set new licenses using the Infoblox client which is spawned when you `ssh` into the instance:
```bash
ssh -i key-from-my-keypair admin@myinfobloxinstance
Infoblox > set license
```

**Notes**
- Licenses are generated for a given hardware ID (HWID), so each new EC2 instance of the Infoblox appliance requires new licenses.
- Each license needs to be set separately, so you will have to run `set license` command for each type (NIOS, DNS, Grid, etc.).


## Links
- [Deploy Infoblox vNIOS instances for AWS](https://www.infoblox.com/wp-content/uploads/infoblox-deployment-guide-deploy-infoblox-vnios-instances-for-aws.pdf)
- [Grid Manager. Managing certificates](https://docs.infoblox.com/display/NAG8/Managing+Certificates)
