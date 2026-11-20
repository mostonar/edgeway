# Stage 1: Builder
FROM debian:13-slim AS builder

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  build-essential cmake curl dos2unix gcc g++ git libssl-dev \
  libcurl4-openssl-dev libwebsockets-dev libxml2-dev libjsoncpp-dev \
  pkg-config gettext jq tzdata && \
  apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app

# Single deterministic copy
COPY ./data/ /usr/src/app/tinyIoT/

# Inject getenv fallbacks into config.h.in before building
RUN set -e && \
  cd /usr/src/app/tinyIoT/source/server && \
  sed -i '1i #include <stdlib.h>' config.h.in && \
  sed -i 's/"${SERVER_IP}"/(getenv("SERVER_IP") ? getenv("SERVER_IP") : "0.0.0.0")/g' config.h.in && \
  sed -i 's/"${SERVER_PORT}"/(getenv("SERVER_PORT") ? getenv("SERVER_PORT") : "3002")/g' config.h.in && \
  sed -i 's/"${MQTT_HOST}"/(getenv("MQTT_HOST") ? getenv("MQTT_HOST") : "localhost")/g' config.h.in && \
  sed -i 's/"${MQTT_USERNAME}"/(getenv("MQTT_USERNAME") ? getenv("MQTT_USERNAME") : "")/g' config.h.in && \
  sed -i 's/"${MQTT_PASSWORD}"/(getenv("MQTT_PASSWORD") ? getenv("MQTT_PASSWORD") : "")/g' config.h.in && \
  sed -i 's/${MQTT_PORT}/(getenv("MQTT_PORT") ? atoi(getenv("MQTT_PORT")) : 1883)/g' config.h.in && \
  sed -i 's/char \*PORT = SERVER_PORT;/char \*PORT = NULL;/g' main.c && \
  sed -i 's/bool initialBoot = false;/bool initialBoot = false; PORT = SERVER_PORT;/g' main.c && \
  export SERVER_TYPE="IN_CSE" && \
  export MQTT_ENABLE="1" && \
  export ENABLE_COAP="0" && \
  export CSE_BASE_NAME="TinyIoT" && \
  export CSE_BASE_RI="tinyiot-in" && \
  export ALLOWED_REMOTE_CSE_ID="/tinyiot,/tinyiot-mn-cse,/tinyiot-in-cse,/id-in,/id-mn" && \
  envsubst < config.h.in > config.h && \
  make clean && \
  make

# Stage 2: Final Image
FROM debian:13-slim

RUN DEBIAN_FRONTEND=noninteractive apt-get update && \
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
  curl libssl3 libcurl4 libwebsockets19 libxml2 libjsoncpp26 jq gettext tzdata ca-certificates && \
  apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app

# Copy the compiled binary and necessary runtime files
COPY --from=builder /usr/src/app/tinyIoT/source/server/server /usr/src/app/tinyIoT/source/server/server
COPY --from=builder /usr/src/app/tinyIoT/entrypoint.sh /usr/src/app/tinyIoT/entrypoint.sh

# Ensure entrypoint is executable
RUN chmod +x /usr/src/app/tinyIoT/entrypoint.sh

ENTRYPOINT ["/usr/src/app/tinyIoT/entrypoint.sh"]