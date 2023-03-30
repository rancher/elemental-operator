# SPDX-License-Identifier: Apache-2.0
# Define the names/tags of the container
#!BuildTag: rancher/elemental-operator/5.3:latest
#!BuildTag: rancher/elemental-operator/5.3:%OPERATOR_VERSION%
#!BuildTag: rancher/elemental-operator/5.3:%OPERATOR_VERSION%-%RELEASE%
#

FROM suse/sle15:15.4 as BASE

RUN mkdir /installroot
RUN zypper --installroot /installroot in -y elemental-operator

FROM scratch as OPERATOR

COPY --from=BASE /installroot /

# Define labels according to https://en.opensuse.org/Building_derived_containers
# labelprefix=com.rancher.elemental
LABEL org.opencontainers.image.title="Rancher Elemental Operator"
LABEL org.opencontainers.image.description="Implements the Elemental endpoint on the management cluster"
LABEL org.opencontainers.image.version="%OPERATOR_VERSION%"
LABEL org.opencontainers.image.url="https://github.com/rancher/elemental-operator"
LABEL org.opencontainers.image.created="%BUILDTIME%"
LABEL org.opencontainers.image.vendor="SUSE LLC"
LABEL org.opensuse.reference="%%IMG_REPO%%/rancher/elemental-operator/5.3"
LABEL org.openbuildservice.disturl="%DISTURL%"
LABEL com.suse.supportlevel="techpreview"
# endlabelprefix

USER 10010:10010

ENTRYPOINT ["elemental-operator"]
