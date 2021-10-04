# Local Development

## Overview

This document describes howto get a local development enviroment setup, so that a developer can make code changes, fix bugs add new features, test and debug etc for the ExternalDNS operator \
We will consider the following scenarios, working with CRC (code ready containers), Kind (kubernetes in Docker) and finally with any external Kubernetes/OpenShift cluster

Please refer to these links for further info regarding 
- CRC [link](https://developers.redhat.com/products/cdk/overview)
- Kind [link](https://kind.sigs.k8s.io/docs/user/quick-start/)
- External Kubernetes/OpenShift cluster


## CRC (Code Ready Containers)

This section assumes you have downloaded, installed and have a running instance of CRC \
We will address two modes of working with the ExternalDNS operator
- build and push to the internal CRC registry
- running the external-dns-operator locally

### Build and push to internal CRC registry

1. If you haven't done so create a fork of the openshift/external-dns-operator to your github account
2. Clone the repo from your github account to your local machine
3. Change directory `cd <projects-dir>/external-dns-operator`
4. Ensure your can access your CRC environment
   ```bash
   $ export KUBECONFIG=~/.crc/machines/crc/kubeconfig
   ```
5. Access to the CRC internal registry
   ```bash
   # assume that podman (tool used to create and maintain containers) is our default
   # assume our default user is kubeadmin
   $ podman login -u kubeadmin -p $(oc whoami -t) default-route-openshift-image-registry.apps-crc.testing --tls-verify=false
   ```
6. We can now follow the standard make recipes
   ```bash
   # create the external-dns-operator namespace
   $ oc create ns external-dns-operator
   # set the IMG envar before executing the `make deploy` command
   # url format is default-route-openshift-image-registry.apps-crc.testing/<namespace>/<image-name>:tag
   $ export IMG=default-route-openshift-image-registry.apps-crc.testing/external-dns-operator/external-dns-operator:dev
   $ CONTAINER_ENGINE=podman make image-build
   $ CONTAINER_ENGINE=popdman TLS_VERIFY=false make image-push
   # check that the image was pushed
   $ oc -n external-dns-operator get is
   ``` 
7. Now you can deploy an instance of ExternalDNS
   ```bash
   # deploy the operator
   $ make deploy
   # switch to the external-dns-namespace - so that we don't need to type out -n external-dns-operator all the time
   $ oc project external-dns-operator
   # check deployment
   $ oc get all
   # if there are errors/problems
   $ oc get events
   ```
8. Run the following command to create the credentials secret for AWS.\
   Ensure you have your AWS credentials set up in `~/.aws/credentials`.
   ```bash
   $ oc create secret generic aws-access-key \
   --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
   --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
   ```
   *Note*: other provider options can be found in `api/v1alpha1/externaldns_types.go`, e.g. the `ExternalDNSAWSProviderOptions` structure for AWS.

   Execute the following
   ```bash
   $ oc apply -k config/samples/aws
   # check for the newly created ExternalDNS
   $ oc get externaldns sample -o yaml  
   ``` 
   *Note*: other providers are available in `config/samples/`

### Running the external-dns-operator locally (with CRC)

1. Assume that the operator has been deployed (step 7 in the previous section)
2. Execute the following
   ```bash
   # scale the current deployed external-dns-operator to 0
   $ oc scale --replicas 0 -n external-dns-operator deployments external-dns-operator
   $ make run
   ```

### Working with external Kuberenets/OpenShift clusters

If you have any other cluster you'd like to test the operator on you just need to set KUBECONFIG and follow the instructions layed out in both sections indicated above
