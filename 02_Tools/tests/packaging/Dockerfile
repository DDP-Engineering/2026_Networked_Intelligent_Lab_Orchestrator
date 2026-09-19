FROM nilo-appliance-stage:local

ARG GENERATION
ARG HOST_UID
ARG HOST_GID
COPY . /opt/nilo/python
RUN mkdir -p /opt/nilo/cache-seed /work/runtime-cache /work/source \
    && chown "${HOST_UID}:${HOST_GID}" /work/runtime-cache /work/source \
    && ln -s /opt/nilo/python/bin/python3 /usr/local/bin/python

ENV PATH="/opt/nilo/python/bin:${PATH}"
LABEL io.nilo.build-environment.generation="${GENERATION}"
