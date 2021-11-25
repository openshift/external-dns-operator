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
3. The previous step deploys the validation webhook, which requires TLS authentication for the webhook server. The
   manifests deployed through the `make deploy` command do not contain a valid certificate and key. You must provision a valid certificate and key through other tools.
   You can use a convenience script, `hack/generate-certs.sh` to generate the certificate bundle and patch the validation webhook config.   
   _Important_: Do not use the hack/generate-certs.sh script in a production environment.   
   Run the `hack/generate-certs.sh` script with the following inputs:
   ```bash
   $ hack/generate-certs.sh --service webhook-service --webhook validating-webhook-configuration \
   --secret webhook-server-cert --namespace external-dns-operator
   ```
4. Now you can deploy an instance of ExternalDNS:
    * Run the following command to create the credentials secret for AWS:
        ```bash
        $ kubectl -n external-dns-operator create secret generic aws-access-key \
                --from-literal=aws_access_key_id=${ACCESS_KEY_ID} \
                --from-literal=aws_secret_access_key=${ACCESS_SECRET_KEY}
        ```
       *Note*: See [this guide](./docs/usage.md) for instructions specific to other providers.
      
    * Run the following command:
      ```bash
      # for AWS
      $ kubectl apply -k config/samples/aws`
      ```
       *Note*: For other providers, see `config/samples/`.


### Verify ExternalDNS works with sample service example:
**Note**: On completion of the step 4, Make sure that the ExternalDNS should up and running.

Create the following sample application to test that ExternalDNS works.

> For services ExternalDNS will look for the annotation `external-dns.mydomain.org/publish: "yes"` on the service and use the corresponding value.

Sample service creation yaml as below:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    external-dns.mydomain.org/publish: "yes"
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: nginx

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - image: nginx
        name: nginx
        ports:
        - containerPort: 80
          name: http
```
After roughly two minutes check that a corresponding DNS record for nginx service was created.

*logs from externalDNS service to confirm the entries' creation*:
```
time="2021-11-25T08:31:01Z" level=debug msg="Refreshing zones list cache"
time="2021-11-25T08:31:02Z" level=debug msg="Considering zone: /hostedzone/Z02538821DNU9OAO61F0B (domain: myzonedomain.com.)"
time="2021-11-25T08:31:03Z" level=debug msg="Endpoints generated from service: test-service/nginx: [nginx.myzonedomain.com 0 IN A  10.96.255.127 []]"
time="2021-11-25T08:31:03Z" level=debug msg="Endpoints generated from service: external-dns-operator/webhook-service: [webhook-service.myzonedomain.com 0 IN A  10.96.125.29 []]"
time="2021-11-25T08:31:03Z" level=debug msg="Refreshing zones list cache"
time="2021-11-25T08:31:03Z" level=debug msg="Considering zone: /hostedzone/Z02538821DNU9OAO61F0B (domain: myzonedomain.com.)"
time="2021-11-25T08:31:03Z" level=debug msg="Adding nginx.myzonedomain.com. to zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:31:03Z" level=debug msg="Adding nginx.myzonedomain.com. to zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:31:03Z" level=info msg="Desired change: CREATE nginx.myzonedomain.com A [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:31:03Z" level=info msg="Desired change: CREATE nginx.myzonedomain.com TXT [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:31:04Z" level=info msg="2 record(s) in zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B] were successfully updated"
```

*Cli command to fetch the records from aws which created by externalDNS service*:
 ```
$ aws route53 list-resource-record-sets --output json --hosted-zone-id "/hostedzone/Z02538821DNU9OAO61F0B" \
>     --query "ResourceRecordSets[?Name == 'nginx.myzonedomain.com.']|[?Type == 'A']"
[
    {
        "Name": "nginx.myzonedomain.com.",
        "Type": "A",
        "TTL": 300,
        "ResourceRecords": [
            {
                "Value": "10.96.255.127"
            }
        ]
    }
]
```
>**Note**: Here the zone is `myzonedomain.com` and hostID is `Z02538821DNU9OAO61F0B`


##### Deletion of service and make sure the entries removed in aws
Ofter deleting the service then externalDNS will remove these entries from hostedZone.
 
*logs from externalDNS service to confirm the entries' deletion*:
```
time="2021-11-25T08:38:05Z" level=debug msg="Refreshing zones list cache"
time="2021-11-25T08:38:08Z" level=debug msg="Considering zone: /hostedzone/Z02538821DNU9OAO61F0B (domain: myzonedomain.com.)"
time="2021-11-25T08:38:08Z" level=debug msg="Endpoints generated from service: external-dns-operator/webhook-service: [webhook-service.myzonedomain.com 0 IN A  10.96.125.29 []]"
time="2021-11-25T08:38:08Z" level=debug msg="Refreshing zones list cache"
time="2021-11-25T08:38:09Z" level=debug msg="Considering zone: /hostedzone/Z02538821DNU9OAO61F0B (domain: myzonedomain.com.)"
time="2021-11-25T08:38:09Z" level=debug msg="Adding nginx.myzonedomain.com. to zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:38:09Z" level=debug msg="Adding nginx.myzonedomain.com. to zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:38:09Z" level=info msg="Desired change: DELETE nginx.myzonedomain.com A [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:38:09Z" level=info msg="Desired change: DELETE nginx.myzonedomain.com TXT [Id: /hostedzone/Z02538821DNU9OAO61F0B]"
time="2021-11-25T08:38:09Z" level=info msg="2 record(s) in zone myzonedomain.com. [Id: /hostedzone/Z02538821DNU9OAO61F0B] were successfully updated"

```

*Cli command to fetch the records from aws, there should not be any entries with `nginx.myzonedomain.com`*:
```
$ aws route53 list-resource-record-sets --output json --hosted-zone-id "/hostedzone/Z02538821DNU9OAO61F0B"     --query "ResourceRecordSets[?Name == 'nginx.myzonedomain.com.']|[?Type == 'A']"
[]
```
    
### Installing the `ExternalDNS` Operator using a custom index image on OperatorHub
**Note**: By default container engine used is docker but you can specify podman by adding CONTAINER_ENGINE=podman to your image build and push commands as mentioned below.
    
1. Build and push the operator image to the registry.
   
    a. Select the container runtime you want. Either podman or docker. 
    ```sh
    $ export CONTAINER_ENGINE=podman
    ```
    b. Set your image name:
    ```sh
    $ export IMG=<registry>/<username>/external-dns-operator:latest
    ```
    c. Build and push the image:
    ```sh
    $ make image-build
    $ make image-push
    ```
   
2. Build the bundle image.
  
    a. Export your expected image name which shall have your registry name in an env var.
    ```sh
    $ export BUNDLE_IMG=<registry>/<username>/external-dns-operator-bundle:latest
    ```
    b. In the bundle/manifests/external-dns-operator_clusterserviceversion.yaml
        add the operator image created in Step 1 as follows - 
    ```sh
        In annotations:
        Change containerImage: quay.io/openshift/origin-external-dns-operator:latest
        to containerImage: <registry>/<username, repository name>/external-dns-operator:latest
    
        In spec:
        Change image: quay.io/openshift/origin-external-dns-operator:latest
        to image: <registry>/<username, repository name>/external-dns-operator:latest
    ```
    c. Build the image
    ```sh   
    $ make bundle-image-build
    ```
   
3. Push the bundle image to the registry:
    ```sh
    $ make bundle-image-push
    ```

4. Build and push the image index to the registry:
   ```sh
   $ export INDEX_IMG=<registry>/<username>/external-dns-operator-bundle-index:1.0.0
   $ make index-image-build
   $ make index-image-push
   ```

5. Create the `external-dns-operator` namespace:
   ```sh
   $ oc create ns external-dns-operator
   ```

6. You may need to link the registry secret to the pod of `external-dns-operator` created in the `openshift-marketplace` namespace if the image is not made public ([Doc link](https://docs.openshift.com/container-platform/4.9/openshift_images/managing_images/using-image-pull-secrets.html#images-allow-pods-to-reference-images-from-secure-registries_using-image-pull-secrets)). If you are using `podman` then these are the instructions:

    a. Login to your registry:
    ```sh
    $ podman login quay.io
    ```
    b. Create a secret with registry auth details:
    ```sh
    $ oc -n openshift-marketplace create secret generic extdns-olm-secret  --type=kubernetes.io/dockercfg  --from-file=.dockercfg=${XDG_RUNTIME_DIR}/containers/auth.json
    ```
    c. Link the secret to default and builder service accounts:
    ```sh
    $ oc secrets link builder extdns-olm-secret -n openshift-marketplace
    $ oc secrets link default extdns-olm-secret --for=pull -n openshift-marketplace
    ````
    **Note**: the secret to the pod of `external-dns-operator` is part of the bundle created in step 1.


7. Create the `Catalogsource` object:

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

8. Create a subscription object to install the Operator:
   
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
    **Note**: You can install the `ExternalDNS` Operator through the web console: Navigate to  `Operators` -> `OperatorHub`, search for the `ExternalDNS operator`,  and install it in the `external-dns-operator` namespace.

### Running end-to-end tests

1. Deploy the operator as described above

2. Set the necessary environment variables
   In order for records created during the tests to be accessible, the e2e
   suite creates a public hosted zone as a subdomain of an existing zone. You
   must specify the existing zone's domain name and its zone id in the
   environment variables `EXTDNS_PARENT_DOMAIN` and `EXTDNS_PARENT_ZONEID`,
   respectively.
   ```sh
   $ export EXTDNS_PARENT_ZONEID="ZABCD123456789"
   $ export EXTDNS_PARENT_DOMAIN="example.com." # must include the trailing `.`
   ```

3. Run the test suite
   ```sh
   $ make test-e2e
   ```
