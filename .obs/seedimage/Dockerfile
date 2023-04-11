# SPDX-License-Identifier: Apache-2.0
# Define the names/tags of the container
#!BuildTag: rancher/seedimage-builder/5.3:latest
#!BuildTag: rancher/seedimage-builder/5.3:%OPERATOR_VERSION%
#!BuildTag: rancher/seedimage-builder/5.3:%OPERATOR_VERSION%-%RELEASE%
#

FROM suse/sle15:15.4 as BASE

RUN  mkdir -p /installroot && \
    zypper --gpg-auto-import-keys --installroot /installroot in -y --no-recommends elemental-httpfy xorriso curl coreutils ca-certificates ca-certificates-mozilla


FROM scratch as SEEDIMAGE_BUILDER

COPY --from=BASE /installroot /

RUN /usr/sbin/update-ca-certificates

# Define labels according to https://en.opensuse.org/Building_derived_containers
# labelprefix=com.rancher.elemental
LABEL org.opencontainers.image.title="Rancher Elemental Seed Image Builder"
LABEL org.opencontainers.image.description="Provides the tools needed to build Elemental Seed Images"
LABEL org.opencontainers.image.version="%OPERATOR_VERSION%"
LABEL org.opencontainers.image.url="https://github.com/rancher/elemental-operator"
LABEL org.opencontainers.image.created="%BUILDTIME%"
LABEL org.opencontainers.image.vendor="SUSE LLC"
LABEL org.opensuse.reference="%%IMG_REPO%%/rancher/elemental-seedimage-builder-image/5.3"
LABEL org.openbuildservice.disturl="%DISTURL%"
LABEL com.suse.supportlevel="techpreview"
# endlabelprefix

ENTRYPOINT ["elemental-httpfy"]
