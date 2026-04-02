# Versioning and Branching in ExternalDNS Operator

The ExternalDNS Operator follows the semantic versioning, for any given release `X.Y.Z`:
* an X (major) release indicates a set of backwards-compatible changes. Changing X means there's a breaking change.
* a Y (minor) release indicates a minimum feature set. Changing Y means the addition of a backwards-compatible feature.
* a Z (patch) release indicates minimum set of bugfixes. Changing Z means a backwards-compatible change that doesn't add functionality.

## Branches

The ExternalDNS Operator repository contains two types of branches: the `main` branch and `release-X.Y` branches.

The main branch is where development happens. All the latest code, including breaking changes, happens on `main`.
The `release-X.Y` branches contain stable, backwards compatible code. Every minor (`X.Y`) release, a new such branch is created.

## Channels

The ExternalDNS Operator's releases get published in two types of [OLM channels](https://olm.operatorframework.io/docs/glossary/#channel): the minor `release-vX.Y` and the major `release-vX`.

The minor channels contain patch releases. The major channels contain all patch releases from all minor channels.

## OpenShift Version Compatibility

The ExternalDNS Operator supports all OCP versions down to the latest EUS release. All fixes, including CVE fixes, are published for all supported OCP versions.

Some OCP versions may have reached [End of Life](https://access.redhat.com/support/policy/updates/openshift) (e.g. 4.13, 4.15, 4.17). The ExternalDNS Operator continues to publish on these versions to ensure smooth OCP upgrades, as upgrades traverse through all intermediate versions.

## Support model

| ExternalDNS Operator release       | Support model   |
| :--------------------------------: | :-------------: |
| 1.3                                | Full Support    |
| 1.2                                | Full Support    |
| 1.1                                | Full Support    |
| 1.0                                | Deprecated      |
| 0.1                                | End of Life     |

### Full support

During the Full Support phase, qualified critical and important security fixes will be released as they become available.
Urgent and high priority bug fixes will be released as they become available. Other fixes and qualified patches may be released via periodic updates.
To receive security and bug fixes, users are expected to upgrade the operator to the most current supported patch version.

### Deprecated

Deprecated releases are no longer receiving any fixes, including critical security (CVE) fixes. A deprecated release will be pruned with the next minor release of the operator. Users are strongly encouraged to upgrade to a supported release.

## Minor version release history

| ExternalDNS Operator version | First published on OCP |
| :--------------------------: | :--------------------: |
| 1.3                          | 4.17                   |
| 1.2                          | 4.14                   |
| 1.1                          | 4.12                   |
| 1.0                          | 4.11                   |
| 0.1                          | 4.10                   |
