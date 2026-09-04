ARG NILO_BASE=debian@sha256:f324c7ff54321e8d9c588493a20244965938ce0aa50bbd1022d38010e9ffc4b1
FROM ${NILO_BASE}
RUN apt-get update && apt-get install -y --no-install-recommends network-manager dbus iproute2 iputils-ping nftables ca-certificates gcc libc6-dev && rm -rf /var/lib/apt/lists/*
RUN mkdir -p /run/dbus /etc/NetworkManager/conf.d && printf '[main]\nplugins=keyfile\nno-auto-default=*\n[device]\nmatch-device=interface-name:eth0\nmanaged=1\n' > /etc/NetworkManager/conf.d/nilo-test.conf
WORKDIR /repo/05_Software/nilo_assistant
