FROM eclipse-mosquitto:latest

# Install some libraries
RUN apk update && apk add curl jq

RUN mkdir -p /run/mosquitto
# Create config and log directories
RUN mkdir -p /mosquitto/config/conf.d \
    && mkdir -p /mosquitto/data /mosquitto/log
# Copy the main mosquitto.conf
COPY data/mosquitto.conf /mosquitto/config/mosquitto.conf
# Copy additional listener config
COPY data/listener.conf /mosquitto/config/conf.d/listener.conf

WORKDIR /usr/src/app
COPY ./data /usr/src/app
RUN chmod +x /usr/src/app/entrypoint.sh
ENTRYPOINT [ "/usr/src/app/entrypoint.sh" ]
