# Usage

- [AWS](#aws)
    - [Assume Role](#assume-role)
    - [GovCloud Regions](#govcloud-regions)
    - [STS Clusters](#sts-clusters)
- [Infoblox](#infoblox)
- [BlueCat](#bluecat)
- [GCP](#gcp)
- [Azure](#azure)

### Credentials for DNS providers

The _external-dns-operator_ manages external-dns deployments. It creates pods with correct credentials based on the
provider configuration in the `ExternalDNS` resource. However, it does not provision the credentials themselves, instead
it expects the credentials to be in the same namespace as the operator itself. It then copies over the credentials into
the namespace where the _external-dns_ deployments are created so that they can be mounted by the pods.

# AWS

1. Create a secret with the access key id and secret:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
    name: aws-access-key
    namespace: external-dns-operator
    stringData:
    credentials: |-
        [default]
        aws_access_key_id = " <AWS_ACCESS_KEY_ID>"
        aws_secret_access_key = "<AWS_SECRET_ACCESS_KEY>"
    ```

2. Create an `ExternalDNS` resource as follows:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: aws-example
    spec:
    provider:
        type: AWS
        aws:
        credentials:
            name: aws-access-key
    zones: # Replace with the desired hosted zone IDs
        - "Z3URY6TWQ91KXX"
    source:
        type: Service
        fqdnTemplate:
        - '{{.Name}}.mydomain.net'
    ```

Once this is created the _external-dns-operator_ will create a deployment of _external-dns_ which is configured to
manage DNS records in AWS Route53.

## Assume Role

The _external-dns-operator_ supports managing records in another AWS account's hosted zone. To achieve this, you will
require an IAM Role ARN with the necessary permissions properly set up. This Role ARN should then be specified in the
`ExternalDNS` resource in the following manner:

```yaml
apiVersion: externaldns.olm.openshift.io/v1beta1
kind: ExternalDNS
metadata:
  name: aws-example
spec:
  provider:
    type: AWS
    aws:
      credentials:
        name: aws-access-key
      assumeRole:
        arn: arn:aws:iam::123456789012:role/role-name # Replace with the desire Role ARN
  zones: # Replace with the desired hosted zone IDs
    - "Z3URY6TWQ91KXX"
  source:
    type: Service
    fqdnTemplate:
    - '{{.Name}}.mydomain.net'
```

## GovCloud Regions
The operator makes the assumption that `ExternalDNS` instances which target GovCloud DNS also run on the GovCloud. This is needed to detect the AWS region.
As for the rest: the usage is exactly the same as for [AWS](#aws).

## STS Clusters

1. Generate the trusted policy file using your identity provider:

    ```bash
    IDP="<my-oidc-provider-name>"
    ACCOUNT="<my-aws-account>"
    IDP_ARN="arn:aws:iam::${ACCOUNT}:oidc-provider/${IDP}"
    EXTERNAL_DNS_NAME="<my-external-dns-instance-name>"
    cat <<EOF > external-dns-trusted-policy.json
    {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": {
                    "Federated": "${IDP_ARN}"
                },
                "Action": "sts:AssumeRoleWithWebIdentity",
                "Condition": {
                    "StringEquals": {
                        "${IDP}:sub": "system:serviceaccount:external-dns-operator:external-dns-${EXTERNAL_DNS_NAME}"
                    }
                }
            }
        ]
    }
    EOF
    ```

2. Create and verify the role with the generated trusted policy:

    ```bash
    aws iam create-role --role-name external-dns --assume-role-policy-document file://external-dns-trusted-policy.json
    EXTERNAL_DNS_ROLEARN=$(aws iam get-role --role-name external-dns --output=text | grep '^ROLE' | grep -Po 'arn:aws:iam[0-9a-z/:\-_]+')
    echo $EXTERNAL_DNS_ROLEARN
    ```

3. Attach the permission policy to the role:

    ```bash
    curl -o external-dns-permission-policy.json https://raw.githubusercontent.com/openshift/external-dns-operator/main/assets/iam-policy.json
    aws iam put-role-policy --role-name external-dns --policy-name perms-policy-external-dns --policy-document file://external-dns-permission-policy.json
    ```

4. Create a secret with the role:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
    name: aws-sts-creds
    namespace: external-dns-operator
    stringData:
    credentials: |-
        [default]
        sts_regional_endpoints = regional
        role_arn = ${EXTERNAL_DNS_ROLEARN}
        web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
    ```

5. Create an `ExternalDNS` resource as follows:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: ${EXTERNAL_DNS_NAME}
    spec:
    provider:
        type: AWS
        aws:
        credentials:
            name: aws-sts-creds
    zones: # Replace with the desired hosted zone IDs
        - "Z3URY6TWQ91KXX"
    source:
        type: Service
        fqdnTemplate:
        - '{{.Name}}.mydomain.net'
    ```

# Infoblox

Before creating an `ExternalDNS` resource for the [Infoblox](https://www.infoblox.com/wp-content/uploads/infoblox-deployment-infoblox-rest-api.pdf)
the following information is required:

- Grid Master Host
- WAPI version
- WAPI port
- WAPI username
- WAPI password

1. Create a secret with the username and password as follows:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
    name: infoblox-credentials
    namespace: #operator namespace
    data:
    EXTERNAL_DNS_INFOBLOX_WAPI_USERNAME: # Base-64 encoded username
    EXTERNAL_DNS_INFOBLOX_WAPI_PASSWORD: # Base-64 encoded password
    ```

2. Create an `ExternalDNS` resource as follows:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: infoblox-example
    spec:
    provider:
        type: Infoblox
        infoblox:
        credentials:
            name: infoblox-credentials
        gridHost: # the grid master host from the previous step. eg: 172.26.1.200
        wapiPort: # the WAPI port, eg: 80, 443, 8080
        wapiVersion: # the WAPI version, eg: 2.11, 2.3.1
    zones: # Replace with the desired hosted zones
        - "ZG5zLm5ldHdvcmtfdmlldyQw"
    source:
        type: Service
        fqdnTemplate:
        - '{{.Name}}.mydomain.net'
    ```

Once this is created the _external-dns-operator_ will create a deployment of _external-dns_ which is configured to
manage DNS records in Infoblox.

# BlueCat

The BlueCat provider requires
the [BlueCat Gateway](https://docs.bluecatnetworks.com/r/Gateway-Installation-Guide/Installing-BlueCat-Gateway/20.3.1)
and the [community workflows](https://github.com/bluecatlabs/gateway-workflows) to be installed. Once the gateway is
running note down the following details:

- Gateway Host
- Gateway Username(optional)
- Gateway Password(optional)
- Root Zone

1. Create a JSON file with the details:

    ```json
    {
    "gatewayHost": "https://bluecatgw.example.com",
    "gatewayUsername": "user",
    "gatewayPassword": "pass",
    "dnsConfiguration": "Example",
    "dnsView": "Internal",
    "rootZone": "example.com",
    "skipTLSVerify": false
    }
    ```

2. Create a secret in the operator namespace with the command:

    ```bash
    kubectl create secret -n $EXTERNAL_DNS_OPERATOR_NAMESPACE generic bluecat-config --from-file ~/bluecat.json
    ```

For more details consult the
external-dns [documentation for BlueCat](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/bluecat.md)
.

3. Create an `ExternalDNS` resource as shown below:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: bluecat-example
    spec:
    provider:
        type: BlueCat
        blueCat:
        config:
            name: bluecat-config
    zones: # Replace with the desired hosted zones
        - "78127234..."
    ```

# GCP

Before creating an ExternalDNS resource for GCP, the following is required:

1. Create a secret with the service account credentials to be used by the operator:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
        name: gcp-access-key
        namespace: #operator namespace
    data:
        gcp-credentials.json: # gcp-service-account-key-file
    ```

2. Create an `ExternalDNS` CR as follows:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: sample-gcp
    spec:
    # DNS provider
    provider:
        type: GCP
        gcp:
        credentials:
            name: gcp-access-key
        project: gcp-devel
    zones: # Replace with the desired managed zones
        - "3651032588905568971"
    source:
        type: Service
        fqdnTemplate:
        - '{{.Name}}.mydomain.net'
    ```

# Azure

Before creating an ExternalDNS resource for Azure, the following is required:

1. Create a secret with the service account credentials to be used by the operator:

    ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
        name: azure-config-file
        namespace: #operator namespace
    data:
        azure.json: # azure-config-file
    ```

    The contents of `azure.json` should be similar to this:

    ```json
    {
    "tenantId": "01234abc-de56-ff78-abc1-234567890def",
    "subscriptionId": "01234abc-de56-ff78-abc1-234567890def",
    "resourceGroup": "MyDnsResourceGroup",
    "aadClientId": "01234abc-de56-ff78-abc1-234567890def",
    "aadClientSecret": "<clientSecret>"
    }
    ```

2. Create an `ExternalDNS` CR as follows:

    ```yaml
    apiVersion: externaldns.olm.openshift.io/v1beta1
    kind: ExternalDNS
    metadata:
    name: sample-azure
    spec:
    # DNS provider
    provider:
        type: Azure
        azure:
        configFile:
            name: azure-config-file
    zones: # Replace with the desired hosted zones
        - "myzoneid"
    source:
        type: Service
        fqdnTemplate:
        - '{{.Name}}.mydomain.net'
    ```
