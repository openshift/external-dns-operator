# Build the manager binary
FROM golang:1.22 as builder

WORKDIR /opt/app-root/src
COPY . .

# Build
RUN make build-operator

# Use minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi9/ubi-micro:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

