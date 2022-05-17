# Use ExternalDNS Operator on Openshift in GCP environment

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

3. Extract the value of `service_account.json` field from `gcp-credentials` secret into a file:
```bash
$ oc get secret gcp-credentials -n kube-system --template='{{$v := index .data "service_account.json"}}{{$v}}' | base64 -d - > decoded-gcloud.json
```

4. Set Google credentials:
```bash
$ export GOOGLE_CREDENTIALS=decoded-gcloud.json
```

5. Fetch the values from the credentials file:
```bash
$ CLIENT_EMAIL=$(jq -r .client_email < decoded-gcloud.json)
$ PROJECT_ID=$(jq -r .project_id < decoded-gcloud.json)
```

6. Activate your account:
```bash
$ gcloud auth activate-service-account "${CLIENT_EMAIL}" --key-file=decoded-gcloud.json
```

7. Set your project:
```bash
$ gcloud config set project "${PROJECT_ID}"
```

8. Get the routes to check your cluster's domain (everything after `apps.`):
```bash
$ oc get routes --all-namespaces | grep console
openshift-console          console             console-openshift-console.apps.qe.gcp.devcluster.openshift.com                       console             https   reencrypt/Redirect     None
openshift-console          downloads           downloads-openshift-console.apps.qe.gcp.devcluster.openshift.com                     downloads           http    edge/Redirect          None
```

9. Get your cluster's zone:
```bash
$ gcloud dns managed-zones list | grep qe.gcp.devcluster.openshift.com
qe-cvs4g-private-zone qe.gcp.devcluster.openshift.com
```

10. Create [ExternalDNS CR](https://github.com/openshift/external-dns-operator/blob/main/config/samples/gcp/operator_v1beta1_externaldns_openshift.yaml) as follows:
```bash
$ cat <<EOF | oc create -f -
apiVersion: externaldns.olm.openshift.io/v1alpha1
kind: ExternalDNS
metadata:
  name: sample-gcp
spec:
  domains:
  - filterType: Include
    matchType: Exact
    name: qe.gcp.devcluster.openshift.com
  provider:
    type: GCP
  source:
    type: OpenShiftRoute
    openshiftRouteOptions:
      routerName: default
EOF
```

11. Check the records created for `console` routes:
```bash
$ gcloud dns record-sets list --zone= qe-cvs4g-private-zone | grep console
```
