# TEST TEXT
# Use ExternalDNS Operator on Openshift in Azure environment
**Note**: These instructions are not for Azure private DNS.

## Steps

1. Export your cluster's kubeconfig:
```bash
$ export KUBECONFIG=/path/to/your/cluster/kubeconfig
```

2. Make sure your user have the access to `kube-system` namespace:
```bash
$ oc whoami
system:admin
```

3. Fetch the values from `azure-credentials` secret present in `kube-system` namespace:
```bash
$ CLIENT_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_client_id}} | base64 -d)
$ CLIENT_SECRET=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_client_secret}} | base64 -d)
$ RESOURCE_GROUP=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_resourcegroup}} | base64 -d)
$ SUBSCRIPTION_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_subscription_id}} | base64 -d)
$ TENANT_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_tenant_id}} | base64 -d)
```

4. Login to Azure with base64 decoded values you get from above:
```bash
$ az login --service-principal -u "${CLIENT_ID}" -p "${CLIENT_SECRET}" --tenant "${TENANT_ID}"
```

5. Get the routes to check your cluster's domain (everything after `apps.`):
```bash
$ oc get routes --all-namespaces | grep console
openshift-console          console             console-openshift-console.apps.test-azure.qe.azure.devcluster.openshift.com                       console             https   reencrypt/Redirect     None
openshift-console          downloads           downloads-openshift-console.apps.test-azure.qe.azure.devcluster.openshift.com                     downloads           http    edge/Redirect          None
```

6. Get the list of dns zones w.r.t your resource group to find the one which corresponds to the previously found route’s domain.

For public dns zones:
```bash
$ az network dns zone list -g "${RESOURCE_GROUP}" -o tsv --query '[].name'
example.com
anotherdomain.net
test-azure.qe.azure.devcluster.openshift.com

$ ZONE_NAME="test-azure.qe.azure.devcluster.openshift.com"
```

For private dns zones:
```bash
$ az network private-dns zone list -g "${RESOURCE_GROUP}" -o tsv --query '[].name'
example.com
anotherdomain.net
test-azure.qe.azure.devcluster.openshift.com

$ ZONE_NAME="test-azure.qe.azure.devcluster.openshift.com"
```

7. Create an ExternalDNS CR.

For public dns zones:
```bash
$ cat <<EOF | oc create -f -
apiVersion: externaldns.olm.openshift.io/v1beta1
kind: ExternalDNS
metadata:
  name: sample-azure
spec:
  zones:
  - "/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RESOURCE_GROUP}/providers/Microsoft.Network/dnszones/${ZONE_NAME}"
  provider:
    type: Azure
  source:
    type: OpenShiftRoute
    openshiftRouteOptions:
      routerName: default
EOF
```

For private dns zones:
```bash
$ cat <<EOF | oc create -f -
apiVersion: externaldns.olm.openshift.io/v1beta1
kind: ExternalDNS
metadata:
  name: sample-azure-private
spec:
  zones:
  - "/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RESOURCE_GROUP}/providers/Microsoft.Network/privateDnsZones/${ZONE_NAME}"
  provider:
    type: Azure
  source:
    type: OpenShiftRoute
    openshiftRouteOptions:
      routerName: default
EOF
```

8. Check the records created for the routes.

For public dns zones:
```bash
$ az network dns record-set list -g "${RESOURCE_GROUP}" -z "${ZONE_NAME}" | grep console
```

For private dns zones:
```bash
$ az network private-dns record-set list -g "${RESOURCE_GROUP}" -z "${ZONE_NAME}" | grep console
```
