# SPDX-License-Identifier: Apache-2.0
# Define the names/tags of the container
#!BuildTag: rancher/elemental-operator/5.3:latest
#!BuildTag: rancher/elemental-operator/5.3:%OPERATOR_VERSION%
#!BuildTag: rancher/elemental-operator/5.3:%OPERATOR_VERSION%-%RELEASE%
#

FROM suse/sle15:15.4

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

RUN zypper in -y elemental-operator

# Save some space
RUN zypper clean --all && \
    rm -rf /var/log/update* && \
    >/var/log/lastlog && \
    rm -rf /boot/vmlinux*

USER 10010:10010

ENTRYPOINT ["elemental-operator"]
