# Konflux Hermetic Build Configuration

This directory contains RPM prefetch configuration for
[Cachi2](https://github.com/containerbuildsystem/cachi2), enabling
hermetic (network-isolated) Konflux builds that install RPM packages.

## Files

| File | Purpose |
|------|---------|
| `rpms.in.yaml` | Declares RPM packages to prefetch and the base image context |
| `rpms.lock.yaml` | Generated lockfile with exact RPM URLs and checksums (per arch) |
| `redhat.repo` | Repository definitions used for RPM resolution |

## How it works

The operator image is based on `ubi-micro`, which has no package manager.
A `package-installer` stage in the Containerfile uses `dnf --installroot`
to install `openssl-fips-provider` (required for FIPS compliance) into
the `ubi-micro` rootfs.

Konflux builds run with `hermetic: true` (no network during `buildah`).
The Cachi2 `prefetch-dependencies` task downloads the locked RPMs before
the build starts, and `buildah` injects them as a local repo so `dnf`
can resolve them offline.

The Tekton pipelines reference this directory via:
```yaml
- name: prefetch-input
  value: '[{"type": "rpm", "path": ".konflux"}]'
```

## Prerequisites

The [rpm-lockfile-prototype](https://github.com/konflux-ci/rpm-lockfile-prototype)
tool is required to generate `rpms.lock.yaml`.

## Updating

When the `ubi-micro` base image digest changes (e.g. via Renovate), the
lockfile must be regenerated:

1. Update `context.image` in `rpms.in.yaml` to match the new digest in
   `Containerfile.external-dns-operator`
2. Run `make konflux-rpm-lock`
3. Commit the updated `rpms.in.yaml` and `rpms.lock.yaml`

To add or remove packages, edit the `packages` list in `rpms.in.yaml`
and run `make konflux-rpm-lock`.

## Extracting repo definitions

The `redhat.repo` file was extracted from the UBI 9 base image. If it
needs to be refreshed:

```
podman run --rm registry.access.redhat.com/ubi9/ubi:latest \
  cat /etc/yum.repos.d/ubi.repo > .konflux/redhat.repo
```

Then keep only the enabled repos (baseos, appstream).
