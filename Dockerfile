# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.26 as builder

WORKDIR /opt/app-root/src
COPY . .
RUN git config --global --add safe.directory /opt/app-root/src

# Build. GOEXPERIMENT=strictfipsruntime is set here (container build only, on the
# Red Hat go-toolset) so the shipped binary aborts if it cannot operate in FIPS
# mode on a FIPS-enabled host. It is intentionally not in the Makefile: it is a
# go-toolset-only experiment that upstream Go rejects, so `make build` stays
# portable for contributors.
RUN GOEXPERIMENT=strictfipsruntime make build-operator

# Use minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi8/ubi-micro:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

