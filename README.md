# ExternalDNS Operator

The ExternalDNS Operator deploys and manages [ExternalDNS](https://github.com/kubernetes-sigs/external-dns), which dynamically manages
external DNS records in specific DNS Providers for specific Kubernetes resources.

This Operator is in the early stages of implementation. For the time being, please reference the
[ExternalDNS Operator OpenShift Enhancement Proposal](https://github.com/openshift/enhancements/pull/786).

## Deploy operator

### Quick development
1. Build and push the operator image to a registry:
   ```sh
   $ export IMG=<registry>/<username>/external-dns-operator:latest
   $ make image-build
   $ make image-push
   ```
2. Run `make deploy` to deploy the operator
3. Now you can deploy an instance of ExternalDNS:
    * Run the following command to create the credentials secret for AWS:
        ```bash
        $ kubectl -n external-dns-operator create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
        *Note*: other provider options can be found in `api/v1alpha1/externaldns_types.go`, e.g. the `ExternalDNSAWSProviderOptions` structure for AWS.
    * Run `kubectl apply -k config/samples/aws` for AWS    
        *Note*: other providers available in `config/samples/`

### OperatorHub install with custom index image

This process refers to building the operator in a way that it can be installed locally via the OperatorHub with a custom index image.

1. Build and push the bundle image to a registry:
   ```sh
   $ export BUNDLE_IMG=<registry>/<username>/external-dns-operator-bundle:latest
   $ make bundle-image-build
   $ make bundle-image-push
   ```

2. Build and push the image index for operator-registry:
   ```sh
   $ export INDEX_IMG=<registry>/<username>/external-dns-operator-bundle-index:1.0.0
   $ make index-image-build
   $ make index-image-push
   ```

3. Create the catalogsource (registry secret may need to be linked to the external-dns-operator's pod created in `openshift-marketplace`):
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

4. Create `external-dns-operator` namespace:
   ```sh
   $ oc create ns external-dns-operator
   ```
5. Create a subscription to install the operator:
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
    **Note**: Same thing can be done via the console: `Operators` -> `OperatorHub`, search for `ExternalDNS operator` and install the operator.
