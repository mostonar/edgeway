# MEC Sandbox Ansible Deployment Guide

This runbook provides step-by-step instructions for deploying the ETSI MEC Sandbox platform using Ansible.

---

## Prerequisites

Before running the playbooks, ensure you have:

1. **Ubuntu OS** (required by the setup script)
2. **Python 3** with `python3-venv` and `python3-pip` packages
3. **Both repositories** cloned as siblings:
   - `~/etsi-mec-sandbox` (backend)
   - `~/etsi-mec-sandbox-frontend` (frontend)
4. **GitHub OAuth Application** credentials (Client ID & Client Secret)
5. A **target IP address or domain** for your MEC Sandbox installation

---

## Environment Setup (Required First Step)

Before running any playbooks, you must set up the Ansible environment:

```bash
# Make the setup script executable
chmod +x ~/etsi-mec-sandbox/deploy/ansible/setup_ansible_env.sh

# Navigate to the playbooks directory
cd ~/etsi-mec-sandbox/deploy/ansible

# Run the setup script
./setup_ansible_env.sh

# Activate the virtual environment
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate
```

The setup script:
- Creates a Python virtual environment (`ansible-venv`)
- Installs `pip`, `ansible`, and `kubernetes` Python packages
- Installs Ansible collections from `collections/requirements.yml`:
  - `community.general`
  - `ansible.posix`
  - `community.docker`
  - `kubernetes.core`
- Updates `.gitignore` to exclude the virtual environment

---

## Inventory Layout

- **k8s_masters** → Control plane (API server, etcd, scheduler, controller-manager)
- **k8s_workers** → Optional worker nodes (run pods, kubelet, container runtime)

Example `inventories/dev/hosts.ini`:
```ini
[k8s_masters]
localhost ansible_connection=local ansible_python_interpreter=auto_silent ansible_user=<your-username>

[k8s_workers]
# worker1 ansible_host=192.168.1.11 ansible_user=ubuntu
# worker2 ansible_host=192.168.1.12 ansible_user=ubuntu

[all:vars]
ansible_become=true
ansible_become_method=sudo
```

---

## Quick Start (Single-Node Deployment)

### Step 1: Setup Environment (if not done)

```bash
chmod +x ~/etsi-mec-sandbox/deploy/ansible/setup_ansible_env.sh
cd ~/etsi-mec-sandbox/deploy/ansible
./setup_ansible_env.sh
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate
```

### Step 2: Run the Playbook

```bash
cd ~/etsi-mec-sandbox/deploy/ansible
ansible-playbook -i inventories/dev/hosts.ini site.yml
```

You will be prompted for:
- **Sudo password**: Your local sudo password
- **MEC host address**: IP or domain (e.g., `192.168.1.100` or `mec.example.com`)
- **GitHub OAuth Client ID**: From your GitHub OAuth app
- **GitHub OAuth Client Secret**: From your GitHub OAuth app

### Step 3: Verify Deployment

After successful completion, access the MEC Sandbox at:
```
https://<your-mec-host-address>
```

---

## Execution Flow

The playbook executes the following roles in order:

| Order | Role                         | Description                                      |
|-------|------------------------------|--------------------------------------------------|
| 1     | common                       | Base packages, APT keyring setup                 |
| 2     | kernel                       | Disable swap, kernel modules, sysctl tuning      |
| 3     | containerd                   | Install & configure containerd with SystemdCgroup|
| 4     | docker                       | Docker engine installation & daemon config       |
| 5     | kubernetes/master            | Initialize Kubernetes control plane (kubeadm init)|
| 6     | cni_calico                   | Deploy Calico CNI via Tigera operator            |
| 7     | helm                         | Install Helm package manager (via snap)          |
| 8     | dev_env/golang (conditional) | Go development environment + GolangCI-Lint       |
| 9     | dev_env/node (conditional)   | Node.js/NVM environment                          |
| 10    | mec_sandbox/mec_config       | Configure MEC Sandbox (charts, secrets, OAuth)   |
| 11    | mec_sandbox/mec_deploy       | Build and deploy MEC Sandbox components          |

---

## MEC Sandbox Deployment Details

### mec_sandbox/mec_config Role

This role configures the MEC Sandbox environment:

1. **Adds kubectl bash completion** to `.bashrc`
2. **Updates /etc/hosts** with docker registry entry (`meep-docker-registry`)
3. **Copies Kubernetes CA** to system trust store (`/usr/local/share/ca-certificates/`)
4. **Runs `update-ca-certificates`** to refresh system CA store
5. **Restarts docker and containerd** daemons
6. **Patches chart values** (uid/gid 1001 → 1000):
   - `charts/data-stores/postgis/values.yaml`
   - `charts/data-stores/redis/values.yaml`
   - `charts/data-stores/docker-registry/values.yaml`
7. **Patches frontend config** (uid/gid 1001 → 1000):
   - `etsi-mec-sandbox-frontend/config/.meepctl-repocfg.yaml`
8. **Updates GitHub OAuth credentials** in `secrets.yaml`
9. **Updates ingress host address** in `.meepctl-repocfg.yaml`

### mec_sandbox/mec_deploy Role

This role builds and deploys all MEC Sandbox components:

1. **Install meepctl**: Runs `install.sh` from `go-apps/meepctl`
2. **Verify meepctl**: Checks `meepctl version` is available
3. **Configure meepctl**:
   ```bash
   meepctl config ip <mec-host-address>
   meepctl config gitdir <sandbox-dir>
   ```
4. **Build & Deploy Frontend**:
   ```bash
   cd etsi-mec-sandbox-frontend && bash build.sh && bash deploy.sh
   ```
5. **Configure Secrets**: Runs `configure-secrets.py set`
6. **Deploy Dependencies**: `meepctl deploy dep all` (with up to 3 retries using `-f` flag)
7. **Build All**: `meepctl build --nolint all`
8. **Dockerize All**: `meepctl dockerize all` (runs via `sg docker`)
9. **Prune Docker Images**: `docker image prune -f`
10. **Deploy Core**: `meepctl deploy core all`

---

## Multi-node (Masters + Optional Workers)

If you want to add worker nodes (separate machines), follow these steps:

1. On each worker node, ensure SSH access is configured and Ansible can reach them.

2. Edit `inventories/dev/hosts.ini` and add entries under `[k8s_workers]`:
   ```ini
   [k8s_workers]
   worker1 ansible_host=192.168.56.11 ansible_user=ubuntu
   worker2 ansible_host=192.168.56.12 ansible_user=ubuntu
   ```

3. Uncomment the worker play in `site.yml`:
   ```yaml
   - hosts: k8s_workers
     become: true
     vars_prompt:
       - name: ansible_become_pass
         prompt: "Enter sudo password for workers"
         private: true
     roles:
       - common
       - kernel
       - containerd
       - kubernetes/common
       - kubernetes/worker
   ```

4. Run the playbook for master first (to initialize control plane and produce join script):
   ```bash
   ansible-playbook -l k8s_masters site.yml
   ```
   After successful run, a join command will be generated at `/tmp/kubeadm_join.sh`.

5. Copy the `/tmp/kubeadm_join.sh` to each worker node:
   ```bash
   scp /tmp/kubeadm_join.sh user@worker1:/tmp/kubeadm_join.sh
   ```

6. Run the worker play:
   ```bash
   ansible-playbook -l k8s_workers site.yml
   ```

---

## Conditional Roles

The following roles can be enabled/disabled via variables in `group_vars/all.yml`:

| Variable             | Default | Description                          |
|----------------------|---------|--------------------------------------|
| `install_dev_env`    | `true`  | Install Go and Node.js environments  |
| `install_mec_sandbox`| `true`  | Configure and deploy MEC Sandbox     |

To skip MEC Sandbox deployment:
```bash
ansible-playbook -i inventories/dev/hosts.ini site.yml -e "install_mec_sandbox=false"
```

To skip development environment setup:
```bash
ansible-playbook -i inventories/dev/hosts.ini site.yml -e "install_dev_env=false"
```

---

## Troubleshooting

### Virtual Environment Not Activated
If you see "ansible: command not found", activate the virtual environment:
```bash
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate
```

### Thanos/Prometheus Deployment Failures
During `meepctl deploy dep all`, thanos and prometheus failures are **expected and ignored**. The deployment will continue.

### Repository Not Found Errors
Ensure both repositories are cloned as siblings:
```
~/etsi-mec-sandbox/
~/etsi-mec-sandbox-frontend/
```

### Permission Issues (uid/gid 1001)
The `mec_config` role automatically patches chart values from uid/gid 1001 → 1000. If you still encounter issues, verify the patches were applied:
```bash
grep -r "runAsUser\|fsGroup" ~/etsi-mec-sandbox/charts/*/*/values.yaml
```

### Docker Group Issues
If dockerize fails, ensure your user is in the docker group:
```bash
sudo usermod -aG docker $USER
newgrp docker
```

### Kubernetes Collection Errors
If you see errors about `kubernetes.core.k8s`, ensure collections are installed:
```bash
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate
ansible-galaxy collection install -r collections/requirements.yml
```

---

## Logs

Deployment logs are saved to `/tmp/`:
- `/tmp/meepctl_deploy_dep.log` (and retry logs)
- `/tmp/meepctl_build.log`
- `/tmp/meepctl_dockerize.log`
- `/tmp/meepctl_deploy_core.log`

---

## Key Variables

Default values from `inventories/dev/group_vars/all.yml`:

| Variable              | Default Value        |
|-----------------------|----------------------|
| `kubernetes_version`  | `v1.35.1`            |
| `calico_version`      | `v3.31.4`            |
| `containerd_version`  | `2.2.5-1~ubuntu.22.04~jammy` |
| `pod_network_cidr`    | `192.168.0.0/16`     |
| `go_version`          | `1.17`               |
| `node_version`        | `12.19.0`            |
| `npm_version`         | `6.14.8`             |
| `eslint_version`      | `5.16.0`             |

---

## Notes

- **Always run `setup_ansible_env.sh` first** and activate the virtual environment
- Worker nodes will only run `common`, `kernel`, `containerd`, `kubernetes/common`, and `kubernetes/worker` roles
- The `kubernetes/worker` role expects a join script (created on master) at `/tmp/kubeadm_join.sh`
- The MEC Sandbox deployment requires significant resources; ensure adequate CPU, memory, and disk space
- The playbook uses `vars_prompt` for interactive input; for automation, pass variables via `-e` flag