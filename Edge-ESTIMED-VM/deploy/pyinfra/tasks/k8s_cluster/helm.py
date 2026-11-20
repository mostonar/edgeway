from pyinfra.operations import snap

# Install Helm
snap.package(
    name="Install Helm",
    packages=["helm"],
    channel="latest/stable",
    classic=True,
    present=True,
    _sudo=True,
    _retries=3,
    _retry_delay=5
)
