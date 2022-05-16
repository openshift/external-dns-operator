# ExternalDNS Operator

The `ExternalDNS` Operator allows you to deploy and manage [ExternalDNS](https://github.com/kubernetes-sigs/external-dns), a cluster-internal component which makes Kubernetes resources discoverable through public DNS servers. For more information about the initial motivation, see [ExternalDNS Operator OpenShift Enhancement Proposal](https://github.com/openshift/enhancements/pull/786).

- [Deploying the ExternalDNS Operator](#deploying-the-externaldns-operator)
    - [Preparing the environment](#preparing-the-environment)
    - [Installing the ExternalDNS Operator by building and pushing the Operator image to a registry](#installing-the-externaldns-operator-by-building-and-pushing-the-operator-image-to-a-registry)
    - [Installing the ExternalDNS Operator using a custom index image on OperatorHub](#installing-the-externaldns-operator-using-a-custom-index-image-on-operatorhub)
- [Using custom operand image](#using-custom-operand-image)
- [Running end-to-end tests manually](#running-end-to-end-tests-manually)
- [Proxy support](#proxy-support)
- [Metrics](#metrics)

## Deploying the `ExternalDNS` Operator
The following procedure describes how to deploy the `ExternalDNS` Operator for AWS.     

### Preparing the environment
Prepare your environment for the installation commands.

- Select the container runtime you want to build the images with (`podman` or `docker`):
    ```sh
    export CONTAINER_ENGINE=podman
    ```
- Select the name settings of the image:
    ```sh
    export REGISTRY=quay.io
    export REPOSITORY=myuser
    export VERSION=1.0.0
    ```
- Login to the image registry:
    ```sh
    ${CONTAINER_ENGINE} login ${REGISTRY} -u ${REPOSITORY}
    ```

### Installing the `ExternalDNS` Operator by building and pushing the Operator image to a registry
1. Build and push the Operator image to a registry:
   ```sh
   export IMG=${REGISTRY}/${REPOSITORY}/external-dns-operator:${VERSION}
   make image-build image-push
   ```

2. _Optional_: you may need to link the registry secret to `external-dns-operator` service account if the image is not public ([Doc link](https://docs.openshift.com/container-platform/4.10/openshift_images/managing_images/using-image-pull-secrets.html#images-allow-pods-to-reference-images-from-secure-registries_using-image-pull-secrets)):

    a. Create a secret with authentication details of your image registry:
    ```sh
    oc -n external-dns-operator create secret generic extdns-pull-secret  --type=kubernetes.io/dockercfg  --from-file=.dockercfg=${XDG_RUNTIME_DIR}/containers/auth.json
    ```
    b. Link the secret to `external-dns-operator` service account:
    ```sh
    oc -n external-dns-operator secrets link external-dns-operator extdns-pull-secret --for=pull
    ````

3. Run the following command to deploy the `ExternalDNS` Operator:
    ```sh
    make deploy
    ```

4. The previous step deploys the validation webhook, which requires TLS authentication for the webhook server. The
   manifests deployed through the `make deploy` command do not contain a valid certificate and key. You must provision a valid certificate and key through other tools.
   You can use a convenience script, `hack/generate-certs.sh` to generate the certificate bundle and patch the validation webhook config.   
   _Important_: Do not use the hack/generate-certs.sh script in a production environment.   
   Run the `hack/generate-certs.sh` script with the following inputs:
   ```sh
   hack/generate-certs.sh --service webhook-service --webhook validating-webhook-configuration \
   --secret webhook-server-cert --namespace external-dns-operator
   ```
   *Note*: you may need to wait for the retry of the volume mount in the operator's POD

5. Now you can deploy an instance of ExternalDNS:
    * Run the following command to create the credentials secret for AWS:
        ```sh
        kubectl -n external-dns-operator create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
       *Note*: See [this guide](./docs/usage.md) for instructions specific to other providers.
      
    * Run the following command:
      ```sh
      # for AWS
      kubectl apply -k config/samples/aws
      ```
       *Note*: For other providers, see `config/samples/`.


### Installing the `ExternalDNS` Operator using a custom index image on OperatorHub
1. Build and push the operator image to the registry:
    ```sh
    export IMG=${REGISTRY}/${REPOSITORY}/external-dns-operator:${VERSION}
    make image-build image-push
    ```

2. Build and push the bundle image to the registry:
    ```sh
    export BUNDLE_IMG=${REGISTRY}/${REPOSITORY}/external-dns-operator-bundle:${VERSION}
    make bundle-image-build bundle-image-push
    ```

3. Build and push the index image to the registry:
   ```sh
   export INDEX_IMG=${REGISTRY}/${REPOSITORY}/external-dns-operator-bundle-index:${VERSION}
   make index-image-build index-image-push
   ```

4. _Optional_: you may need to link the registry secret to the pod of `external-dns-operator` created in the `openshift-marketplace` namespace if the image is not made public ([Doc link](https://docs.openshift.com/container-platform/4.10/openshift_images/managing_images/using-image-pull-secrets.html#images-allow-pods-to-reference-images-from-secure-registries_using-image-pull-secrets)). If you are using `podman` then these are the instructions:

    a. Create a secret with authentication details of your image registry:
    ```sh
    oc -n openshift-marketplace create secret generic extdns-olm-secret  --type=kubernetes.io/dockercfg  --from-file=.dockercfg=${XDG_RUNTIME_DIR}/containers/auth.json
    ```
    b. Link the secret to `default` service account:
    ```sh
    oc -n openshift-marketplace secrets link default extdns-olm-secret --for=pull
    ```

5. Create the `CatalogSource` object:
   ```sh
   cat <<EOF | oc apply -f -
   apiVersion: operators.coreos.com/v1alpha1
   kind: CatalogSource
   metadata:
     name: external-dns-operator
     namespace: openshift-marketplace
   spec:
     sourceType: grpc
     image: ${INDEX_IMG}
   EOF
   ```

6. Create the operator namespace:
    ```sh
    oc create namespace external-dns-operator
    oc label namespace external-dns-operator openshift.io/cluster-monitoring=true
    ```

7. Create the `OperatorGroup` object to scope the operator to `external-dns-operator` namespace:
    ```sh
    cat <<EOF | oc apply -f -
    apiVersion: operators.coreos.com/v1
    kind: OperatorGroup
    metadata:
      name: external-dns-operator
      namespace: external-dns-operator
    spec:
      targetNamespaces:
      - external-dns-operator
    EOF
    ```

8. Create the `Subscription` object:
    ```sh
    cat <<EOF | oc apply -f -
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
      name: external-dns-operator
      namespace: external-dns-operator
    spec:
      channel: stable-v1.0
      name: external-dns-operator
      source: external-dns-operator
      sourceNamespace: openshift-marketplace
    EOF
    ```
    *Note*: The steps starting from the 7th can be replaced with the following actions in the web console: Navigate to  `Operators` -> `OperatorHub`, search for the `ExternalDNS Operator`,  and install it in the `external-dns-operator` namespace.

10. Now you can deploy an instance of ExternalDNS:
    ```sh
    # for AWS
    oc apply -k config/samples/aws
    ```
    *Note*: For other providers, see `config/samples/`.

## Using custom operand image
1. _Optional_: you may need to link the registry secret to the operand's service account if your custom image is not public ([Doc link](https://docs.openshift.com/container-platform/4.10/openshift_images/managing_images/using-image-pull-secrets.html#images-allow-pods-to-reference-images-from-secure-registries_using-image-pull-secrets)):

    a. Create a secret with authentication details of your image registry:
    ```sh
    oc -n external-dns-operator create secret generic extdns-pull-secret --type=kubernetes.io/dockercfg --from-file=.dockercfg=${XDG_RUNTIME_DIR}/containers/auth.json
    ```
    b. Find the service account of your operand:
    ```sh
    oc -n external-dns-operator get sa | grep external-dns
    ```
    c. Link the secret to found service account:
    ```sh
    oc -n external-dns-operator secrets link external-dns-sample-aws extdns-pull-secret --for=pull
    ```

2. Patch `RELATED_IMAGE_EXTERNAL_DNS` environment variable's value with your custom operand image:
    - In the operator's deployment:
    ```sh
    # "external-dns-operator" container has index 0
    # "RELATED_IMAGE_EXTERNAL_DNS" environment variable has index 1
    oc -n external-dns-operator patch deployment external-dns-operator --type='json' -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/env/1/value", "value":"<CUSTOM_IMAGE_TAG>"}]'
    ```
    - Or in the operator's subscription:
    ```sh
    oc -n external-dns-operator patch subscription external-dns-operator --type='json' -p='[{"op": "add", "path": "/spec/config", "value":{"env":[{"name":"RELATED_IMAGE_EXTERNAL_DNS","value":"<CUSTOM_IMAGE_TAG>"}]}}]'
    ```

## Running end-to-end tests manually

1. Deploy the operator as described above

2. Set the necessary environment variables

   For AWS:
   ```sh
   export KUBECONFIG=/path/to/mycluster/kubeconfig
   export DNS_PROVIDER=AWS
   export AWS_ACCESS_KEY_ID=my-aws-access-key
   export AWS_SECRET_ACCESS_KEY=my-aws-access-secret
   ```
   For Infoblox:
   ```sh
   export KUBECONFIG=/path/to/mycluster/kubeconfig
   export DNS_PROVIDER=INFOBLOX
   export INFOBLOX_GRID_HOST=myinfoblox.myorg.com
   export INFOBLOX_WAPI_USERNAME=my-infoblox-username
   export INFOBLOX_WAPI_PASSWORD=my-infoblox-password
   ```
   For the other providers: check out [e2e directory](./test/e2e/).

3. Run the test suite
   ```sh
   make test-e2e
   ```

## Proxy support

[Configuring proxy support for ExternalDNS Operator](./docs/proxy.md)

## Metrics

The ExternalDNS Operator exposes [controller-runtime metrics](https://book.kubebuilder.io/reference/metrics.html) in the format expected by [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator).
`ServiceMonitor` object is created in the operator's namespace (by default `external-dns-operator`), make sure that your instance of the Prometheus Operator is properly configured to find it.     
You can check `.spec.serviceMonitorNamespaceSelector` and `.spec.serviceMonitorSelector` fields of `prometheus` resource and edit the operator's namespace or service monitor accordingly:
```sh
kubectl -n monitoring get prometheus k8s --template='{{.spec.serviceMonitorNamespaceSelector}}{{"\n"}}{{.spec.serviceMonitorSelector}}{{"\n"}}'
map[matchLabels:map[openshift.io/cluster-monitoring:true]]
map[]
```
For OpenShift:
```sh
oc -n openshift-monitoring get prometheus k8s --template='{{.spec.serviceMonitorNamespaceSelector}}{{"\n"}}{{.spec.serviceMonitorSelector}}{{"\n"}}'
map[matchLabels:map[openshift.io/cluster-monitoring:true]]
map[]
```
