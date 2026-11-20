"""
Configuration helpers for PyInfra deployment of ETSI MEC Sandbox.
Organized into clean functional sections by comment banners:
1. Environment & Secret Helpers
2. Kubernetes Host, Inventory & Sudo Helpers
3. MEC Sandbox & OAuth Configuration Helpers
"""
import os
import sys
import getpass
from lib.scripts.verify_oauth import verify_github_oauth, verify_gitlab_oauth

# ======================================================================
# 1. ENVIRONMENT & SECRET HELPERS
# ======================================================================

# Automatically load .env variables from pyinfra root before reading any configuration
_env_path = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), ".env")
try:
    from dotenv import load_dotenv
    load_dotenv(_env_path)
except ImportError:
    pass


def is_valid_secret(val, placeholders=None):
    """Checks if a secret or config variable is present and not a default placeholder."""
    if not val:
        return False
    val_clean = str(val).strip()
    if not val_clean:
        return False
    if placeholders and val_clean in placeholders:
        return False
    return True


def is_interactive():
    """Returns True if running in an interactive terminal without CI flags."""
    # Check if standard input is attached to a TTY (terminal)
    if not sys.stdin.isatty():
        return False
    # Explicit CI and automation environment variable overrides prevent interactive prompts
    # from hanging unattended automation pipelines.
    ci_flags = ["CI", "PYINFRA_NONINTERACTIVE"]
    for flag in ci_flags:
        if os.environ.get(flag):
            return False
    if os.environ.get("DEBIAN_FRONTEND") == "noninteractive":
        return False
    return True


# ======================================================================
# 2. KUBERNETES HOST, INVENTORY & SUDO HELPERS
# ======================================================================


def _parse_host_list(raw_str):
    """
    Parses a comma-separated host string into PyInfra host tuples: (address, ssh_user).
    For localhost/127.0.0.1, returns ("@local", local_user).
    For non-localhost targets, enforces mandatory "<username>@<ip>" format.
    """
    hosts = []
    local_user = os.environ.get("SUDO_USER") or os.environ.get("USER", getpass.getuser())
    for h in str(raw_str or "").split(","):
        h_clean = h.strip()
        if not h_clean:
            continue
        if h_clean in ("localhost", "127.0.0.1"):
            hosts.append(("@local", local_user))
        else:
            if "@" not in h_clean:
                raise ValueError(
                    f"Remote host '{h_clean}' must be specified in '<username>@<ip>' format "
                    f"(e.g., 'ubuntu@{h_clean}')."
                )
            user, addr = h_clean.split("@", 1)
            user_clean = user.strip()
            addr_clean = addr.strip()
            if not user_clean or not addr_clean:
                raise ValueError(
                    f"Remote host '{h_clean}' has an empty username or address. "
                    f"Must be specified in '<username>@<ip>' format."
                )
            hosts.append((addr_clean, user_clean))
    return hosts

def get_k8s_inventory():
    """
    Resolves, validates, and builds the declarative PyInfra inventory tuples
    for k8s_masters and k8s_workers.

    - Validates that K8S_MASTERS contains exactly 1 master node and is not co-located with workers on @local.
    - For remote targets (<user>@<host>), automatically re-uses the single provided password
      for both ssh_password and sudo_password so users are only prompted once.
    - For localhost (@local), no SSH password is needed.
    - If SSH keys or passwordless sudo are configured, pressing Enter defaults passwords to None.
    """
    masters = get_k8s_masters()
    workers = get_k8s_workers()

    # 1. Validation
    master_addrs = [addr for addr, _ in masters]
    if len(masters) != 1:
        raise ValueError(
            f"K8S_MASTERS must contain exactly 1 master node (found {len(masters)}). "
            "The ETSI MEC Sandbox architecture requires a single control-plane master node."
        )

    worker_addrs = [addr for addr, _ in workers]
    if "@local" in master_addrs and "@local" in worker_addrs:
        raise ValueError(
            "K8S_MASTERS and K8S_WORKERS cannot both be set to localhost (@local). "
            "A single machine cannot act as both an independent master and worker node."
        )

    def _verify_and_prompt_pw(nodes, pw, label):
        if pw:
            return pw
        import subprocess
        for host_addr, ssh_user in nodes:
            if host_addr == "@local":
                cmd = ["sudo", "-n", "-k", "true"]
                err_msg = "[ERROR] Localhost (@local) requires a password for sudo, but no password was entered."
            else:
                cmd = ["ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", f"{ssh_user}@{host_addr}", "sudo", "-n", "-k", "true"]
                err_msg = f"[ERROR] Remote host {host_addr} ({ssh_user}) requires a password for SSH or sudo, but no password was entered."

            res = subprocess.run(cmd, stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if res.returncode != 0:
                print(f"\033[91m{err_msg}\033[0m")
                prompted = getpass.getpass(f"Enter SSH/sudo password for {label} ({host_addr}): ").strip()
                if prompted:
                    return prompted
        return None

    # 2. Credential acquisition (single unified prompt)
    master_pw = None
    worker_pw = None
    if is_interactive():
        prompted_m = getpass.getpass("Enter password for K8S_MASTERS node(s) (for SSH/sudo, or press Enter if using SSH keys/passwordless sudo): ").strip()
        master_pw = _verify_and_prompt_pw(masters, prompted_m if prompted_m else None, "K8S_MASTERS")
        if workers:
            prompted_w = getpass.getpass("Enter password for K8S_WORKERS node(s) (for SSH/sudo, or press Enter if using SSH keys/passwordless sudo): ").strip()
            worker_pw = _verify_and_prompt_pw(workers, prompted_w if prompted_w else None, "K8S_WORKERS")

    # 3. Build PyInfra host tuples
    def _build_host_tuple(host_addr, ssh_user, pw):
        data = {"ssh_user": ssh_user, "sudo_password": pw, "_sudo_password": pw}
        if host_addr != "@local" and pw:
            data["ssh_password"] = pw
        return (host_addr, data)

    k8s_masters = [_build_host_tuple(addr, user, master_pw) for addr, user in masters]
    k8s_workers = [_build_host_tuple(addr, user, worker_pw) for addr, user in workers]
    return k8s_masters, k8s_workers


def get_target_user_and_home():
    """
    Determines the target SSH/deployment user and their home directory (~ folder).
    - For localhost (@local), uses the currently logged-in user (SUDO_USER/USER).
    - For remote targets (<user>@<host>), uses the username specified in K8S_MASTERS.
    - Enforces that the deployment must be under a normal user's home directory.
    - Raises an error if executed directly as the root user.
    """
    masters = get_k8s_masters()
    if masters and masters[0][0] != "@local":
        target_user = masters[0][1]
    else:
        target_user = os.environ.get("SUDO_USER") or os.environ.get("USER", getpass.getuser())
        
    if target_user == "root":
        raise ValueError(
            "Deployment as the 'root' user is not supported. "
            "Please run the script as a normal user without sudo (e.g., 'pyinfra inventory.py deploy.py -y'). "
            "Sudo passwords will be prompted during execution. "
            "This ensures the sandbox is properly installed in a /home/<user> directory."
        )
        
    target_home = f"/home/{target_user}"
    return target_user, target_home

def get_k8s_masters():
    """
    Retrieves K8S_MASTERS from environment or .env.
    Mandatory: if unset/empty in interactive mode, prompts the user.
    """
    raw = os.environ.get("K8S_MASTERS")
    if not raw or not str(raw).strip():
        if is_interactive():
            raw = input("Enter IP or hostname for K8S_MASTERS [default: localhost]: ").strip() or "localhost"
        else:
            raise ValueError("K8S_MASTERS is mandatory and must be set in environment or .env file.")
    return _parse_host_list(raw)


def get_k8s_workers():
    """Retrieves K8S_WORKERS from environment or .env."""
    raw = os.environ.get("K8S_WORKERS", "")
    return _parse_host_list(raw)


# ======================================================================
# 3. MEC SANDBOX & OAUTH CONFIGURATION HELPERS
# ======================================================================


def get_mec_host_address():
    """
    Retrieves and validates the MEC Sandbox host address from environment or interactive prompt.
    Fails fast with ValueError if missing in non-interactive mode.
    """
    mec_host_address = os.environ.get("MEC_HOST_ADDRESS")
    if not is_valid_secret(mec_host_address, ["", "your-mec-host", "localhost"]):
        if is_interactive():
            mec_host_address = input("Enter the IP or domain for MEC Sandbox (e.g. 192.168.1.100): ").strip()
        else:
            raise ValueError("MEC_HOST_ADDRESS is required and must be set in environment or .env file.")

    if not mec_host_address:
        raise ValueError("MEC_HOST_ADDRESS cannot be empty.")
        
    return mec_host_address


def get_oauth_config():
    """
    Retrieves and validates GitHub and/or GitLab OAuth credentials.
    Ensures at least one OAuth provider is configured according to PyInfra best practices.
    """
    gh_id_raw = os.environ.get("GITHUB_CLIENT_ID", "")
    gh_sec_raw = os.environ.get("GITHUB_CLIENT_SECRET", "")
    gl_id_raw = os.environ.get("GITLAB_CLIENT_ID", "")
    gl_sec_raw = os.environ.get("GITLAB_CLIENT_SECRET", "")

    gh_placeholders = ["", "your-github-client-id", "your-github-client-secret", "my-github-client-id", "my-github-secret"]
    gl_placeholders = ["", "your-gitlab-client-id", "your-gitlab-client-secret", "my-gitlab-client-id", "my-gitlab-secret"]

    github_enabled = is_valid_secret(gh_id_raw, gh_placeholders) and is_valid_secret(gh_sec_raw, gh_placeholders)
    gitlab_enabled = is_valid_secret(gl_id_raw, gl_placeholders) and is_valid_secret(gl_sec_raw, gl_placeholders)

    # Interactive prompting if NEITHER is configured and stdin is a TTY
    if not github_enabled and not gitlab_enabled:
        if is_interactive():
            print("\n[MEC Sandbox Configuration] No valid OAuth credentials found in .env or environment.")
            print("You can configure GitHub, GitLab, or BOTH for OAuth authentication.")
            choice = input("Which OAuth provider would you like to configure? [github/gitlab/both]: ").strip().lower()

            if choice in ("github", "both", "gh"):
                gh_id_raw = input("Enter GitHub OAuth Client ID: ").strip()
                gh_sec_raw = getpass.getpass("Enter GitHub OAuth Client Secret: ").strip()
                github_enabled = is_valid_secret(gh_id_raw, gh_placeholders) and is_valid_secret(gh_sec_raw, gh_placeholders)

            if choice in ("gitlab", "both", "gl"):
                gl_id_raw = input("Enter GitLab OAuth Client ID: ").strip()
                gl_sec_raw = getpass.getpass("Enter GitLab OAuth Client Secret: ").strip()
                gitlab_enabled = is_valid_secret(gl_id_raw, gl_placeholders) and is_valid_secret(gl_sec_raw, gl_placeholders)

    # Validate that at least ONE OAuth provider is properly configured
    if not github_enabled and not gitlab_enabled:
        raise ValueError(
            "MEC Sandbox deployment requires at least one OAuth provider (GitHub OR GitLab OR both) "
            "to be configured with valid Client ID and Secret in .env or environment variables."
        )

    # Validate partial configurations (e.g. ID provided without secret)
    if is_valid_secret(gh_id_raw, gh_placeholders) and not is_valid_secret(gh_sec_raw, gh_placeholders):
        raise ValueError("GITHUB_CLIENT_ID is set but GITHUB_CLIENT_SECRET is missing or invalid.")
    if is_valid_secret(gl_id_raw, gl_placeholders) and not is_valid_secret(gl_sec_raw, gl_placeholders):
        raise ValueError("GITLAB_CLIENT_ID is set but GITLAB_CLIENT_SECRET is missing or invalid.")

    configured_providers = []
    if github_enabled:
        valid, msg = verify_github_oauth(gh_id_raw, gh_sec_raw)
        if not valid:
            raise ValueError(f"GitHub OAuth credentials failed live API check: {msg}")
        print(f"[OAuth Check] {msg}")
        configured_providers.append("github")
    if gitlab_enabled:
        valid, msg = verify_gitlab_oauth(gl_id_raw, gl_sec_raw)
        if not valid:
            raise ValueError(f"GitLab OAuth credentials failed live API check: {msg}")
        print(f"[OAuth Check] {msg}")
        configured_providers.append("gitlab")

    return {
        "github_enabled": github_enabled,
        "gitlab_enabled": gitlab_enabled,
        "github_client_id": gh_id_raw if github_enabled else "",
        "github_client_secret": gh_sec_raw if github_enabled else "",
        "gitlab_client_id": gl_id_raw if gitlab_enabled else "",
        "gitlab_client_secret": gl_sec_raw if gitlab_enabled else "",
        "configured_providers": configured_providers,
    }


__all__ = [
    "is_valid_secret",
    "is_interactive",
    "_parse_host_list",
    "get_k8s_masters",
    "get_k8s_workers",
    "validate_k8s_hosts",
    "get_master_sudo_password",
    "get_worker_sudo_password",
    "get_sudo_password",
    "get_target_user_and_home",
    "get_mec_host_address",
    "get_oauth_config",
]
