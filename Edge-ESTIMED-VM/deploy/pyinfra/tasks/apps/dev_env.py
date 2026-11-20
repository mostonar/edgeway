from pyinfra import host
from pyinfra.operations import server, files, apt
from lib.operations import dev

target_user = host.data.get('target_user')
target_home = host.data.get('target_home')
go_version = host.data.get('go_version')
go_url = host.data.get('go_url')
node_version = host.data.get('node_version')
npm_version = host.data.get('npm_version')
eslint_version = host.data.get('eslint_version')
mec_sandbox_dir = host.data.get('mec_sandbox_dir')

# ================================
# Golang Setup
# ================================
dev.install_go(
    name="Install Go",
    version=go_version,
    url=go_url,
    _sudo=True
)

files.directory(
    name="Create GOPATH directory",
    path=f"{target_home}/gocode",
    user=target_user,
    group=target_user,
    mode="0755",
    present=True,
    _sudo=True
)


files.directory(
    name="Create GOPATH bin directory",
    path=f"{target_home}/gocode/bin",
    user=target_user,
    group=target_user,
    mode="0755",
    present=True,
    _sudo=True
)

files.directory(
    name="Create GOPATH pkg directory",
    path=f"{target_home}/gocode/pkg",
    user=target_user,
    group=target_user,
    mode="0755",
    present=True,
    _sudo=True
)

files.directory(
    name="Create GOCACHE directory",
    path=f"{target_home}/gocode/cache",
    user=target_user,
    group=target_user,
    mode="0755",
    present=True,
    _sudo=True
)

files.block(
    name="Setup Development environment in .profile",
    path=f"{target_home}/.profile",
    marker="# {mark} PYINFRA MANAGED - Development environment setup",
    content=(
        "export GOPATH=$HOME/gocode\n"
        "export GOCACHE=$GOPATH/cache\n"
        "export GOMODCACHE=$GOPATH/pkg/mod\n"
        "export GOLANGCI_LINT_CACHE=$GOPATH/cache/golangci-lint\n"
        f"export PATH=$PATH:$GOPATH/bin:/usr/local/go/bin:{target_home}/.nvm/versions/node/v{node_version}/bin"
    )
)

dev.install_golangci_lint(
    name="Install GolangCI-Lint",
    version="v2.11.4",
    gocode_bin_dir=f"{target_home}/gocode/bin",
    _env={'PATH': f"/usr/local/go/bin:{target_home}/gocode/bin:/usr/bin:/bin"}
)


# ================================
# Node Setup
# ================================
apt.packages(
    name="Install required system packages for Node",
    packages=["build-essential", "libssl-dev"],
    present=True,
    _sudo=True
)

dev.install_nvm(
    name="Install nvm",
    version="v0.39.7",
    target_home=target_home
)

dev.install_node_and_packages(
    name="Install node, npm, and eslint",
    node_version=node_version,
    npm_version=npm_version,
    eslint_version=eslint_version,
    target_home=target_home,
    _env={'BASH_ENV': f"{target_home}/.bashrc"}
)
