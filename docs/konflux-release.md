# Konflux Release Process

This document describes the release process for the ExternalDNS Operator using the Konflux build system.

## 1. Prerequisites

### Code Readiness

Ensure all code changes are merged in both repositories:
- [external-dns-operator](https://github.com/openshift/external-dns-operator) (operator)
- [external-dns](https://github.com/openshift/external-dns) (operand)

Both repositories must be on the same branch: `main`/`master` for the latest development or `release-X.Y` for a specific release.

The merged code should include the version change in the [`VERSION`](../VERSION) file matching the target release version.

### Nudging

Nudging must be enabled on the Konflux application components. Make sure the version is correct for the components being nudged.

The nudging order is as follows:
1. The `external-dns` (operand) component nudges the `bundle` component
2. The `external-dns-operator` (operator) component nudges the `bundle` component

### Container Digests

Verify that the latest images pushed by the component push pipelines are reflected in [`bundle-hack/container_digest.sh`](../bundle-hack/container_digest.sh). This file contains the image pullspecs with digests for:
- **Operator image** (`OPERATOR_IMAGE_PULLSPEC`)
- **Operand image** (`OPERAND_IMAGE_PULLSPEC`)
- **kube-rbac-proxy image** (`KUBE_RBAC_PROXY_IMAGE_PULLSPEC`)

All digests must match the images produced by the latest successful push pipelines before proceeding with the release.

**Note on multi-arch digests:** The `external-dns` (operand) component is a multi-arch build, so its snapshot entry contains a manifest list digest. However, `bundle-hack/container_digest.sh` and the nudging PRs use the amd64-specific digest from within that manifest list. To verify that the digests match, inspect the manifest list from the snapshot and confirm the amd64 entry matches the digest in `container_digest.sh`:

```bash
podman manifest inspect <operand-image>@sha256:<manifest-list-digest> | jq '.manifests[] | select(.platform.architecture == "amd64") | .digest'
```

If nudging is configured correctly, a dedicated PR with the updated digests will be automatically created in the [external-dns-operator](https://github.com/openshift/external-dns-operator) repository. This PR needs to be merged into the target branch.

### Verify Conforma

Each merge into an `external-dns-operator` branch triggers an automatic release to the stage registry. The release pipeline includes a `verify-conforma` task. All violations reported by this task must be resolved before the release to the stage index can proceed.

The Conforma results can be found in the releases section. Common violations and how to fix them:

- **Outdated Konflux task images**: MintMaker automatically creates PRs to update Konflux references. Make sure these PRs are merged before the release.
- **Missing annotations in CSV**: Add the required annotations to the ClusterServiceVersion manifest.
- **Hermetic builds**: Ensure the build pipelines are configured for hermetic builds.
- **Images referenced by OLM bundle are from allowed registries**: This violation appears when images from `quay.io/redhat-user-workload` registry are used. Only stage or production registries (e.g. `registry.stage.redhat.io`, `registry.redhat.io`) are allowed.

Refer to the [Konflux documentation](https://konflux-ci.dev/docs/) and other release examples in the repository to resolve these violations.

### Update ReleasePlanAdmission Tags

The RPA now uses the `version` label from the built image as the floating tag, so manual tag updates are no longer needed for patch releases. The `version` label is set from the [`VERSION`](../VERSION) file during the build. Make sure the `VERSION` file is updated before the release build.

For a new minor release, new ReleasePlanAdmission and ReleasePlan objects need to be created in the [konflux-release-data](https://gitlab.cee.redhat.com/releng/konflux-release-data) repository.

## 2. Release to Stage

This step can be skipped if QE verification is not needed or for the sake of time saving. In that case, proceed directly to [Release to Production](#3-release-to-production).

### Release bundle to Stage Registry

Each push into the branch of the `external-dns-operator` repository automatically creates a Release CR for the stage registry. The Release CR references a snapshot which contains all 3 images (operator, operand, and bundle).

Previously, the bundle's [`bundle-hack/container_digest.sh`](../bundle-hack/container_digest.sh) had to be updated to reference `registry.stage.redhat.io/edo/*` for the stage release. This is no longer needed — the Konflux ITS deploy pipeline and QE testing rely on image mirror sets that resolve `registry.redhat.io` references to the Konflux builds (see [PR #477](https://github.com/openshift/external-dns-operator/pull/477)).

**Important:** Before proceeding, pause all merges to the target branch of both `external-dns-operator` and `external-dns` repositories. Any new merge to the operator or operand will create a new snapshot, and if the corresponding nudging changes are not merged, the Conforma check during the "release to production" pipeline will fail.

### Update FBC Catalogs with Stage Bundle

Once the bundle is released to the stage registry, add it to the FBC catalog templates for all supported OCP versions. For each `catalog/v4.XX/catalog-template.yaml`:

1. Add the new bundle version entry to the relevant channels, updating the `replaces` chain accordingly.
2. Add an `olm.bundle` entry pointing to the stage registry bundle image.
3. Regenerate the catalogs using `make generate-catalog`.

Create a PR with the updated catalog templates and generated catalogs, and merge it into `main` — the catalog directory only exists on `main`, not on release branches.

### Test index images

Once the FBC update PR is merged, releases are automatically created. The release artifacts contain index images that can be used by QE to validate the release on different OCP versions.

The index image will be from the proxy registry. The QE engineer will need to configure mirroring and create a custom CatalogSource pointing to the index image.

## 3. Release to Production

### Release bundle to Production Registry

Update [`bundle-hack/container_digest.sh`](../bundle-hack/container_digest.sh) to use the production registry (`registry.redhat.io/edo/*`), keeping the same digests. Create a PR with this change and merge it into the target branch.

Once the push pipeline creates a new snapshot, it needs to be released to production. The automated release to stage will fail because the images contain unreleased images from the production registry. To release, either create a Release CR referencing the snapshot, or trigger it from the Konflux console by going to the corresponding release plan (e.g. `1.2`, `1.3`) and triggering a release from there.

Example Release CR for the production release:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  generateName: ext-dns-optr-1-2-rhel-8-20260413-121325-000-az-a1b2c3d-
  namespace: external-dns-operator-tenant
  labels:
    release.appstudio.openshift.io/automated: "false"
    release.appstudio.openshift.io/author: gpiotrow
spec:
  releasePlan: external-dns-operator-1-2-release-plan-prod
  snapshot: ext-dns-optr-1-2-rhel-8-20260413-121325-000-az
```

Once the release pipeline succeeds, all images — operator, operand, and bundle — will be published on `registry.redhat.io`.

### Update FBC Catalogs with Production Bundle

Update the FBC catalog templates for all supported OCP versions to use the production bundle from `registry.redhat.io`. The digest remains the same as the stage bundle — this is the image that was tested. If the digest changes, the testing process must be repeated. For each `catalog/v4.XX/catalog-template.yaml`:

1. Replace the stage registry bundle image with the production one. The digest remains the same, only the registry changes.
2. Regenerate the catalogs using `make generate-catalog`.

Create a PR with the updated catalog templates and generated catalogs, and merge it into `main` — the catalog directory only exists on `main`, not on release branches.

### Release FBC to Production Operator Index

Once the FBC update PR is merged, a push pipeline runs and creates a snapshot for each FBC component. Each snapshot needs to be released — either by creating a Release CR or by triggering it from the Konflux console.

Example Release CR for the `4.21` FBC:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  generateName: ext-dns-optr-fbc-v4-21-45gt6-1830b2f-
  namespace: external-dns-operator-tenant
  labels:
    release.appstudio.openshift.io/automated: "false"
    release.appstudio.openshift.io/author: gpiotrow
spec:
  releasePlan: external-dns-fbc-v4-21-release-plan-prod
  snapshot: ext-dns-optr-fbc-v4-21-45gt6
```

Real-world examples of FBC Release CRs can be found in the [konflux-release-examples](konflux-release-examples/) directory.

### Verify Production Operator Index

Once the FBC release pipelines succeed, verify that the released version appears in the production `redhat-operators` index for all supported OCP versions:

```bash
for v in 12 13 14 15 16 17 18 19 20 21 22; do
  echo "=== v4.${v} ==="
  podman run --rm --pull=always --entrypoint cat \
    registry.redhat.io/redhat/redhat-operator-index:v4.${v} \
    /configs/external-dns-operator/catalog.yaml \
    | grep "external-dns-operator.v${VERSION}"
done
```

Set `VERSION` to the version that was released.
