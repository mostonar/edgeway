# Copyright (c) 2022  The AdvantEDGE Authors
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM debian:12-slim

# Real inputs read by entrypoint.sh. In the MEC Sandbox/Helm deployment,
# these are injected automatically by Helm and override the empty
# defaults below. For local `docker run` testing, pass them via -e or
# --env-file (see docker_run.sh).
ENV MEEP_HOST_URL=""
ENV MEEP_SANDBOX_NAME=""
ENV MEEP_LOCATION_NAME=""
ENV MEEP_INSTANCE_ID=""
ENV MEEP_IPE_URL=""
ENV MEEP_IPE_PORT=""
ENV MEEP_HTTPS=""
ENV MEEP_CSE_URL=""
ENV MEEP_CSE_PORT=""
ENV MEEP_CSE_ID=""
ENV MEEP_CSE_NAME=""
ENV MEEP_INITIATED_MN_CSE=""
ENV MEEP_STANDALONE=""

COPY ./meep-ipe /meep-ipe
COPY ./data /
# COPY config.yaml.in /

RUN apt-get update && apt-get install -y --no-install-recommends \
    gettext \
    && apt-get autoremove --purge -y \
    && apt-get autoclean \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
RUN chmod +x /entrypoint.sh /meep-ipe

ENTRYPOINT ["/entrypoint.sh"]