# Build the manager binary
FROM golang:1.17:latest as builder

WORKDIR /opt/app-root/src
COPY . .

# Build
RUN make build-operator

# Use minimal base image to package the manager binary
FROM registry.access.redhat.com/ubi8/ubi-micro:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

