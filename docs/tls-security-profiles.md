# Operator serving TLS security profiles

The operator retains controller-runtime for its metrics, webhook and controller
servers. When platform detection sets `Config.IsOpenShift` to true, both secure
metrics and admission webhook listeners use the cluster
`APIServer` configuration (`apiservers.config.openshift.io/cluster`) when
`spec.tlsAdherence` is `StrictAllComponents`. Unknown adherence values also honor
the profile for forward compatibility and produce a warning log. Unset adherence
and `LegacyAdheringComponentsOnly` use the shared Modern default (TLS 1.3 minimum).
The same default applies on non-OpenShift clusters and when platform detection
is disabled; these cases skip the centralized TLS singleton read and observer.
Explicitly honored Intermediate, Old, Modern and Custom profiles retain their
configured settings; there is no additional weak-profile filtering.

Named profiles and Custom minimum TLS version, TLS 1.2 ciphers, and groups are
converted using library-go crypto helpers and a bounded Go group mapping:
X25519, secp256r1, secp384r1, secp521r1, and X25519MLKEM768. Runtime and FIPS
constraints still apply; mapping a group does not guarantee it can negotiate.
TLS 1.3 cipher suites are selected by Go, not the TLS 1.2 cipher list.

Conversion follows the AWS Load Balancer Operator's fallback behavior:
unsupported entries are skipped; if no groups map, Go's default group selection
applies, and if no ciphers map, library-go supplies default ciphers. There is no
additional rejection of an all-unmapped list. Nil/unknown profiles and a Custom
profile without a Custom body use the shared Modern default. An invalid minimum
TLS version is a startup error. API schema validation may reject malformed
profiles before the operator sees them.

## Changes restart the operator container

Every serving replica watches the singleton independently of leader election.
A change restarts that replica if adherence changes whether the cluster profile
is honored, or if the raw profile changes while honored. Group-only changes
count. Unset-to-Legacy changes, status changes, and profile-only changes while
ignored do not restart it. Raw nil versus explicit Modern and list-order
changes may restart even when negotiated settings would be equivalent.

The observer cancels the manager context. Controllers and listeners stop,
`Start` returns, and the process exits normally. The Deployment-managed Pod's
`Always` restart policy restarts the container; the Pod UID need not change.
The new process reads the latest policy and creates new listeners. There is no
in-process reload or previous-process policy persistence. Certificate loading
and rotation, metrics authentication/authorization, webhook HTTP/2 configuration,
and metrics HTTP/1.1 restriction retain their existing controller-runtime paths.

Replicas can restart concurrently; Deployment rolling-update budgets do not
serialize self-exits. Admission, metrics, and reconciliation can be unavailable
during restart, especially with a single replica. Existing ExternalDNS operand
processes do not need a policy-triggered restart.

## Platform gating and read failures

The existing platform detection result gates centralized TLS handling; no
additional API-availability discovery is performed. With `Config.IsOpenShift`
false, no TLS config client or observer is created. With it true, an initial
singleton GET failure (including NotFound, Forbidden or timeout) selects Modern
and still registers the observer. The diagnostic contains only a fixed message
and a bounded reason classification, not the raw error. TLS client construction
errors still abort startup. The TLS config client's requests and initial GET
are bounded by ten seconds. Controller-runtime may perform its normal REST
mapping discovery as part of reading the singleton; mapping errors follow the
same default/log/watch path, not a separate availability gate.

The failed-GET comparison baseline is nil profile plus Strict adherence. A later
successful observer read can detect a difference and restart the container to
recover the desired policy. Strict plus a nil desired profile needs no restart;
Legacy or Strict plus a nonnil desired profile does. Reconcile NotFound waits
for later events; other read errors retry without cancelling merely for the
error. Initial list events close the startup-read/watch gap. Watch bookmarks are
enabled so WatchList initial-events-end can complete cache synchronization.

Default serving during initial-read recovery may temporarily differ from desired
Strict restrictions. Persistent API or watch errors can prevent readiness or
convergence. An invalid honored policy can cause a restart followed by startup
failure until the desired configuration is corrected.

## Validation contract

Acceptance testing measures up to 300 seconds from a persisted policy change to
all serving replicas restarting, becoming ready and enforcing both endpoint
policies. No-restart cases have a 60-second observation plus deterministic
comparator tests. These are validation bounds, not availability guarantees.
Fault recovery is measured separately from fault clearance; persistent faults
cannot guarantee convergence. Fresh-process handshake/session tests must not
confuse shutdown/refusal with rejection of a prohibited TLS offer.

Local tests use isolated API/watch and TLS fixtures; they do not modify a live
cluster. Live acceptance additionally requires separately authorized access to
a cluster with the Groups and TLSAdherence schemas/gates and capable PQC/FIPS
clients. Missing capabilities must be recorded as deferred, not passed.
