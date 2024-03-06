# SPDX-License-Identifier: Apache-2.0
# Define the names/tags of the container
#!BuildTag: rancher/elemental-operator:latest
#!BuildTag: rancher/elemental-operator:%OPERATOR_VERSION%
#!BuildTag: rancher/elemental-operator:%OPERATOR_VERSION%-%RELEASE%
#

ARG SLE_VERSION

FROM suse/sle-micro/$SLE_VERSION/toolbox as BASE

RUN mkdir /installroot
RUN zypper --installroot /installroot in -y --no-recommends \
      elemental-operator ca-certificates ca-certificates-mozilla

FROM scratch as OPERATOR

COPY --from=BASE /installroot /

RUN /usr/sbin/update-ca-certificates

ARG BUILD_REPO=%%IMG_REPO%%
ARG IMAGE_REPO=$BUILD_REPO/rancher/elemental-operator

# Define labels according to https://en.opensuse.org/Building_derived_containers
# labelprefix=com.suse.elemental
LABEL org.opencontainers.image.title="Rancher Elemental Operator"
LABEL org.opencontainers.image.description="Implements the Elemental endpoint on the management cluster"
LABEL org.opencontainers.image.version="%OPERATOR_VERSION%"
LABEL org.opencontainers.image.url="https://github.com/rancher/elemental-operator"
LABEL org.opencontainers.image.created="%BUILDTIME%"
LABEL org.opencontainers.image.vendor="SUSE LLC"
LABEL org.opensuse.reference=$IMAGE_REPO
LABEL org.openbuildservice.disturl="%DISTURL%"
LABEL com.suse.supportlevel="l3"
# endlabelprefix

USER 10010:10010

ENTRYPOINT ["elemental-operator"]
