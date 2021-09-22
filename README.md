# ExternalDNS Operator

The ExternalDNS Operator deploys and manages [ExternalDNS](https://github.com/kubernetes-sigs/external-dns), which dynamically manages
external DNS records in specific DNS Providers for specific Kubernetes resources.

This Operator is in the early stages of implementation. For the time being, please reference the
[ExternalDNS Operator OpenShift Enhancement Proposal](https://github.com/openshift/enhancements/pull/786).

## Deploy operator

### Quick development
1. Build and push the operator image to a registry:
   ```sh
   $ podman build -t <registry>/<username>/external-dns-operator:latest -f Dockerfile .
   $ podman push <registry>/<username>/external-dns-operator:latest
   ```
2. Make sure to uncomment the `image` in `config/manager/kustomization.yaml` and set it to the operator image you pushed
3. Run `kubectl apply -k config/default`
4. Now you can deploy an instance of ExternalDNS:
    * Run the following commands to create the credentials secret on AWS:
        ```bash
        $ kubectl create namespace external-dns
        $ kubectl -n external-dns create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
        *Note*: other provider options can be found in `api/v1alpha1/externaldns_types.go`, e.g. the `ExternalDNSAWSProviderOptions` structure for AWS.
    * Run `kubectl apply -k config/samples/aws` for AWS    
        *Note*: other providers available in `config/samples/`
