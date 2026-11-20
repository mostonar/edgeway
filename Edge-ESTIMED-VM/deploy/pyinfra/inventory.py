"""
Declarative PyInfra Inventory for ETSI MEC Sandbox Deployment.

All host resolution, validation, and credential acquisition logic is modularized
in lib/config_helpers.py:
- For remote targets (<user>@<host>), automatically re-uses the provided sudo password
  as the SSH fallback password so users are only prompted once.
- For localhost (@local), no SSH password is needed.
- If SSH keys or passwordless sudo are configured, pressing Enter defaults passwords to None.
"""
from lib.config_helpers import get_k8s_inventory

k8s_masters, k8s_workers = get_k8s_inventory()
