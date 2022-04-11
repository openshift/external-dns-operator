# Using ExternalDNS Operator with the operands in a separate namespace
The ExternalDNS Operator deploys operands in its own namespace (`external-dns-operator` by default). This deployment model fits well the opinionated OLM design.
However ExternalDNS Operator was initially designed similar to the other cluster level operators which often deploy their operands in a separate namespace.
Even though the old model is **not** officially supported, technically the operator is still capable of working with the operands residing in a different namespace.

## WHYs
Why you may want to deploy operands in a separate namespace?
- Easy clean-up of the operands: they can be removed without the risk of touching the operator
- Safe operator namespace removal, risk of a deadlocked namespace. Not quite applicable for the cluster level CRs but still worth a mention:
    - Race between the removals of the operand and the operator which may still receive a webhook for the delete event
    - Race between the removals of the CR which needs to be finalized and the operator which does the finalization

## Deployment
- Remove `--operand-namespace` flag from the `args` of `external-dns-operator` container [here](../../config/manager/manager.yaml)
- Update OLM bundle manifests using `make bundle` command if you plan to install via Operator Hub
- Create the operand namespace and extra RBAC:
    ```sh
    kubectl create ns external-dns
    kubectl apply -f config/rbac/extra-roles.yaml
    ```
- Follow [README](../../README.md) instructions to install the operator

## End to end test
- Instruct the e2e test to ensure the operand resources:
    ```sh
    export E2E_SEPARATE_OPERAND_NAMESPACE=true
    ```
- Follow [README](../../README.md) instructions to run the e2e test

## Technical debt
The ExternalDNS Operator was migrated to the model with the single namespace for the operator and the operands when it was already mature.  
This left some technical decisions which are not strictly necessary anymore. Here is the evolving list of them:
- Dedicated controllers for the credentials secret and trusted CA configmap
    - Copy of the secret and the configmap from the operator namespace into the operand's one doesn't need to be made anymore
- `--operand-namespace` flag
