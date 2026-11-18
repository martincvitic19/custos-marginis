# Build environment for Yocto Project 6.0 LTS "wrynose" targeting the
# Globalscale ESPRESSObin.
#
# Base: Ubuntu 26.04 LTS -- listed in SANITY_TESTED_DISTROS of meta-poky
# (yocto-6.0.4) and the newest LTS release on that list.
#
# Build:  scripts/container-build.sh
# Run:    scripts/container-run.sh
FROM ubuntu:26.04

ARG USERNAME=builder
ARG USER_UID=1000
ARG USER_GID=1000

ENV DEBIAN_FRONTEND=noninteractive

# Yocto 6.0 host packages, verbatim from the reference manual
# (documentation/tools/host_packages_scripts/ubuntu_essential.sh), plus
# python3-websockets (required on all distros except Debian 11/Ubuntu 22.04).
RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential chrpath cpio debianutils diffstat file gawk gcc git \
        iputils-ping libacl1 libcrypt-dev locales python3 python3-git \
        python3-jinja2 python3-pexpect python3-pip python3-subunit socat \
        texinfo unzip wget xz-utils zstd \
        python3-websockets \
    && apt-get install -y --no-install-recommends \
        ca-certificates iproute2 less procps \
        tmux picocom bmaptool openssh-client \
        tftpd-hpa \
    && rm -rf /var/lib/apt/lists/*

# UTF-8 locale required by bitbake
RUN echo "en_US.UTF-8 UTF-8" > /etc/locale.gen && locale-gen
ENV LANG=en_US.UTF-8 \
    LC_ALL=en_US.UTF-8

# Non-root build user with the host user's UID/GID, so files written to the
# bind-mounted project directory stay owned by the host user. The image ships
# an "ubuntu" user (UID/GID 1000) which would collide, so drop any user/group
# already holding the requested IDs. dialout lets the user open a passed-in
# serial device.
RUN set -e; \
    old_user=$(getent passwd "${USER_UID}" | cut -d: -f1); \
    if [ -n "$old_user" ]; then userdel -r "$old_user" 2>/dev/null || userdel "$old_user"; fi; \
    old_group=$(getent group "${USER_GID}" | cut -d: -f1); \
    if [ -n "$old_group" ]; then groupdel "$old_group"; fi; \
    groupadd -g "${USER_GID}" "${USERNAME}"; \
    useradd -m -s /bin/bash -u "${USER_UID}" -g "${USER_GID}" -G dialout "${USERNAME}"

USER ${USERNAME}

# bitbake-setup records its configuration in git, so give the user an identity.
RUN git config --global user.name "ESPRESSObin builder" \
    && git config --global user.email "builder@espressobin.invalid"

WORKDIR /home/${USERNAME}
CMD ["/bin/bash", "-l"]
