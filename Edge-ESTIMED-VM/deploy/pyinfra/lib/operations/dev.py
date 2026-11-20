import shlex

from pyinfra import host
from pyinfra.api import operation, StringCommand
from pyinfra.facts.files import File
from pyinfra.operations import files
from pyinfra.facts.server import Command

@operation()
def install_go(version, url):
    """
    install Go with retry resilience.
    """
    if host.get_fact(File, path="/usr/local/go/bin/go"):
        return
        
    yield from files.download._inner(
        src=url,
        dest=f"/tmp/go{version}.linux-amd64.tar.gz"
    )
    yield StringCommand("rm -rf /usr/local/go")
    yield StringCommand(f"tar -C /usr/local -xzf /tmp/go{version}.linux-amd64.tar.gz")
    yield from files.file._inner(
        path=f"/tmp/go{version}.linux-amd64.tar.gz",
        present=False
    )

@operation()
def install_golangci_lint(version, gocode_bin_dir):
    """
    Install golangci-lint at the specified version.
    """
    binary_path = f"{gocode_bin_dir}/golangci-lint"
    current_version_output = host.get_fact(Command, f"{binary_path} --version 2>/dev/null || echo missing")
    
    clean_version = version.lstrip('v')
    if current_version_output and clean_version in current_version_output:
        return
        
    gopath = f"{gocode_bin_dir}/.."
    gocache = f"{gopath}/cache"
    golangci_cache = f"{gopath}/cache/golangci-lint"
    cmd = (
        f"export GOPATH={gopath} GOCACHE={gocache} GOLANGCI_LINT_CACHE={golangci_cache} && "
        f"mkdir -p {gocache} {golangci_cache} && "
        f"/usr/local/go/bin/go env -w GOPATH={gopath} && "
        f"curl --retry 3 --retry-delay 5 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b {gocode_bin_dir} {version}"
    )
    yield StringCommand(cmd)


@operation()
def install_nvm(version, target_home):
    """
    Install NVM with retry resilience.

    Pipe curl to bash (nvm's documented installer) instead of pyinfra
    files.download. files.download writes with `curl -o` into a hashed temp
    file under TmpDir, which is memoized from earlier sudo operations and can
    be unwritable for this unprivileged step (curl error 23).
    """
    nvm_dir = f"{target_home}/.nvm"
    if host.get_fact(File, path=f"{nvm_dir}/nvm.sh"):
        return

    url = f"https://raw.githubusercontent.com/nvm-sh/nvm/{version}/install.sh"
    yield StringCommand(
        f"export HOME={shlex.quote(target_home)} NVM_DIR={shlex.quote(nvm_dir)} && "
        f"curl --retry 3 --retry-delay 5 -fsSL {shlex.quote(url)} | bash"
    )

@operation()
def install_node_and_packages(node_version, npm_version, eslint_version, target_home):
    """
    Install node, npm, and eslint via NVM idempotently.
    Checks both Node.js and global NPM packages so NPM packages are installed even if Node.js was already present.
    """
    node_bin = f"{target_home}/.nvm/versions/node/v{node_version}/bin/node"
    eslint_bin = f"{target_home}/.nvm/versions/node/v{node_version}/bin/eslint"
    
    # Verify both the Node engine and global NPM packages independently.
    # Why: If a target host previously installed Node.js without global dev tools, checking only
    # node_bin would skip package installation and cause subsequent lint/build steps to fail.
    node_present = host.get_fact(File, path=node_bin)
    eslint_present = host.get_fact(File, path=eslint_bin)
    
    if node_present and eslint_present:
        return
        
    # NVM relies on bash shell functions (source/.nvm.sh). PyInfra's local executor defaults
    # to POSIX `sh`, which does not support sourcing script functions; we must wrap in bash -c.
    if not node_present:
        script = (
            f'export NVM_DIR="{target_home}/.nvm" && '
            f'. "$NVM_DIR/nvm.sh" && '
            f'nvm install {node_version} && '
            f'nvm use {node_version} && '
            f'npm install -g npm@{npm_version} && '
            f'npm install -g eslint@{eslint_version} && '
            f'npm install -g eslint-plugin-react'
        )
    else:
        # Node is present, but global npm packages are missing
        script = (
            f'export NVM_DIR="{target_home}/.nvm" && '
            f'. "$NVM_DIR/nvm.sh" && '
            f'nvm use {node_version} && '
            f'npm install -g npm@{npm_version} && '
            f'npm install -g eslint@{eslint_version} && '
            f'npm install -g eslint-plugin-react'
        )
    yield StringCommand(f"bash -c '{script}'")


