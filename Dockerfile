# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:latest as builder

WORKDIR /opt/app-root/src
COPY . .

# Build
RUN make build-operator

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8/ubi-micro:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

