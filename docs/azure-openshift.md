# Use external dns operator on Openshift in Azure environment
## Note: These instructions are not for Azure private DNS. 

### Steps

1. Export your kubeconfig
```bash
 $ export KUBECONFIG=~/<path to>/kubeconfig
```

2. Check user. The user shall access to kube-system.

```bash
 $ oc whoami
 system:admin
```

3. Fetch the values from azure-credentials secret present in kube-system
```bash
$ CLIENT_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_client_id}} | base64 -d)
$ CLIENT_SECRET=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_client_secret}} | base64 -d)
$ RESOURCE_GROUP=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_resourcegroup}} | base64 -d)
$ SUBSCRIPTION_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_subscription_id}} | base64 -d)
$ TENANT_ID=$(oc get secrets azure-credentials  -n kube-system  --template={{.data.azure_tenant_id}} | base64 -d)
```

4. Login to azure with base64 decoded values you get from above

```bash
$ az login --service-principal -u "${CLIENT_ID}" -p "${CLIENT_SECRET}" --tenant "${TENANT_ID}"
```

5. Get the routes to check the domain
```bash
oc get routes --all-namespaces | grep console
openshift-console          console             console-openshift-console.apps.test-azure.qe.azure.devcluster.openshift.com                       console             https   reencrypt/Redirect     None
openshift-console          downloads           downloads-openshift-console.apps.test-azure.qe.azure.devcluster.openshift.com                     downloads           http    edge/Redirect          None
```

6. Get the dns zone list w.r.t your resource group.
```bash
$ az network dns zone list --resource-group "${RESOURCE_GROUP}"
[]
```
Initially you will see nothing. So you will have to create a zone for your resource group.

7. Create a dns zone for you resourcegroup.
```bash
$ az network dns zone create -g "${RESOURCE_GROUP}" -n <pick up the zone name from the route  eg- the part after apps. eg - test-azure.qe.azure.devcluster.openshift.com>
```

8. Then create [ExternalDNS CR](https://github.com/openshift/external-dns-operator/blob/main/config/samples/azure/operator_v1alpha1_externaldns_openshift.yaml) as follows -  
```yaml
$ cat <<EOF | oc create -f -
apiVersion: externaldns.olm.openshift.io/v1alpha1
kind: ExternalDNS
metadata:
  name: sample-azure
spec:
  domains:
  - filterType: Include
    matchType: Exact
    name: test-azure1.qe.azure.devcluster.openshift.com
  provider:
    type: Azure
  source:
    openshiftRouteOptions:
      routerName: default
    fqdnTemplate:
    - "{{.Name}}.apps.test-azure1.qe.azure.devcluster.openshift.com"
    type: OpenShiftRoute
  zones:
  - "/subscriptions/53b4f551-f0fc-4bea-8cba-11111111111/resourceGroups/test-azure1-nxkxm-rg/providers/Microsoft.Network/dnszones/test-azure1.qe.azure.devcluster.openshift.com"
EOF
```

9. Now you shall see records getting created for OCP created routes using the following command -
```bash
$ az network dns record-set list -g "${RESOURCE_GROUP}"  -z <zone name you created in step 7 eg - test-azure.qe.azure.devcluster.openshift.com>  > record-list-azure.txt
```

10. You can try to create a route with a sample app.
```bash
$ oc new-app --docker-image=openshift/hello-openshift -l app=hello-openshift
$ oc expose service/hello-openshift -l app=hello-openshift
```

11. `hello-openshift` DNS records shall exist in the zone:
```bash
$ az network dns record-set list -g "${RESOURCE_GROUP}" -z <zone name you created in step 7 eg - test-azure.qe.azure.devcluster.openshift.com>  | grep -c "hello-openshift"
```

13. Similarly you can delete the route and app
```bash
$ oc delete all -l  app=hello-openshift 
```

14. Then again run the command to check that `hello-openshift` records are not there anymore:
```bash
$ az network dns record-set list -g "${RESOURCE_GROUP}"  -z <zone name you created in step 7 eg - test-azure.qe.azure.devcluster.openshift.com>  | grep -c "hello-openshift"
```