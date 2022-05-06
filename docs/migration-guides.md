# Migration guides

## From 0.1.x (TechPreview) to 1.0.x (GA)

Features to consider:
- Operands are deployed in the operator namespace (`external-dns-operator` by default)

### Steps
The below steps are designed to provide the migration without the service interruption (DNS records will be preserved):

- Remove `external-dns` namespace to prevent multiple ExternalDNS instances from managing the same records
    ```sh
    oc delete namespace external-dns
    ```
- Upgrade the operator to the new version
- Check out `external-dns-operator` namespace, it's supposed to have the operands:
    ```sh
    oc -n external-dns-operator get pods
    NAME                                         READY   STATUS    RESTARTS   AGE
    external-dns-operator-64f885498c-75djt       2/2     Running   0          5m
    external-dns-sample-aws-5c6cdc5dd8-tpvmx     1/1     Running   0          2m
    ```
