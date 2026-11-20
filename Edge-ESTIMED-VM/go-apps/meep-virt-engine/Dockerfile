# Copyright (c) 2024  The AdvantEDGE Authors
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
COPY ./meep-virt-engine /meep-virt-engine
COPY ./api /api
COPY ./user-api /user-api
COPY ./data /

ENV HELM_VERSION="v3.10.1"
RUN mkdir -p /active \
    && apt-get update \
    && apt-get install -y wget \
    && wget -q https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz -O - | tar -xzO linux-amd64/helm > /usr/local/bin/helm \
    && chmod +x /usr/local/bin/helm \
    && wget -q https://dl.k8s.io/release/v1.28.4/bin/linux/amd64/kubectl -O /usr/local/bin/kubectl \
    && chmod +x /usr/local/bin/kubectl \
    && apt-get purge -y --auto-remove wget \
    && rm -rf /var/lib/apt/lists/* \
    && chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
