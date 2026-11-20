import os
import shlex
from pyinfra import host
from pyinfra.api import operation, StringCommand
from pyinfra.facts.files import File
from pyinfra.operations import files

_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
_SECRETS_SCRIPT = os.path.normpath(os.path.join(_SCRIPT_DIR, "../scripts/update_secrets.py"))
_REPOCFG_SCRIPT = os.path.normpath(os.path.join(_SCRIPT_DIR, "../scripts/update_repocfg.py"))
_IMPORT_SCENARIOS_SCRIPT = os.path.normpath(os.path.join(_SCRIPT_DIR, "../scripts/import_scenarios.py"))

def _build_env_prefix(target_home, node_version):
    """Creates a bash command prefix that sets all required environment variables for meepctl to function."""
    sudo_pass = host.data.get("sudo_password") or ""
    gopath = f"{target_home}/gocode"
    gocache = f"{gopath}/cache"
    gomodcache = f"{gopath}/pkg/mod"
    golangci_cache = f"{gopath}/cache/golangci-lint"
    node_bin = f"{target_home}/.nvm/versions/node/v{node_version}/bin"
    path = f"/usr/local/go/bin:{gopath}/bin:{node_bin}:/snap/bin:/usr/local/bin:/usr/bin:/bin"
    # Tool caches live under GOPATH so `go` / golangci-lint never touch a
    # root-owned ~/.cache left behind by earlier sudo builds.
    return (
        f"export PATH={shlex.quote(path)} "
        f"GOPATH={shlex.quote(gopath)} "
        f"GOCACHE={shlex.quote(gocache)} "
        f"GOMODCACHE={shlex.quote(gomodcache)} "
        f"GOLANGCI_LINT_CACHE={shlex.quote(golangci_cache)} "
        f"HOME={shlex.quote(target_home)} "
        f"KUBECONFIG={shlex.quote(target_home + '/.kube/config')} "
        f"SUDO_PASSWORD={shlex.quote(sudo_pass)} && "
        f"mkdir -p {shlex.quote(gocache)} {shlex.quote(gomodcache)} {shlex.quote(golangci_cache)} &&"
    )

@operation()
def validate_mec_host_address(ip_address):
    """
    Validates over SSH that the provided IP address actually exists on the target machine.
    """
    from pyinfra.operations import server, files
    script_path = os.path.normpath(os.path.join(_SCRIPT_DIR, "../scripts/validate_ip.py"))
    
    yield from files.put._inner(
        src=script_path,
        dest="/tmp/validate_ip.py",
        mode="0755"
    )
    yield from server.shell._inner(
        commands=[f"python3 /tmp/validate_ip.py {ip_address}"]
    )

@operation()
def configure_sudoers(target_user):
    """
    Create /etc/sudoers.d/meepctl with NOPASSWD rules for meepctl
    certificate trust operations (sudo cp, update-ca-certificates, systemctl restart).

    This operation must be called with _sudo=True so pyinfra handles authentication
    natively, avoiding the nested-sudo problem when install.sh tries sudo -S inside
    a pyinfra-managed sudo context.
    """
    if host.get_fact(File, path="/etc/sudoers.d/meepctl"):
        return

    sudoers_rule = (
        f"{target_user} ALL=(ALL:ALL) NOPASSWD: "
        "/usr/bin/cp, /bin/cp, "
        "/usr/sbin/update-ca-certificates, /usr/bin/update-ca-certificates, "
        "/usr/bin/systemctl restart containerd, /usr/bin/systemctl restart docker, "
        "/bin/systemctl restart containerd, /bin/systemctl restart docker"
    )
    escaped_rule = shlex.quote(sudoers_rule)
    yield StringCommand(f"echo {escaped_rule} > /etc/sudoers.d/meepctl && chmod 0440 /etc/sudoers.d/meepctl")

@operation()
def install(mec_sandbox_dir, target_home, node_version):
    """
    Install meepctl.
    """
    # if host.get_fact(File, path=f"{target_home}/gocode/bin/meepctl"):
    #     return
        
    prefix = _build_env_prefix(target_home, node_version)
    yield StringCommand(f"{prefix} cd {mec_sandbox_dir}/go-apps/meepctl && bash install.sh")

@operation()
def configure(ip, gitdir, target_home, node_version):
    """
    Configure meepctl.
    """
    # if host.get_fact(File, path=f"{target_home}/.meep/.meepctl_configured"):
    #     return
        
    prefix = _build_env_prefix(target_home, node_version)
    yield StringCommand(f"{prefix} meepctl config ip {ip}")
    yield StringCommand(f"{prefix} meepctl config gitdir {gitdir}")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.meepctl_configured")

@operation()
def deploy_frontend(mec_frontend_dir, target_home, node_version):
    """
    Build and deploy the frontend.
    """
    # if host.get_fact(File, path=f"{target_home}/.meep/.frontend_deployed"):
    #     return
        
    prefix = _build_env_prefix(target_home, node_version)
    yield StringCommand(f"{prefix} cd {mec_frontend_dir} && bash build.sh && bash deploy.sh")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.frontend_deployed")

@operation()
def configure_sandbox_secrets(mec_sandbox_dir, target_home, node_version):
    """
    Configure MEC Sandbox secrets (secrets.yaml).
    """
    # if not host.get_fact(File, path=f"{target_home}/.meep/.01_secrets_configured"):
    prefix = _build_env_prefix(target_home, node_version)
    yield StringCommand(f"{prefix} python3 {mec_sandbox_dir}/config/configure-secrets.py set {mec_sandbox_dir}/config/secrets.yaml")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.01_secrets_configured")


@operation()
def deploy_dep(mec_sandbox_dir, target_home, node_version, force=True):
    """
    Deploy MEC Sandbox dependencies (meepctl deploy dep all).
    """
    # if not host.get_fact(File, path=f"{target_home}/.meep/.02_deps_deployed"):
    prefix = _build_env_prefix(target_home, node_version)
    force_flag = "-f " if force else ""
    yield StringCommand(f"{prefix} meepctl deploy dep all {force_flag} > /dev/tty 2>&1 || true")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.02_deps_deployed")


@operation()
def build_all(mec_sandbox_dir, target_home, node_version, nolint=True, build_cache=True):
    """
    Compile all MEC Sandbox binaries (meepctl build all).
    """
    # if not host.get_fact(File, path=f"{target_home}/.meep/.03_binaries_built"):
    prefix = _build_env_prefix(target_home, node_version)
    nolint_flag = "--nolint " if nolint else ""
    build_cache_flag = "--no-cache " if build_cache else ""
    yield StringCommand(f"{prefix} meepctl build {build_cache_flag } {nolint_flag}all > /dev/tty 2>&1")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.03_binaries_built")


@operation()
def dockerize_all(mec_sandbox_dir, target_home, node_version):
    """
    Build and package all MEC Sandbox container images (meepctl dockerize all).
    """
    # if not host.get_fact(File, path=f"{target_home}/.meep/.04_images_dockerized"):
    prefix = _build_env_prefix(target_home, node_version)
    yield StringCommand(f"{prefix} cd {mec_sandbox_dir} && meepctl dockerize all > /dev/tty 2>&1")
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.04_images_dockerized")



@operation()
def deploy_core(mec_sandbox_dir, target_home, node_version, force=True):
    """
    Deploy MEC Sandbox core K8s platform (meepctl deploy core all).
    """
    prefix = _build_env_prefix(target_home, node_version)
    force_flag = "-f" if force else ""
    yield StringCommand(f"{prefix} cd {mec_sandbox_dir} && meepctl deploy core all {force_flag}")


@operation()
def update_oauth_secrets(secrets_path, github_client_id="", github_client_secret="", gitlab_client_id="", gitlab_client_secret=""):
    """
    Update GitHub and/or GitLab OAuth credentials in secrets.yaml using standalone helper script.
    """
    yield from files.put._inner(
        src=_SECRETS_SCRIPT,
        dest="/tmp/meep_update_secrets.py",
        add_deploy_dir=False,
        mode="0755"
    )
    gh_id = shlex.quote(github_client_id or "")
    gh_sec = shlex.quote(github_client_secret or "")
    gl_id = shlex.quote(gitlab_client_id or "")
    gl_sec = shlex.quote(gitlab_client_secret or "")
    path = shlex.quote(secrets_path)
    cmd = (
        f"python3 /tmp/meep_update_secrets.py --path {path} "
        f"--github-client-id {gh_id} --github-client-secret {gh_sec} "
        f"--gitlab-client-id {gl_id} --gitlab-client-secret {gl_sec}"
    )
    yield StringCommand(cmd)


@operation()
def update_meepctl_repocfg(repocfg_path, host_address, github_enabled=True, gitlab_enabled=True):
    """
    Update ingress host, redirect URIs, and provider enabled status in .meepctl-repocfg.yaml using standalone helper script.
    """
    yield from files.put._inner(
        src=_REPOCFG_SCRIPT,
        dest="/tmp/meep_update_repocfg.py",
        add_deploy_dir=False,
        mode="0755"
    )
    
    protocol = os.environ.get("MEC_PROTOCOL", "https").strip().lower()

    path = shlex.quote(repocfg_path)
    host_addr = shlex.quote(host_address)
    prot = shlex.quote(protocol)
    gh_flag = "--github-enabled" if github_enabled else ""
    gl_flag = "--gitlab-enabled" if gitlab_enabled else ""
    cmd = f"python3 /tmp/meep_update_repocfg.py --path {path} --host {host_addr} --protocol {prot} {gh_flag} {gl_flag}".strip()
    yield StringCommand(cmd)


@operation()
def import_scenarios(mec_frontend_dir, target_home, node_version):
    """
    Load, convert, and import pre-loaded network scenario YAMLs into meep-platform-ctrl API.
    """
    # if host.get_fact(File, path=f"{target_home}/.meep/.05_scenarios_imported"):
    #     return

    yield from files.put._inner(
        src=_IMPORT_SCENARIOS_SCRIPT,
        dest="/tmp/meep_import_scenarios.py",
        add_deploy_dir=False,
        mode="0755"
    )

    prefix = _build_env_prefix(target_home, node_version)
    cmd = (
        f"{prefix} python3 /tmp/meep_import_scenarios.py "
        f"--networks-dir {shlex.quote(mec_frontend_dir + '/networks')} "
        f"--kubeconfig {shlex.quote(target_home + '/.kube/config')}"
    )
    yield StringCommand(cmd)
    # yield StringCommand(f"mkdir -p {target_home}/.meep && touch {target_home}/.meep/.05_scenarios_imported")

