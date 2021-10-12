# ExternalDNS Operator

The `ExternalDNS` Operator allows you to deploy and manage [ExternalDNS](https://github.com/kubernetes-sigs/external-dns), a cluster-internal component which makes Kubernetes resources discoverable through public DNS servers. \
**Note**: This Operator is in the early stages of implementation. For more information, see
[ExternalDNS Operator OpenShift Enhancement Proposal](https://github.com/openshift/enhancements/pull/786).

## Deploying the `ExternalDNS` Operator
The following procedure describes how to deploy the `ExternalDNS` Operator for AWS.

### Installing the `ExternalDNS` Operator by building and pushing the Operator image to a registry
1. Build and push the Operator image to a registry:
   ```sh
   $ export IMG=<registry>/<username>/external-dns-operator:latest
   $ make image-build
   $ make image-push
   ```
2. Run the following command to deploy the `ExternalDNS` Operator:
    ```
    $ make deploy
    ```
3. Deploy an instance of ExternalDNS:
    * Create the `credentials` secret for AWS:  
       *Note*: See the following guide for the other providers: [usage guide](./docs/usage.md).

        ```bash
        $ kubectl -n external-dns-operator create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
      
    * Run the following command:
      ```bash
      # for AWS
      $ kubectl apply -k config/samples/aws`
      ```
       *Note*: For other providers, see `config/samples/`.


### Installing the `ExternalDNS` Operator using a custom index image on OperatorHub

1. Build and push the bundle image to a registry:
   ```sh
   $ export BUNDLE_IMG=<registry>/<username>/external-dns-operator-bundle:latest
   $ make bundle-image-build
   $ make bundle-image-push
   ```

2. Build and push the image index for `operator-registry`:
   ```sh
   $ export INDEX_IMG=<registry>/<username>/external-dns-operator-bundle-index:1.0.0
   $ make index-image-build
   $ make index-image-push
   ```

3. Create the `Catalogsource` object (you may need to link the registry secret to the pod of `external-dns-operator` created in the `openshift-marketplace` namespace): \
   *Note* the secret to the pod of `external-dns-operator` is part of the bundle created in step 1.
   ```yaml
   $ cat <<EOF | oc apply -f -
   apiVersion: operators.coreos.com/v1alpha1
   kind: CatalogSource
   metadata:
     name: external-dns-operator
     namespace: openshift-marketplace
   spec:
     sourceType: grpc
     image: <registry>/<username>/external-dns-operator-bundle-index:1.0.0
   EOF
   ```

4. Create the `external-dns-operator` namespace:
   ```sh
   $ oc create ns external-dns-operator
   ```
5. Create a subscription object to install the Operator:
    ```yaml
    cat <<EOF | oc apply -f -
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
      name: external-dns-operator
      namespace: external-dns-operator
    spec:
      channel: alpha
      name: external-dns-operator
      source: external-dns-operator
      sourceNamespace: openshift-marketplace
    EOF
    ```
    **Note**: You can install the `ExternalDNS` Operator through the web console: Navigate to  `Operators` -> `OperatorHub`, search for the `ExternalDNS operator`,  and install the operator.
