# ETSI MEC Sandbox Ansible Setup

This folder provides an **Ansible-based automation framework** to set up a multi-node Kubernetes cluster and deploy the ETSI MEC Sandbox platform.

---

## Pre-requisites

Before running the playbooks, ensure:

1. **Ubuntu OS** (required by the setup script)
2. **Python 3** with `python3-venv` and `python3-pip` packages
3. Both repositories cloned as siblings:
   - `etsi-mec-sandbox` (backend)
   - `etsi-mec-sandbox-frontend` (frontend)
4. A **GitHub OAuth application** configured (Client ID & Secret)

> **Note:** SSH setup is only required for remote worker nodes, not for localhost deployments.

---

## Environment Setup (Required)

Before running any playbooks, set up the Ansible environment:

```bash
chmod +x ~/etsi-mec-sandbox/deploy/ansible/setup_ansible_env.sh
cd ~/etsi-mec-sandbox/deploy/ansible
./setup_ansible_env.sh
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate
```

---

## Quick Start

```bash
# Activate virtual environment
source ~/etsi-mec-sandbox/deploy/ansible/ansible-venv/bin/activate

# Run the playbook
cd ~/etsi-mec-sandbox/deploy/ansible
ansible-playbook -i inventories/dev/hosts.ini site.yml
```

You will be prompted for:
- Sudo password
- MEC host IP/domain
- GitHub OAuth Client ID & Secret

> **For detailed deployment instructions**, see [RUNBOOK.md](RUNBOOK.md)

---

## Folder Structure

```
deploy/ansible/
├── setup_ansible_env.sh        # Environment setup script (run first!)
├── site.yml                    # Main playbook entrypoint
├── ansible.cfg                 # Ansible configuration
├── collections/requirements.yml
├── inventories/dev/
│   ├── hosts.ini               # Inventory (hosts & groups)
│   └── group_vars/all.yml      # Variables
└── roles/                      # Ansible roles (see below)
```

---

## Roles Overview

| Role                         | Purpose                                   |
| ---------------------------- | ----------------------------------------- |
| **common**                   | Base system packages                      |
| **kernel**                   | Kernel modules & sysctl tuning            |
| **containerd**               | Containerd runtime                        |
| **docker**                   | Docker engine                             |
| **cni\_calico**              | Calico CNI networking                     |
| **kubernetes/master**        | Initialize Kubernetes control plane       |
| **kubernetes/worker**        | Join worker nodes to cluster              |
| **helm**                     | Helm package manager                      |
| **dev\_env/golang**          | Go development environment (conditional)  |
| **dev\_env/node**            | Node.js/NVM environment (conditional)     |
| **mec\_sandbox/mec\_config** | Configure MEC Sandbox                     |
| **mec\_sandbox/mec\_deploy** | Build & deploy MEC Sandbox                |

---

## Key Variables

Variables are defined in `inventories/dev/group_vars/all.yml`.

| Variable              | Default      | Description                        |
|-----------------------|--------------|------------------------------------|
| `kubernetes_version`  | `v1.35.1`    | Kubernetes version                 |
| `calico_version`      | `v3.31.4`    | Calico CNI version                 |
| `install_dev_env`     | `true`       | Enable Go & Node.js setup          |
| `install_mec_sandbox` | `true`       | Enable MEC Sandbox deployment      |

---

## Documentation

| Document                     | Description                                          |
| ---------------------------- | ---------------------------------------------------- |
| **[RUNBOOK.md](RUNBOOK.md)** | Step-by-step deployment guide, troubleshooting, multi-node setup, and detailed configuration |

---

## Notes

* Run `setup_ansible_env.sh` first before executing any playbooks
* Both `etsi-mec-sandbox` and `etsi-mec-sandbox-frontend` repositories must be siblings
* Thanos/Prometheus failures during deployment are expected and ignored

---

