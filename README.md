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
    * Run the following command to create the credentials secret on AWS:
        ```bash
        $ kubectl -n external-dns-operator create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
        *Note*: other provider options can be found in `api/v1alpha1/externaldns_types.go`, e.g. the `ExternalDNSAWSProviderOptions` structure for AWS.
    * Run `kubectl apply -k config/samples/aws` for AWS    
        *Note*: other providers available in `config/samples/`
