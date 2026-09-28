# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.26 as builder

WORKDIR /opt/app-root/src
COPY . .
RUN git config --global --add safe.directory /opt/app-root/src

# Build
RUN make build-operator

# Install the OpenSSL FIPS provider (ubi-micro has no package manager)
FROM registry.access.redhat.com/ubi9/ubi-micro:latest AS micro-base

FROM registry.access.redhat.com/ubi9/ubi:latest AS package-installer
COPY --from=micro-base / /mnt/rootfs/
RUN dnf install -y \
      --installroot=/mnt/rootfs \
      --releasever=9 \
      --noplugins \
      --setopt=reposdir=/etc/yum.repos.d \
      --setopt=install_weak_deps=False \
      --nodocs \
      openssl-fips-provider \
    && test -f /mnt/rootfs/usr/lib64/ossl-modules/fips.so \
    && dnf clean all --installroot=/mnt/rootfs \
    && rm -rf /mnt/rootfs/var/cache/dnf \
              /mnt/rootfs/var/cache/yum \
              /mnt/rootfs/var/log/* \
              /mnt/rootfs/var/tmp/*

# Use micro base image to package the manager binary
FROM micro-base
WORKDIR /
COPY --from=package-installer /mnt/rootfs/ /
COPY --from=builder /opt/app-root/src/bin/external-dns-operator .

ENTRYPOINT ["/external-dns-operator"]

