#!/usr/bin/env python3
"""
Updates ingress host, redirect URIs, and provider enabled status in .meepctl-repocfg.yaml.
"""
import argparse
import os
import re
import sys
from ruamel.yaml import YAML


def main():
    parser = argparse.ArgumentParser(description="Update .meepctl-repocfg.yaml")
    parser.add_argument("--path", required=True, help="Path to .meepctl-repocfg.yaml")
    parser.add_argument("--host", required=True, help="MEC host address")
    parser.add_argument("--protocol", default="https", help="MEC protocol (http or https)")
    parser.add_argument("--github-enabled", action="store_true", help="Enable GitHub OAuth")
    parser.add_argument("--gitlab-enabled", action="store_true", help="Enable GitLab OAuth")
    args = parser.parse_args()

    if not os.path.exists(args.path):
        print(f"Skipping {args.path} (file not found)")
        return 0

    yaml = YAML()
    yaml.preserve_quotes = True

    try:
        with open(args.path, "r") as f:
            data = yaml.load(f) or {}
    except Exception as e:
        print(f"Error reading {args.path}: {e}", file=sys.stderr)
        return 1

    changed = False
    deploy = data.get("repo", {}).get("deployment", {})

    perms = deploy.get("permissions", {})
    if perms:
        if perms.get("uid") != os.getuid():
            perms["uid"] = os.getuid()
            changed = True
        if perms.get("gid") != os.getgid():
            perms["gid"] = os.getgid()
            changed = True

    ingress = deploy.get("ingress", {})
    if ingress:
        if ingress.get("host") != args.host:
            ingress["host"] = args.host
            changed = True
        is_ip = bool(re.match(r"^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$", args.host))
        if is_ip and ingress.get("ca") == "lets-encrypt":
            ingress["ca"] = "self-signed"
            changed = True
            
        is_https_only = (args.protocol != "http")
        if ingress.get("https-only") != is_https_only:
            ingress["https-only"] = is_https_only
            changed = True

    auth = deploy.get("auth", {})

    gh = auth.get("github", {})
    if gh:
        if gh.get("enabled") != args.github_enabled:
            gh["enabled"] = args.github_enabled
            changed = True
        new_redirect = f"https://{args.host}/platform-ctrl/v1/authorize"
        if gh.get("redirect-uri") != new_redirect:
            gh["redirect-uri"] = new_redirect
            changed = True

    gl = auth.get("gitlab", {})
    if gl:
        if gl.get("enabled") != args.gitlab_enabled:
            gl["enabled"] = args.gitlab_enabled
            changed = True
        new_redirect = f"https://{args.host}/platform-ctrl/v1/authorize"
        if gl.get("redirect-uri") != new_redirect:
            gl["redirect-uri"] = new_redirect
            changed = True

    if changed:
        with open(args.path, "w") as f:
            yaml.dump(data, f)
        print(f"Updated {args.path}")
    else:
        print(f"No changes needed for {args.path}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
