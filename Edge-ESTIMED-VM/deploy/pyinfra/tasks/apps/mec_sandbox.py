from pyinfra import host
from pyinfra.operations import server, files
from lib.operations import meep

mec_sandbox_dir = host.data.get('mec_sandbox_dir')
mec_frontend_dir = host.data.get('mec_frontend_dir')
mec_host_address = host.data.get('mec_host_address', '')
github_enabled = host.data.get('github_enabled', False)
gitlab_enabled = host.data.get('gitlab_enabled', False)
github_client_id = host.data.get('github_client_id', '')
github_client_secret = host.data.get('github_client_secret', '')
gitlab_client_id = host.data.get('gitlab_client_id', '')
gitlab_client_secret = host.data.get('gitlab_client_secret', '')
target_user = host.data.get('target_user')
target_home = host.data.get('target_home')
node_version = host.data.get('node_version', '24.18.0')

# Environment configurations for meepctl are now handled directly within lib.operations.meep

if mec_host_address:
    meep.validate_mec_host_address(
        name="Pre-flight Check: Verify MEC_HOST_ADDRESS",
        ip_address=mec_host_address
    )

# Add kubectl bash completion
files.line(
    name="Add kubectl bash completion to .bashrc",
    path=f"{target_home}/.bashrc",
    line="source <(kubectl completion bash)"
)

# Add docker registry entry to /etc/hosts (replacing any existing entry)
files.line(
    name="Add docker registry entry to /etc/hosts",
    path="/etc/hosts",
    line=r".*\smeep-docker-registry.*",
    replace=f"{mec_host_address} meep-docker-registry",
    _sudo=True
)

# # Verify directories exist
files.directory(
    name="Verify etsi-mec-sandbox directory exists",
    path=mec_sandbox_dir,
    present=True
)

files.directory(
    name="Verify etsi-mec-sandbox-frontend directory exists",
    path=mec_frontend_dir,
    present=True
)

# Update OAuth secrets in both frontend and backend config directories
meep.update_oauth_secrets(
    name="Update GitHub/GitLab OAuth credentials in frontend secrets.yaml",
    secrets_path=f"{mec_frontend_dir}/config/secrets.yaml",
    github_client_id=github_client_id,
    github_client_secret=github_client_secret,
    gitlab_client_id=gitlab_client_id,
    gitlab_client_secret=gitlab_client_secret,
)

meep.update_oauth_secrets(
    name="Update GitHub/GitLab OAuth credentials in backend secrets.yaml",
    secrets_path=f"{mec_sandbox_dir}/config/secrets.yaml",
    github_client_id=github_client_id,
    github_client_secret=github_client_secret,
    gitlab_client_id=gitlab_client_id,
    gitlab_client_secret=gitlab_client_secret,
)

# Update ingress host, redirect URIs, and provider enabled status in .meepctl-repocfg.yaml
meep.update_meepctl_repocfg(
    name="Update host and OAuth configuration in frontend .meepctl-repocfg.yaml",
    repocfg_path=f"{mec_frontend_dir}/config/.meepctl-repocfg.yaml",
    host_address=mec_host_address,
    github_enabled=github_enabled,
    gitlab_enabled=gitlab_enabled,
)

meep.update_meepctl_repocfg(
    name="Update host and OAuth configuration in backend .meepctl-repocfg.yaml",
    repocfg_path=f"{mec_sandbox_dir}/.meepctl-repocfg.yaml",
    host_address=mec_host_address,
    github_enabled=github_enabled,
    gitlab_enabled=gitlab_enabled,
)

# MEC Deploy Logic
meep.install(
    name="Install meepctl",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version
)

# meep.configure_sudoers(
#     name="Configure sudoers NOPASSWD rules for meepctl certificate operations",
#     target_user=target_user,
#     _sudo=True,
# )

meep.configure(
    name="Configure meepctl ip and gitdir",
    ip=mec_host_address,
    gitdir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version
)

meep.deploy_frontend(
    name="Build and deploy frontend",
    mec_frontend_dir=mec_frontend_dir,
    target_home=target_home,
    node_version=node_version
)

meep.configure_sandbox_secrets(
    name="Configure MEC Sandbox secrets (secrets.yaml)",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version,
)

meep.deploy_dep(
    name="Deploy MEC Sandbox dependencies (meepctl deploy dep all)",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version,
    force=False,
    _sudo=True,
    _sudo_user=target_user,
)

meep.build_all(
    name="Build all MEC Sandbox binaries (meepctl build --nolint all)",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version,
    nolint=True,
    build_cache=False,
)

meep.dockerize_all(
    name="Dockerize all MEC Sandbox container images (meepctl dockerize all)",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version,
    _sudo=True,
    _sudo_user=target_user,
)

meep.deploy_core(
    name="Deploy MEC Sandbox core platform (meepctl deploy core all)",
    mec_sandbox_dir=mec_sandbox_dir,
    target_home=target_home,
    node_version=node_version,
    force=False,
)

meep.import_scenarios(
    name="Import pre-loaded network scenarios to meep-platform-ctrl API",
    mec_frontend_dir=mec_frontend_dir,
    target_home=target_home,
    node_version=node_version
)

