# Use ExternalDNS Operator on Openshift in GCP environment

### This document provides information about how to use the `ExternalDNS Operator` in OpenShift Container Platform on GCP.

### Steps
1. Export your cluster's kubeconfig
```bash
 $ export KUBECONFIG=/path/to/your/cluster/kubeconfig
```

2. Check user. The user shall have the access to `kube-system` namespace.

```bash
 $ oc whoami
 system:admin
```

3. Copy the value of service_account.json in gcp-credentials secret in a file encoded-gcloud.json
```bash
$ oc get secret gcp-credentials -n kube-system --template='{{$v := index .data "service_account.json"}}{{$v}}' | base64 -d - > decoded-gcloud.json
```

4. Export Google credentials
```bash
$ export GOOGLE_CREDENTIALS=decoded-gcloud.json
```

5. Activate your account
```bash
$ gcloud auth activate-service-account  <client_email as per decoded-gcloud.json> --key-file=decoded-gcloud.json
```

6. Set your project
```bash
$ gcloud config set project <project_id as per decoded-gcloud.json>
```

7. Get the routes to check the domain
```bash
$ oc get routes --all-namespaces | grep console
openshift-console          console             console-openshift-console.apps.qe.gcp.devcluster.openshift.com                       console             https   reencrypt/Redirect     None
openshift-console          downloads           downloads-openshift-console.apps.qe.gcp.devcluster.openshift.com                     downloads           http    edge/Redirect          None
```

8. Get your zone which was created by the installer
```bash
$ gcloud dns managed-zones list | grep <dns name eg- As per the route the section after apps. i.e misalunk-azure.qe.azure.devcluster.openshift.com>
```

9. Check the zone doesn't have DNS records other than `NS` and `SOA`
```bash
$ gcloud dns record-sets list --zone=<the zone name you get from step 9>
```

10. Create a [ExternalDNS CR](https://github.com/openshift/external-dns-operator/blob/main/config/samples/gcp/operator_v1alpha1_externaldns_openshift.yaml)
```yaml
$ cat <<EOF | oc create -f -
apiVersion: externaldns.olm.openshift.io/v1alpha1
kind: ExternalDNS
metadata:
  name: sample-gcp
spec:
  domains:
    - filterType: Include
      matchType: Exact
      name: test-gcp1.qe.gcp.devcluster.openshift.com
  provider:
    type: GCP
  source:
    openshiftRouteOptions:
      routerName: default
    type: OpenShiftRoute
    fqdnTemplate:
      - "{{.Name}}.apps.test-gcp1.qe.gcp.devcluster.openshift.com"
  #You will get this from step 9
  zones:
    - test-gcp1-q6m5v-private-zone
EOF
```

11. Now you shall see records created for OCP routes using the following command
```bash
$ gcloud dns record-sets list --zone=<zone name you get from step 9>
```

12. You can try to create a route with a sample app.
```bash
$ oc new-app --docker-image=openshift/hello-openshift -l app=hello-openshift
$ oc expose service/hello-openshift -l app=hello-openshift
```

13. Check the record for the hello-openshift route
```bash
$ gcloud dns record-sets list --zone=<zone name you get from step 9> | grep hello-openshift
```

14. Similarly you can delete the route and app
```bash
$ oc delete all -l app=hello-openshift
```

15. Then again run the command in step 13 to get the records for hello. They should not exist.