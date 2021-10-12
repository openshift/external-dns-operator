# Overview

This document describes how to set up a local development environment to work with the `ExternalDNS` Operator.
In this document, you can find information about working with the `ExternalDNS` Operator in CRC (code-ready containers), Kind (Kubernetes in Docker), and external clusters - Kubernetes or an OpenShift cluster.
## Working with the `ExternalDNS` Operator in CRC
Prerequisites:
- You have a running instance of CRC.

CRC is a pre-built Container Development Environment based on Red Hat Enterprise Linux which helps you develop container-based applications quickly. You can work with the `ExternalDNS` Operator in CRC using either of the following two methods:
- Build and push to the internal CRC registry.
- Run the `ExternalDNS` Operator locally.

### Building and pushing to the internal CRC registry
1. Create a fork of the `openshift/external-dns-operator` repository in your GitHub account.
2. Clone the `openshift/external-dns-operator` repository from your GitHub account to your local machine.
3. Navigate to the downloaded `external-dns-operator` repository:
    ``` bash
    $ cd <projects-dir>/external-dns-operator
    ```
    `<projects-dir>` is the directory where you downloaded the  `external-dns-operator` repository.
4. Verify that you can access your CRC environment:
    ```bash
    $ export KUBECONFIG=~/.crc/machines/crc/kubeconfig
    ```
5. Access the CRC internal registry as the `kubeadmin` user, using the podman tool:
    ```bash
    $ podman login -u kubeadmin -p $(oc whoami -t)    default-route-openshift-image-registry.apps-crc.testing --tls-verify=false
    ```
6. Follow the standard make recipes:
- Create the `external-dns-operator` namespace:
    ```bash
    $ oc create ns external-dns-operator
    ```
- Set the `IMG` EnVar to the following URL:
`default-route-openshift-image-registry.apps-crc.testing/<namespace>/<image-name>:tag`
- Run the following commands to set the `IMG` EnVar:
    ```bash
    $ export IMG=default-route-openshift-image-registry.apps-crc.testing/external-dns-operator/external-dns-operator:dev
    $ CONTAINER_ENGINE=podman make image-build
    $ CONTAINER_ENGINE=podman TLS_VERIFY=false make image-push
    ```
- Verify that the image is pushed:
    ```bash
    $ oc -n external-dns-operator get is
    ```
7. Deploy an instance of ExternalDNS:
- Deploy the `ExternalDNS` Operator:
    ```bash
    $ make deploy
    ```
- Switch to the `external-dns-operator`namespace:
    ```bash
    $ oc project external-dns-operator
    ```
- Verify that the `ExternalDNS` Operator is deployed:
    ```bash
    $ oc get all
    ```
- If you encounter any errors, run the following command:
    ```bash
    $ oc get events
    ```
8. Create the `credentials` secret for AWS:
Note: Ensure you have your AWS credentials set up in  `~/.aws/credentials`. For other providers, see `api/v1alpha1/externaldns_types.go`.
Examples:
`ExternalDNSAzureProviderOptions` structure for Azure
`ExternalDNSGCPProviderOptions`  structure for GCP
    ```bash
    $ oc create secret generic aws-access-key \
    --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
    --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
    ```
9.  Run the following commands:
    ```bash
    $ oc apply -k config/samples/aws
    $ oc get externaldns sample -o yaml
    ```
 Note: `config/samples/aws` is the sample configuration file for AWS. For other providers, see other samples in `config/samples/`


### Running the `ExternalDNS` Operator locally
1. Deploy the `ExternalDNS` Operator. To know how to deploy, see step 7 in “**Building and pushing to the internal CRC registry**”.
2. Run the following commands:
    ```bash
    $ oc scale --replicas 0 -n external-dns-operator deployments external-dns-operator
    $ make run
    ```
## Deploying the `ExternalDNS` Operator in Kind (Kubernetes in Docker)
This section provides instructions for deploying  `external-dns-operator` in a `Kind` environment.
Prerequisites
- [Go](https://golang.org/doc/install#install) and [Docker](https://docs.docker.com/engine/install/) are installed.
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) is installed and a local Kubernetes [cluster](https://kind.sigs.k8s.io/docs/user/quick-start/#creating-a-cluster) is created.
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) is installed and has an access to the above created cluster. To know how to configure `kubectl` to access the created cluster, see “**Configuring kubectl to access a cluster**”.
- Any registry that is accessible from your cluster to store and distribute the docker images (quay.io is shown as an example in the following steps).

### Build and push the docker image:
1. Clone the `external-dns-operator` repository from your GitHub account to your local machine.
2. Navigate to the downloaded `external-dns-operator` repository:
    ``` bash
    $ cd <projects-dir>/external-dns-operator
    ```
    `<projects-dir>` is the directory where you downloaded the  `external-dns-operator` repository.
3. Run `docker login` and provide your login  credentials for quay.io to access the registry.
4. Build and push the `external-dns-operator` docker image by providing the relevant registry name and tag:
    ```bash
    $ export IMG=quay.io/<username>/external-dns-operator:latest
    $ make image-build
    $ make image-push
    ```
### Deploy `external-dns-operator`:
- Run the following command:
    ```bash
    $ make deploy
    ```
- List the pods deployed in the `external-dns-operator` namespace:
    ```bash
    $ kubectl get pods -n external-dns-operator
    ```
**Note**: You can deploy an `ExternalDNS` instance based on the provider. A sample AWS configuration file is provided in the `config/samples/aws` folder. Make sure a secret named `aws-access-key` is created in the `external-dns-operator` namespace before applying this configuration.

### Configuring `kubectl` to access a cluster
- Get the name of the cluster created in `kind`:
    ```bash
    $ kind get clusters
    # Sample output:
    kind
    ```
- Set the kubectl context to point to the cluster:
    ```bash
    $ kubectl config use-context kind-kind
    ```
    *Note*: "**kind-**" prefix is appended to the cluster name.

## Working with the `ExternalDNS` Operator in external clusters
To test the `ExternalDNS` Operator on an external cluster (OpenShift or Kubernetes), set `KUBECONFIG` and follow procedures mentioned in “Working with the `ExternalDNS` Operator in CRC”.
