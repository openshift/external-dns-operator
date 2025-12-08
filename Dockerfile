# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 as builder

WORKDIR /opt/app-root/src
COPY . .
RUN git config --global --add safe.directory /opt/app-root/src

# Build
RUN make build-operator

# Use minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

