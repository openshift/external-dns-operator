# Configuring proxy support for ExternalDNS Operator

The ExternalDNS Operator can work in an environment with a cluster-wide egress proxy set up. There is some configuration to be done to make the operator aware of the proxy:
- operator container's environment has to be populated with one (or all) of the following variables: `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`.
- if the proxy uses some custom TLS certificate authority (CA), it has to be put into a configmap and passed to the operator via the `--trusted-ca-configmap` flag.

The ExternalDNS Operator doesn't need the proxy settings on its own because it doesn't interact with DNS providers. However, it has to propagate the proxy settings and CA certificate down to its operand: the ExternalDNS instance.

## Contents

- [Instructions](#instructions)
- [OpenShift instructions](#openshift-instructions)

## Instructions

### Proxy settings

#### Operator already running

Set HTTP proxy URLs for the operator's deployment:
```bash
kubectl -n external-dns-operator set env deployment/external-dns-operator HTTP_PROXY=http://myproxy.net HTTPS_PROXY=https://myproxy.net NO_PROXY=.cluster.local,.svc
```

### Custom CA

#### Operator already running

1. Create a configmap containing the PEM-encoded proxy CA certificate in the `external-dns-operator` namespace:
    ```bash
    kubectl -n external-dns-operator create configmap trusted-ca --from-file=ca-bundle.crt=/path/to/ca/certificate.pem
    ```

2. Patch the operator's deployment to reference the configmap created in the previous step:
    ```bash
    # "external-dns-operator" container is patched (first number: 0)
    # "TRUSTED_CA_CONFIGMAP_NAME" environment variable is patched (second number: 2)
    kubectl -n external-dns-operator patch deployment external-dns-operator --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/env/2/value", "value":"trusted-ca"}]'
    ```

## OpenShift instructions

If a global proxy is configured on the OpenShift cluster, OLM automatically configures Operators with cluster-wide proxy settings. `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables are added to the ExternalDNS Operator's deployment. The proxy's CA is trusted out of the box: the operator bundle ships an `external-dns-operator-trusted-ca` configmap populated by the Cluster Network Operator with the system and proxy's CA merged together and `TRUSTED_CA_CONFIGMAP_NAME` defaults to it.

### Custom CA

The manual steps below are only needed to trust an additional CA which is not part of the cluster-wide proxy. As the operator consumes a single configmap, such a CA has to be merged with the proxy's CA into one configmap.

#### For running operator

1. Create a configmap holding the merged bundle (the proxy's CA and the additional CA) under `ca-bundle.crt` in the `external-dns-operator` namespace. Do not add the `inject-trusted-cabundle` label, otherwise the Cluster Network Operator overwrites its contents:
    ```bash
    oc -n external-dns-operator create configmap custom-trusted-ca --from-file=ca-bundle.crt=/path/to/merged-ca-bundle.pem
    ```

2. Add `spec.config.env` with the name of the configmap created in the previous step to your subscription created by OperatorHub:
    ```bash
    oc -n external-dns-operator patch subscription external-dns-operator --type='json' -p='[{"op": "add", "path": "/spec/config", "value":{"env":[{"name":"TRUSTED_CA_CONFIGMAP_NAME","value":"custom-trusted-ca"}]}}]'
    ```

#### Manual deployment
You can use the following steps after the `external-dns-operator` namespace has been created and before the operator deployment has been created.

1. Create a configmap holding the merged bundle (the proxy's CA and the additional CA) under `ca-bundle.crt` in the `external-dns-operator` namespace. Do not add the `inject-trusted-cabundle` label, otherwise the Cluster Network Operator overwrites its contents:
    ```bash
    oc -n external-dns-operator create configmap custom-trusted-ca --from-file=ca-bundle.crt=/path/to/merged-ca-bundle.pem
    ```

2. Create the `Subscription` object:
    ```bash
    cat <<EOF | oc apply -f -
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
        name: external-dns-operator
        namespace: external-dns-operator
    spec:
        channel: stable-v1
        name: external-dns-operator
        source: external-dns-operator
        sourceNamespace: openshift-marketplace
        config:
          env:
          - name: TRUSTED_CA_CONFIGMAP_NAME
            value: custom-trusted-ca
    EOF
    ```
