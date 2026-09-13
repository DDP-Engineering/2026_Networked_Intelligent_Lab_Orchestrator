ARG NILO_BASE=debian@sha256:f324c7ff54321e8d9c588493a20244965938ce0aa50bbd1022d38010e9ffc4b1
FROM ${NILO_BASE} AS network-system

RUN apt-get update && apt-get install -y --no-install-recommends network-manager dbus iproute2 iputils-ping nftables ca-certificates gcc libc6-dev && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /run/dbus /etc/NetworkManager/conf.d && printf '[main]\nplugins=keyfile\nno-auto-default=*\n[device]\nmatch-device=interface-name:eth0\nmanaged=1\n' > /etc/NetworkManager/conf.d/nilo-test.conf

WORKDIR /repo/05_Software/nilo_assistant

# The appliance stage extends the deliberately clean network-install fixture.
# Keeping the targets distinct lets that fixture prove WireGuard installation
# while the browser stage has all of the tools an already provisioned appliance
# needs.
FROM network-system AS appliance

# Pinned in 02_Tools/toolchain.versions; 05_Software/scripts/tests/test_appliance_stage.py
# asserts these stay equal to that manifest, so the image never drifts from the
# versions the rest of the repository builds and tests against.
ARG NODE_VERSION=24.20.0
ARG NODE_LINUX_AMD64_SHA256=2f2c0da162318f0de47665410c7c8c2ed3d36c8f3105de4bbc61176c70a7cbf2
ARG PLAYWRIGHT_VERSION=1.62.1

ENV PLAYWRIGHT_BROWSERS_PATH=/opt/nilo/playwright \
    PATH=/opt/nilo/node/bin:${PATH}

RUN apt-get update && apt-get install -y --no-install-recommends curl xz-utils && rm -rf /var/lib/apt/lists/*

# Node.js and the browser engine cannot come from the read-only repository
# mount: 02_Tools/runtime/linux-x86_64/ carries only Go, uv and Python. The Go
# toolchain still comes from that mount through 02_Tools/env/with-go.sh.
RUN mkdir -p /opt/nilo/node /opt/nilo/playwright \
 && curl -fsSL -o /tmp/node.tar.xz "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
 && echo "${NODE_LINUX_AMD64_SHA256}  /tmp/node.tar.xz" | sha256sum -c - \
 && tar -xJf /tmp/node.tar.xz -C /opt/nilo/node --strip-components=1 \
 && rm /tmp/node.tar.xz \
 && npm install -g "playwright@${PLAYWRIGHT_VERSION}" \
 && playwright install --with-deps chromium \
 && rm -rf /var/lib/apt/lists/* /root/.npm

# The browser suite exercises the same userspace WireGuard lifecycle an
# appliance exposes. Keep it after the larger browser layer so adding or
# updating this small fixture dependency does not discard the pinned engine.
RUN apt-get update && apt-get install -y --no-install-recommends wireguard-tools && rm -rf /var/lib/apt/lists/*

# The interface project's locked dependencies. The checkout is mounted
# read-only at stage time, so nothing can be installed into it then; the build
# context is the repository root and Dockerfile.dockerignore keeps it to the two
# manifests this needs. The browser engine is already installed above, so the
# package's own download is skipped rather than duplicated.
COPY 05_Software/nilo_gui/package.json 05_Software/nilo_gui/package-lock.json /opt/nilo/gui-modules/
RUN cd /opt/nilo/gui-modules && PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm ci --no-audit --no-fund && rm -rf /root/.npm

WORKDIR /repo/05_Software/nilo_assistant
