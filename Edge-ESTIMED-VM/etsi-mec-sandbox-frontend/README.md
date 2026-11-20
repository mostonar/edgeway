'# ETSI MEC Sandbox

>_**NOTE: This repository contains the MEC Sandbox Frontend; the backend portion of the MEC Sandbox is realized
> using the open source project [AdvantEDGE](https://github.com/InterDigitalInc/AdvantEDGE)**_

MEC Sandbox is an interactive environment that enables users to learn & experiment with ETSI MEC Service APIs.
These standardised RESTful APIs are targeted towards MEC application developers to expose the value added services
offered by MEC, including real time access to network and context information, as well as location awareness.
The design principles for developing the APIs have also been specified in ETSI GS MEC 009, along with http methods,
templates, conventions and patterns. The MEC service APIs are available in YAML and JSON format at https://forge.etsi.org,
presented via OpenAPI compliant descriptions.

MEC Sandbox provides the user with a choice of scenarios combining different network technologies (4G, 5G, Wi-Fi) and
terminal types. Combining these assets in a geolocated environment, a user can gain hands-on experience on the behaviour
and capabilities of the Location (MEC013), Radio Network Information (MEC012), Traffic Management APIs (MEC015), Device Application Interface (MEC016), 
WLAN Information (MEC028), Edge Platform Application Enablement (MEC011), Application Mobility (MEC021) service APIs, V2X Information API (MEC030) service API,  
MEC IoT API (MEC 033), MEC Federation Service API (MEC040) service APIs and Sensors-Sharing Services (MEC046) service APIs. 
MEC Sandbox provides also a support for Edge Native Connector (3GPP TS 29.222: 3rd Generation Partnership Project; Technical Specification GroupCore
Network and Terminals; Common API Framework for 3GPP Northbound APIs). Such contextual information can offer
significant differential performance for edge based MEC applications.
Application Enablement (MEC011) and Application Mobility (MEC021) service APIs. Such contextual information can offer
significant differential performance for edge based MEC applications.

MEC Sandbox deploys the [AdvantEDGE edge emulation platform](https://github.com/InterDigitalInc/AdvantEDGE) to provide
real-time access to ETSI MEC service implementations.

The MEC Sandbox frontend (provided in this repo) uses the AdvantEDGE platform & service REST APIs to create secure,
user-specific sandboxes where signed-in users can:
- Deploy & dynamically configure a scenario
- Try-out the MEC Service APIs using Swagger UI
- Interact with the MEC Services directly from a user MEC Application

## Getting Started
To use the MEC Sandbox, check out the [live deployment](https://try-mec.etsi.org).

To learn more about the MEC Sandbox, have a look at the [wiki page](https://mecwiki.etsi.org/index.php?title=MEC_Sandbox_Help).

To install or upgrade the MEC Sandbox, follow the instructions below.

## Install MEC Sandbox
The following procedure should be used to install a new MEC Sandbox deployment.

### Prerequisites
MEC Sandbox has the following prerequisites:

1. MEC Sandbox (etsi-mec-sandbox + etsi-mec-sandbox-frontend) VM:
   - [Hardware requirements](https://interdigitalinc.github.io/AdvantEDGE/docs/setup/env-hw/)
1. Long-term Storage (MinIO + Thanos Compactor) VM:
   - [Hardware requirements](https://interdigitalinc.github.io/AdvantEDGE/docs/setup/env-hw/)
1. [etsi-mec-sandbox](https://interdigitalinc.github.io/AdvantEDGE/) runtime & development environments on both VMs
   - Install [runtime environment](guides/runtime-environment-setup.md)
   - Install [development environment](guides/development-environment-setup.md)
   - Or deploy both using [Ansible Playbooks](../etsi-mec-sandbox/playbooks/README.md)
2. OAuth Applications using external providers
   - **GitHub:**
     - github.com User Account --> Settings --> Developer settings --> OAuth Apps --> New OAuth App
     - _Application Name:_ ETSI MEC Sandbox
     - _Homepage URL:_ https://try-mec.etsi.org
     - _Application description:_ Optional description...
     - _Authorization callback URL:_ https://try-mec.etsi.org/platform-ctrl/v1/authorize
     - **NOTES:**
       - You can also add an application logo if you want users to see one during authentication
       - Once created, keep note of the OAuth App Client ID & Secret; this secret is only temporarily available on GitHub... keep a copy in a safe place!
       - If you need to generate a new secret the MEC Sandbox will have to be updated accordingly
       - Enable Device Flow to use API driven sandbox
   - **GitLab:**
     - gitlab.com User Account --> Settings --> Applications --> Add new application
     - _Name:_ ETSI MEC Sandbox
     - _Redirect URI:_ https://try-mec.etsi.org/platform-ctrl/v1/authorize
     - _Confidential:_ Unchecked
     - _Scopes:_ read_user
     - **NOTES:**
       - Once created, keep note of the OAuth App Client ID & Secret
       - Enable Device Flow to use API driven sandbox
3. Python Installation
   - Install Python2:
     - Ubuntu 18.04: `sudo apt install python-minimal`
     - Ubuntu 20.04: `sudo apt install python2-minimal`
   - Install Python-pip:
     - Ubuntu 18.04: `sudo apt install python-pip`
     - Ubuntu 20.04: `sudo apt install python3-pip`
   - Install Pyyaml: `pip install pyyaml`
4. Host NFS server installation
In order to be deployed properly, MEC 016 service requires that an NFS server was deployed on the host. To achieve it, the followimg steps are required:

- Install the NFS server
```sh
$ sudo apt-get update
$ sudo apt install nfs-kernel-server nfs-common portmap
```
- Check that the NFS server is running on the host
```sh
$ sudo systemctl status nfs-server
```
The outpout looks like:
```
nfs-server.service - NFS server and services
     Loaded: loaded (/lib/systemd/system/nfs-server.service; enabled; vendor preset: enabled)
     Active: active (exited) since Mon 2024-09-16 09:10:45 CEST; 42s ago
   Main PID: 923678 (code=exited, status=0/SUCCESS)
      Tasks: 0 (limit: 38358)
     Memory: 0B
     CGroup: /system.slice/nfs-server.service
```
- Create the NFS shared folder
```sh
$ sudo mkdir -p /mnt/nfs/mec_sandbox
$ sudo chmod -R 777 /mnt/nfs/
```
- Update the /etc/exports file by adding the following line:
```sh
sudo echo '/mnt/nfs/mec_sandbox  *(rw,sync,no_subtree_check,no_root_squash,insecure)' > /etc/exports
```
- Export the NFS shared folder
```sh
$ sudo exportfs -rv
```
- Check that the NFS shared folder is exported
```sh
$ showmount -e
```

### Deploy Long-term Storage (MinIO + Thanos Compactor)

1. Clone [etsi-mec-sandbox](https://labs.etsi.org/rep/mec/etsi-mec-sandbox) repository:
   - Fetch required etsi-mec-sandbox baseline:
     - Clone etsi-mec-sandbox repo to the Long-term Storage VM
     - Place repo in home folder
     - Checkout the required etsi-mec-sandbox version tag
1. Configure Long-term Storage:
   - Configure object store:
     - `vi ~/etsi-mec-sandbox/config/secrets.yaml`
       - Set minio _accessKey_ to object store access key value (minimum 3 chars)
       - Set minio _secretKey_ to object store secret key value (minimum 8 chars)
     - `vi ~/etsi-mec-sandbox/config/objstore-thanos.yaml`
       - Set _endpoint_ to VM FQDN
         - **NOTE:** Make sure _/etc/hosts_ file contains entry for VM FQDN that points to VM private IP address
       - Set _access\_key_ to object store access key value
       - Set _secret\_key_ to object store secret key value
   - Configure deployment:
     - `vi ~/etsi-mec-sandbox/config/long-term-storage-repocfg.yaml`
       - Set uid & gid
       - Set host name to FQDN
       - Set Thanos compactor retention times
     - Overwrite repocfg file with provided long-term storage repocfg:
       - `cp ~/etsi-mec-sandbox/config/long-term-storage-repocfg.yaml ~/etsi-mec-sandbox/.meepctl-repocfg.yaml`
   - Configure MinIO helm chart:
     - `vi ~/etsi-mec-sandbox/charts/minio/values.yaml`
       - _securityContext_:
         - Set _runAsUser_ to uid
         - Set _runAsGroup_ to gid
         - Set _fsGroup_ to groups
1. Create MinIO & Thanos Secrets:
   - MinIO:
     - Install Python as described in the prerequisites
     - Create secrets:
       - Ubuntu 18.04: `python ~/etsi-mec-sandbox/config/configure-secrets.py set ~/etsi-mec-sandbox/config/secrets.yaml`
       - Ubuntu 20.04: `python3 ~/etsi-mec-sandbox/config/configure-secrets.py set ~/etsi-mec-sandbox/config/secrets.yaml`
   - Thanos:
     - `~/etsi-mec-sandbox/config/configure-objstore-secrets.sh`
   - **NOTE:** Once the kubernetes secrets have been created, it is recommended to remove the config files containing the clear-text secrets
1. Deploy MinIO & Thanos Compactor:
   - Build & Configure meepctl:
     - `~/etsi-mec-sandbox/go-apps/meepctl/install.sh`
     - `meepctl config ip <VM private IP address>`
     - `meepctl config gitdir <full path to ~/etsi-mec-sandbox>`
   - Deploy dependency pods:
     - `meepctl deploy dep all`
   - Deploy Core pods:
     - `meepctl deploy core all`
     - **NOTE:** Core pods deployment will only install the cert-manager rules to trigger certificate installation
1. Log In & Create buckets:
   - Using a browser, access the long-term storage VM via its FQDN
   - Sign in using the provisioned credentials (access key & secret key)
   - Create the following buckets if they do not exist:
     - influx
     - thanos
     - thanos-archive

### Build & Deploy MEC Sandbox (etsi-mec-sandbox + etsi-mec-sandbox-frontend)

1. Clone [etsi-mec-sandbox-frontend](https://labs.etsi.org/rep/mec/etsi-mec-sandbox-frontend) & [etsi-mec-sandbox](https://labs.etsi.org/rep/mec/etsi-mec-sandbox) repositories:
   - Fetch required etsi-mec-sandbox-frontend:
     - Clone this repo to the MEC Sandbox deployment VM
     - Place repo in home folder
   - Fetch required etsi-mec-sandbox baseline:
     - Clone etsi-mec-sandbox repo to the MEC Sandbox deployment VM
     - Place repo in home folder
     - Checkout the required etsi-mec-sandbox version tag
1. Configure MEC Sandbox:
   - Configure object store:
     - `vi ~/etsi-mec-sandbox-frontend/config/objstore-thanos.yaml` and `vi ~/etsi-mec-sandbox-frontend/config/objstore-thanos-archive.yaml`
       - Set _endpoint_ to long-term storage VM FQDN
         - **NOTE:** Make sure _/etc/hosts_ file contains entry for VM FQDN that points to VM private IP address (if on same local network)
       - Set _access\_key_ to object store access key value
       - Set _secret\_key_ to object store secret key value
     - `vi ~/etsi-mec-sandbox-frontend/config/objstore-influx.cfg`
       - Set _aws\_access\_key\_id_ to object store access key value
       - Set _aws\_secret\_access\_key_ to object store secret key value
   - Configure session & OAuth secrets:
     - `vi ~/etsi-mec-sandbox-frontend/config/secrets.yaml`
       - Set a unique session encryption key
       - Set the OAuth provider client ID & secrets using the values from the OAuth Applications
   - Configure deployment:
     - `vi ~/etsi-mec-sandbox-frontend/config/.meepctl-repocfg.yaml`
       - Set Prometheus external labels
         - **NOTE:** This uniquely identifies Prometheus data sources in the long-term storage data buckets
   - Configure Alert Manager slack channel webhook:
     - Create incoming webhook:
       - Create target slack channel in slack workspace
       - Go to `Manage Apps` in workspace settings
       - Search for `Incoming Webhooks` app
       - Click `Add to Slack`
         - **NOTE:** The webhook url can be found by looking at the app configuration
       - Choose a slack channel to post messages to
       - Click `Add Incoming WebHooks integration`
       - Copy the `Webhook URL`
       - **NOTE:** There should be a message in the selected slack channel indicating that an incoming webhook has be added
     - Set webhook url in Prometheus values override:
       - `vi ~/etsi-mec-sandbox-frontend/config/values/meep-prometheus.yaml`
       - Set `slack_api_url` to your slack channel webhook url
       - Set `channel` to your slack channel
   - Configure MQTT broker e.g `Mosquitto MQTT`:
     - Install mosquitto.
        - `sudo apt update`
        - `sudo apt install -y mosquitto`
     - Create a config file for mosquitto.
        - `echo -e "listener 1883\nallow_anonymous    true" | sudo tee /etc/mosquitto/conf.d/listener.conf > /dev/null`
     - Restart mosquitto.
        - `sudo systemctl restart mosquitto`
   - Configure vis and federation api to use mqtt broker
     - Set the `MEEP_BROKER` variable with the server IP (where Mosquitto is running) and port 1883 in the following two files:
        - `~/etsi-mec-sandbox/charts/meep-federation/values-template.yaml`
        - `~/etsi-mec-sandbox/charts/meep-vis/values-template.yaml`
        - Example: `MEEP_BROKER: mqtt://192.168.x.x:1882`
1. Build & Deploy STF Frontend:
   - SSH to MEC Sandbox deployment VM
   - Build STF Frontend:
     - `cd ~/etsi-mec-sandbox-frontend`
     - `./build.sh`
   - Deploy STF Frontend & Configuration files:
     - `./deploy.sh`
     - **NOTE:** This updates the `~/.meep/user/` folder with the user frontend, sandbox swagger UI, and user chart values.yaml overrides
     - **NOTE:** This also copies the etsi-mec-sandbox configuration files to their respective locations in your local etsi-mec-sandbox clone `~/etsi-mec-sandbox/`. Running `git status` in your etsi-mec-sandboxE clone will list the updated (or new) files.
1. Create Secrets:
   - Session & OAuth:
     - Install Python as described in the prerequisites
     - Create secrets:
       - Ubuntu 18.04: `python ~/etsi-mec-sandbox/config/configure-secrets.py set ~/etsi-mec-sandbox/config/secrets.yaml`
       - Ubuntu 20.04: `python3 ~/etsi-mec-sandbox/config/configure-secrets.py set ~/etsi-mec-sandbox/config/secrets.yaml`
   - Object Store (Thanos & Influx):
     - `~/etsi-mec-sandbox/config/configure-objstore-secrets.sh`
   - **NOTE:** Once the kubernetes secrets have been created, it is recommended to remove the config files containing the clear-text secrets
1. Build & Deploy Sandbox:
   - For more details see [AdvantEDGE Deployment Workflow](https://interdigitalinc.github.io/AdvantEDGE/docs/platform-mgmt/mgmt-workflow/)
   - Build demo4
     - `~/etsi-mec-sandbox/examples/demo4-ue/build-demo4-ue.sh`
   - Build and Configure meepctl:
     - `~/etsi-mec-sandbox/go-apps/meepctl/install.sh`
     - `meepctl config ip <VM private IP address>`
     - `meepctl config gitdir <full path to ~/etsi-mec-sandbox>`
   - Deploy dependency pods:
     - `meepctl deploy dep all`
   - Build & Dockerize:
     - `meepctl build all`
     - `meepctl dockerize all`
   - Prune old docker images:
     - `docker image prune`
   - Deploy Core pods:
     - `meepctl deploy core all`
     - **NOTE:** Wait for dep pods to be successfully deployed before deploying core

### Provision Admin User Accounts
Manage MEC Sandbox admin user accounts as follows:

1. SSH to MEC Sandbox machine
1. SSH into _meep-postgis-0_ pod
1. Login & connect to PostgreSQL user role database:
   - `psql -U postgres`
   - `\c meep_auth_svc`
1. Add entry to give _admin_ rights to a specific username from a specific provider. Examples:
   - `INSERT INTO users (provider, username, password, sboxname, role) VALUES ('github', 'user1', '', 'sbx-user1','admin');`
   - `INSERT INTO users (provider, username, password, sboxname, role) VALUES ('gitlab', 'user2', '', 'sbx-user2','admin');`
1. List configured user roles to verify proper configuration:
   - `SELECT * from users;`

### Configure Scenarios
If necessary, configure the MEC Sandbox scenarios as follows:

1. Sign in to MEC Sandbox using an account with _admin_ privileges
   - Login as admin user @ https://try-mec.etsi.org
1. Open the AdvantEDGE frontend
   - Open AdvantEDGE frontend @ https://try-mec.etsi.org/alt
1. Import & save MEC Sandbox scenarios
   - In the AdvantEDGE Configuration view, import the required scenario _yaml_ file
   - Save the imported scenario to the AdvantEDGE backend to make it available in the MEC Sandbox

## Upgrade MEC Sandbox
The following procedure should be used to upgrade a running MEC Sandbox deployment.

1. Stop MEC Sandbox
   - Delete user sandboxes
     - Using AdvantEDGE frontend:
       - Login as admin @ https://try-mec.etsi.org
       - Open AdvantEDGE frontend @ https://try-mec.etsi.org/alt
       - Go to Exec Page and delete user sandboxes
     - Manually:
       - `helm ls -A --short | grep <sandbox-name> | xargs -L1 helm delete`
   - Delete core & dependency microservices:
     - `meepctl delete core`
     - `meepctl delete dep`
     - **NOTE:** This will clear all active sessions
1. Pull latest updates from MEC Sandbox & AdvantEDGE repositories
1. Configure MEC Sandbox deployment:
   - Configure deployment:
     - `vi ~/etsi-mec-sandbox-frontend/config/.meepctl-repocfg.yaml`
       - Set Prometheus external labels
         - **NOTE:** This uniquely identifies Prometheus data sources in the long-term storage data buckets
   - Configure Alert Manager slack channel webhook:
     - Set webhook url in Prometheus values override:
       - `vi ~/etsi-mec-sandbox-frontend/config/values/meep-prometheus.yaml`
       - Set `slack_api_url` to your slack channel webhook url
       - Set `channel` to your slack channel
1. Build & Deploy STF Frontend:
   - SSH to MEC Sandbox deployment VM
   - Build STF Frontend:
     - `cd ~/etsi-mec-sandbox-frontend`
     - `./build.sh`
   - Deploy STF Frontend & Configuration files:
     - `./deploy.sh`
     - **NOTE:** This updates the `~/.meep/user/` folder with the user frontend, sandbox swagger UI, and user chart values.yaml overrides
     - **NOTE:** This also copies the etsi-mec-sandbox configuration files to their respective locations in your local AdvantEDGE clone `~/etsi-mec-sandbox/`. Running `git status` in your AdvantEDGE clone will list the updated (or new) files.
1. Build & Deploy MEC Sandbox:
   - For more details see [AdvantEDGE Deployment Workflow](https://interdigitalinc.github.io/AdvantEDGE/docs/platform-mgmt/mgmt-workflow/)
   - Build meepctl:
     - `~/etsi-mec-sandbox/go-apps/meepctl/install.sh`
   - Deploy dependency pods:
     - `meepctl deploy dep all`
   - Build & Dockerize:
     - `meepctl build all`
     - `meepctl dockerize all`
   - Prune old docker images:
     - `docker image prune`
   - Deploy Core pods:
     - `meepctl deploy core all`
     - **NOTE:** Wait for dep pods to be successfully deployed before deploying core
1. Update network scenarios (if necessary)

## Troubleshooting & Operations

For detailed troubleshooting, restart procedures, and operational commands, see the **[meepctl Troubleshooting & Operations Guide](docs/meepctl-troubleshooting.md)**.

### Quick Reference — Restarting Individual Components

If a specific component fails, you can restart it individually without affecting the rest of the platform:

```bash
# Restart a dependency (e.g. Grafana, InfluxDB, Prometheus)
meepctl deploy dep meep-grafana --force

# Restart a core app (e.g. platform-ctrl, mon-engine)
meepctl deploy core meep-platform-ctrl --force

# Restart a core app after code change
meepctl build meep-platform-ctrl
meepctl dockerize meep-platform-ctrl
meepctl deploy core meep-platform-ctrl --force
```

### Quick Reference — After Code or Chart Changes

| What changed | What to do |
|-------------|------------|
| Go code of a MEC API service (e.g. `go-apps/meep-ams/`) | `meepctl build meep-ams` → `meepctl dockerize meep-ams` → recreate sandbox from UI |
| Helm chart of a MEC API service (e.g. `charts/meep-ams/`) | `meepctl dockerize meep-virt-engine` → `meepctl deploy core meep-virt-engine -f` → recreate sandbox |
| Go code of a core app | `meepctl build <app>` → `meepctl dockerize <app>` → `meepctl deploy core <app> -f` |
| Helm chart of a core/dep app | `meepctl deploy core <app> -f` or `meepctl deploy dep <app> -f` |

For the full decision tree, MEC standard ↔ app name mapping, and common failure scenarios, see the [complete guide](docs/meepctl-troubleshooting.md).

## Automated Dev Workflow (Tilt)

For automated rebuild and redeployment during development, the MEC Sandbox includes a **[Tiltfile](https://tilt.dev/)** that watches your source code and Helm charts for changes, then runs the correct `meepctl` pipeline automatically.

For full documentation, see the **[Tilt Dev Workflow Guide](docs/tilt-dev-workflow.md)**.

### Quick Start

```bash
cd ~/etsi-mec-sandbox
tilt up                  # Start watching (platform must already be deployed)
# Open http://localhost:10350 for the Tilt dashboard
```

### What Tilt manages

| You edit | Trigger type | What Tilt runs |
|----------|--------------|----------------|
| `go-apps/meep-iot/` (Go code) | **Manual** (from GUI button) | `meepctl build` → `meepctl dockerize` → restart K8s pods |
| `charts/meep-iot/` (Helm chart) | **Manual** | `meepctl dockerize meep-virt-engine` → `meepctl deploy core meep-virt-engine -f` |
| `go-apps/meep-platform-ctrl/` (core Go code) | **Manual**  | `meepctl build` → `meepctl dockerize` → `meepctl deploy core -f` |
| `charts/meep-platform-ctrl/` (core chart) | **Manual** | `meepctl deploy core meep-platform-ctrl -f` |

Dependencies (Grafana, CouchDB, etc.) are available as **manual buttons** in the Tilt UI dashboard.

For configuration options, troubleshooting, and the full automation rules, see the [complete Tilt guide](docs/tilt-dev-workflow.md).

## Licensing
MEC Sandbox is a private repo with all rights reserved to ETSI.
```
Copyright (c) 2022-2025 ETSI.  All rights reserved.
```
