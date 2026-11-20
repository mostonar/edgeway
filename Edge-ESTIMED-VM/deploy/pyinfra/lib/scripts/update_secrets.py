#!/usr/bin/env python3
"""
Updates OAuth credentials in a meep secrets.yaml file.
"""
import argparse
import os
import sys
from ruamel.yaml import YAML


def main():
    parser = argparse.ArgumentParser(description="Update OAuth credentials in secrets.yaml")
    parser.add_argument("--path", required=True, help="Path to secrets.yaml")
    parser.add_argument("--github-client-id", default="", help="GitHub Client ID")
    parser.add_argument("--github-client-secret", default="", help="GitHub Client Secret")
    parser.add_argument("--gitlab-client-id", default="", help="GitLab Client ID")
    parser.add_argument("--gitlab-client-secret", default="", help="GitLab Client Secret")
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

    if args.github_client_id and args.github_client_secret:
        gh = data.setdefault("meep-oauth-github", {})
        if gh.get("client-id") != args.github_client_id or gh.get("secret") != args.github_client_secret:
            gh["client-id"] = args.github_client_id
            gh["secret"] = args.github_client_secret
            changed = True

    if args.gitlab_client_id and args.gitlab_client_secret:
        gl = data.setdefault("meep-oauth-gitlab", {})
        if gl.get("client-id") != args.gitlab_client_id or gl.get("secret") != args.gitlab_client_secret:
            gl["client-id"] = args.gitlab_client_id
            gl["secret"] = args.gitlab_client_secret
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
