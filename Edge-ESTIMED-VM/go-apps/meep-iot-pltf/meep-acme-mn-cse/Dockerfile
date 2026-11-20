FROM ubuntu:24.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential \
      ca-certificates \
      git \
      libffi-dev \
      libssl-dev \
      pkg-config \
      python3 \
      python3-dev \
      python3-pip \
      python3-venv \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app

# --depth 1: skip full git history, much faster/smaller clone
RUN git clone --depth 1 https://github.com/ankraft/ACME-oneM2M-CSE.git ACME-oneM2M-CSE \
    && rm -rf ACME-oneM2M-CSE/.git

WORKDIR /usr/src/app/ACME-oneM2M-CSE

# Isolated venv sidesteps Ubuntu 24.04's PEP 668 "externally-managed-environment" pip error
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"

# Copy requirements first so this layer only rebuilds when deps actually change
COPY ./data/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir -r requirements.txt

COPY ./data ./

RUN cp -r ./acme/* ./acmecse/ \
    && rm -rf ./acme \
    && chmod +x entrypoint.sh

# ---------- Runtime stage ----------
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Only what's needed to *run* the CSE, not build it
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      gettext \
      git \
      gnutls-bin \
      iputils-ping \
      jq \
      libedit2 \
      libglib2.0-0 \
      lsof \
      python3 \
      sudo \
      tzdata \
    && apt-get autoremove --purge -y \
    && apt-get autoclean \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN echo "meep-acme-mn-cse" > /etc/hostname

WORKDIR /usr/src/app/ACME-oneM2M-CSE

# Pull in the pre-built venv and the app code from the builder stage
COPY --from=builder /opt/venv /opt/venv
COPY --from=builder /usr/src/app/ACME-oneM2M-CSE ./

RUN ln -sf /opt/venv/bin/python3 /usr/local/bin/python3 \
    && ln -sf /opt/venv/bin/pip /usr/local/bin/pip

ENV PATH="/opt/venv/bin:$PATH"

ENV SERVER_TYPE="" \
    SERVER_IP="" \
    SERVER_PORT="" \
    CSE_BASE_NAME="" \
    CSE_BASE_RI="" \
    REMOTE_CSE_ID="" \
    REMOTE_CSE_NAME="" \
    REMOTE_CSE_HOST="" \
    REMOTE_CSE_PORT="" \
    EXTERNAL_REMOTE_CSE_HOST="" \
    EXTERNAL_REMOTE_CSE_PORT="" \
    EXTERNAL_REMOTE_CSE_PROTOCOL="" \
    EXTERNAL_REMOTE_SVC_PATH="" \
    REMOTE_CSE_USER="" \
    REMOTE_CSE_PASS="" \
    MQTT_ENABLE="" \
    MQTT_HOST="" \
    MQTT_PORT="" \
    MQTT_USERNAME="" \
    MQTT_PASSWORD="" \
    MEC_ENABLE="" \
    MEC_HOST_URL="" \
    MEC_PLATFORM="" \
    MEC_SANDBOX_ID="" \
    ENABLE_COAP="" \
    ENABLE_WS="" \
    ACME_IN_SERVICE_NAME=""

ENTRYPOINT ["./entrypoint.sh"]