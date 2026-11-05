# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.26 as builder

WORKDIR /opt/app-root/src
COPY . .
# Mark the source dir safe so the commit can be stamped via git in full-clone
# builds (CI, Konflux). Tolerate failure for builds from a git worktree, where
# .git is a gitlink pointing outside the build context and git cannot resolve it.
RUN git config --global --add safe.directory /opt/app-root/src || true

# Build. GOEXPERIMENT=strictfipsruntime is set here (container build only, on the
# Red Hat go-toolset) so the shipped binary aborts if it cannot operate in FIPS
# mode on a FIPS-enabled host. It is intentionally not in the Makefile: it is a
# go-toolset-only experiment that upstream Go rejects, so `make build` stays
# portable for contributors.
RUN GOEXPERIMENT=strictfipsruntime make build-operator

# Package the manager binary on a distroless static base.
# The binary uses Go's native FIPS 140-3 module (no OpenSSL), so it needs no
# RPMs and can run on a minimal Red Hat Hardened Image static base.
FROM registry.access.redhat.com/hi/static:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]
