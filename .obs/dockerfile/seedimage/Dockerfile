# SPDX-License-Identifier: Apache-2.0
# Define the names/tags of the container
#!BuildTag: rancher/seedimage-builder:latest
#!BuildTag: rancher/seedimage-builder:%OPERATOR_VERSION%
#!BuildTag: rancher/seedimage-builder:%OPERATOR_VERSION%-%RELEASE%
#

ARG SLEMICRO_VERSION

FROM suse/sle-micro/$SLEMICRO_VERSION/toolbox as BASE

RUN  mkdir -p /installroot && \
    zypper --gpg-auto-import-keys --installroot /installroot in -y --no-recommends elemental-toolkit elemental-httpfy elemental-seedimage-hooks xorriso curl coreutils ca-certificates ca-certificates-mozilla gptfdisk squashfs util-linux dosfstools mtools e2fsprogs grub2


FROM scratch as SEEDIMAGE_BUILDER

COPY --from=BASE /installroot /

RUN /usr/sbin/update-ca-certificates

ARG BUILD_REPO=%%IMG_REPO%%
ARG IMAGE_REPO=$BUILD_REPO/rancher/elemental-seedimage-builder-image

# Define labels according to https://en.opensuse.org/Building_derived_containers
# labelprefix=com.rancher.elemental
LABEL org.opencontainers.image.title="Rancher Elemental Seed Image Builder"
LABEL org.opencontainers.image.description="Provides the tools needed to build Elemental Seed Images"
LABEL org.opencontainers.image.version="%OPERATOR_VERSION%"
LABEL org.opencontainers.image.url="https://github.com/rancher/elemental-operator"
LABEL org.opencontainers.image.created="%BUILDTIME%"
LABEL org.opencontainers.image.vendor="SUSE LLC"
LABEL org.opensuse.reference=$IMAGE_REPO
LABEL org.openbuildservice.disturl="%DISTURL%"
LABEL com.suse.supportlevel="techpreview"
# endlabelprefix

ENTRYPOINT ["elemental-httpfy"]
