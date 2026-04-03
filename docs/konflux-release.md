# Konflux Release Process

This document describes the release process for the ExternalDNS Operator using the Konflux build system.

## Prerequisites

### Code Readiness

Ensure all code changes are merged in both repositories:
- [external-dns-operator](https://github.com/openshift/external-dns-operator) (operator)
- [external-dns](https://github.com/openshift/external-dns) (operand)

Both repositories must be on the same branch: `main`/`master` for the latest development or `release-X.Y` for a specific release.

The merged code should include the version change in the [`VERSION`](https://github.com/openshift/external-dns-operator/blob/main/VERSION) file matching the target release version.

### Nudging

Nudging must be enabled on the Konflux application components. Make sure the version is correct for the components being nudged.

The nudging order is as follows:
1. The `external-dns` (operand) component nudges the `bundle` component
2. The `external-dns-operator` (operator) component nudges the `bundle` component

### Container Digests

Verify that the latest images pushed by the component push pipelines are reflected in [`bundle-hack/container_digest.sh`](https://github.com/openshift/external-dns-operator/blob/main/bundle-hack/container_digest.sh). This file contains the image pullspecs with digests for:
- **Operator image** (`OPERATOR_IMAGE_PULLSPEC`)
- **Operand image** (`OPERAND_IMAGE_PULLSPEC`)
- **kube-rbac-proxy image** (`KUBE_RBAC_PROXY_IMAGE_PULLSPEC`)

All digests must match the images produced by the latest successful push pipelines before proceeding with the release.

If nudging is configured correctly, a dedicated PR with the updated digests will be automatically created in the [external-dns-operator](https://github.com/openshift/external-dns-operator) repository. This PR needs to be merged into the target branch.

## Verify Conforma

Each merge into an `external-dns-operator` branch triggers an automatic release to the stage registry. The release pipeline includes a `verify-conforma` task. All violations reported by this task must be resolved before the release to the stage index can proceed.

The Conforma results can be found in the releases section. Common violations and how to fix them:

- **Outdated Konflux task images**: MintMaker automatically creates PRs to update Konflux references. Make sure these PRs are merged before the release.
- **Missing annotations in CSV**: Add the required annotations to the ClusterServiceVersion manifest.
- **Hermetic builds**: Ensure the build pipelines are configured for hermetic builds.
- **Images referenced by OLM bundle are from allowed registries**: This violation appears when images from `quay.io/redhat-user-workload` registry are used. Only stage or production registries (e.g. `registry.stage.redhat.io`, `registry.redhat.io`) are allowed.

Refer to the [Konflux documentation](https://konflux-ci.dev/docs/) and other release examples in the repository to resolve these violations.

## Release to Stage Registry

This step releases the operand, operator, and bundle images to the stage registry. Each push into the branch of the `external-dns-operator` repository automatically creates a Release CR for the stage registry. The Release CR references a snapshot which contains all 3 images.

In order for the stage release pipeline to pass the Conforma test, the registry/repository in [`bundle-hack/container_digest.sh`](https://github.com/openshift/external-dns-operator/blob/main/bundle-hack/container_digest.sh) must be set to `registry.stage.redhat.io/edo/*`. Keep the digests the same as the ones created by the nudging PR.

This change of the registry to stage is currently not automated and must be done manually — create a PR with the registry update and merge it into the target branch.

## Update FBC Catalogs with Stage Bundle

Once the bundle is released to the stage registry, add it to the FBC catalog templates for all supported OCP versions. For each `catalog/v4.XX/catalog-template.yaml`:

1. Add the new bundle version entry to the relevant channels, updating the `replaces` chain accordingly.
2. Add an `olm.bundle` entry pointing to the stage registry bundle image.
3. Regenerate the catalogs using `make generate-catalog`.

Create a PR with the updated catalog templates and generated catalogs, and merge it into the target branch.

## Testing

Once the FBC update PR is merged, releases are automatically created. The release artifacts contain index images that can be used by QE to validate the release on different OCP versions.

The index image will be from the proxy registry. The QE engineer will need to configure mirroring and create a custom CatalogSource pointing to the index image.
