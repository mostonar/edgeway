"""User-managed CNT subscriptions and non-destructive display projections."""
import hashlib
import math
import re
import threading
from urllib.parse import urlsplit

from core import CSEError, cse_request, strict_json, resource_path

CHANNELS = {"status", "telemetry", "log", "custom"}
TOKEN = re.compile(r"([A-Za-z_][A-Za-z0-9_-]*)|\[(-?\d+)\]")


def path_tokens(path):
    if not isinstance(path, str) or not 1 <= len(path) <= 200:
        raise ValueError("Enter a field path, e.g. pose.x[-1]")
    result, position = [], 0
    for match in TOKEN.finditer(path):
        gap = path[position:match.start()]
        expected = "." if result and match.group(1) else ""
        if gap != expected:
            raise ValueError("Field path format: pose.x or pose.x[-1].")
        result.append(match.group(1) if match.group(1) else int(match.group(2)))
        position = match.end()
    if position != len(path) or not result or len(result) > 20:
        raise ValueError("Invalid field path.")
    return result


def validate_fields(fields):
    if not isinstance(fields, list) or len(fields) > 16:
        raise ValueError("Maximum 16 display fields.")
    cleaned = []
    for field in fields:
        if not isinstance(field, dict):
            raise ValueError("Invalid field settings.")
        path_tokens(field.get("path"))
        label, unit, scale = field.get("label"), field.get("unit", ""), field.get("scale", 1)
        if not isinstance(label, str) or not 1 <= len(label.strip()) <= 60:
            raise ValueError("Display name must contain 1 to 60 characters.")
        if not isinstance(unit, str) or len(unit) > 20:
            raise ValueError("Unit must contain no more than 20 characters.")
        if type(scale) not in (float, int) or not math.isfinite(scale) or abs(scale) > 1e12:
            raise ValueError("Scale must be finite, with absolute value no greater than one trillion.")
        cleaned.append({"path": field["path"], "label": label.strip(), "unit": unit, "scale": scale})
    return cleaned


def decode_content(value):
    if isinstance(value, str):
        try:
            return strict_json(value)
        except (ValueError, RecursionError):
            pass
    return value


def project(content, fields):
    content = decode_content(content)
    output = []
    for field in validate_fields(fields):
        value, issue = content, ""
        try:
            for key in path_tokens(field["path"]):
                if isinstance(key, int):
                    if not isinstance(value, list):
                        raise KeyError(key)
                elif not isinstance(value, dict):
                    raise KeyError(key)
                value = value[key]
            if type(value) in (float, int):
                value *= field["scale"]
                if not math.isfinite(value):
                    raise ValueError("Converted value exceeds the numeric range.")
            elif field["scale"] != 1:
                raise ValueError("Scale can only be applied to numeric fields.")
            elif isinstance(value, (dict, list)):
                raise ValueError("Select a field within the object or array.")
        except (KeyError, IndexError, TypeError):
            value, issue = None, "This field is missing from the received data."
        except (ValueError, OverflowError) as exc:
            value, issue = None, str(exc)
        output.append({**field, "value": value, "issue": issue})
    return output


def suggestions(content):
    output = []
    def walk(value, path="", depth=0):
        if len(output) >= 100 or depth > 12:
            return
        if isinstance(value, dict):
            for key, child in value.items():
                if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_-]*", key):
                    walk(child, path + ("." if path else "") + key, depth + 1)
        elif isinstance(value, list):
            if value:
                walk(value[-1], path + "[-1]", depth + 1)
        elif path:
            output.append(path)
    walk(decode_content(content))
    return output


def camera_config(data):
    if not isinstance(data, dict):
        raise ValueError("Invalid camera settings.")
    name, url = data.get("name", "Site Camera"), data.get("url", "")
    if not isinstance(name, str) or not 1 <= len(name.strip()) <= 60:
        raise ValueError("Camera name must contain 1 to 60 characters.")
    if not isinstance(url, str) or len(url) > 2048:
        raise ValueError("Invalid web viewer URL.")
    url = url.strip()
    if url:
        parsed = urlsplit(url)
        if (parsed.scheme not in ("http", "https") or not parsed.hostname or parsed.username or parsed.password
                or any(c.isspace() or ord(c) < 32 for c in url)
                or not re.fullmatch(r"[A-Za-z0-9_.:\[\]-]+", parsed.netloc)):
            raise ValueError("Enter a browser-accessible HTTP(S) web viewer URL. Do not include credentials in the URL.")
        try:
            parsed.port
        except ValueError:
            raise ValueError("Invalid camera port.")
    return {"name": name.strip(), "url": url, "mode": "web_viewer"}


class BindingService:
    def __init__(self, cfg, store):
        self.cfg, self.store = cfg, store
        self.lock = threading.RLock()
        store.import_bindings(cfg)

    def get(self, key):
        return self.store.profile(key, self.cfg["cse_url"])

    @staticmethod
    def public(profile):
        return {k: v for k, v in profile.items() if k != "origin"}

    def rows(self):
        self.store.import_bindings(self.cfg)
        result = []
        for profile in self.store.profiles(self.cfg["cse_url"]):
            sample = self.store.latest(profile)
            result.append({**self.public(profile), "last_received_at": sample["received_at"] if sample else None})
        return result

    def sample(self, key):
        profile = self.get(key)
        latest = self.store.latest(profile)
        content = latest["content"] if latest else None
        return {"received": latest is not None, "content": content,
                "cin_ri": latest["cin_ri"] if latest else None,
                "received_at": latest["received_at"] if latest else None,
                "paths": suggestions(content), "values": project(content, profile["fields"])}

    def values(self):
        result = []
        for profile in self.store.profiles(self.cfg["cse_url"]):
            sample = self.sample(profile["id"])
            result.append({"binding": self.public(profile), **{k: sample[k] for k in ("received", "cin_ri", "received_at", "values")}})
        return result

    def optional(self, path, origin=None):
        try:
            return cse_request(self.cfg, "GET", path, origin=origin)
        except CSEError as exc:
            if exc.status == 404 and exc.rsc == "4004":
                return None
            raise

    def register(self, data):
        with self.lock:
            if not isinstance(data, dict):
                raise ValueError("Invalid subscription settings.")
            robot, name, path, channel = (data.get(k) for k in ("robot_id", "name", "resource_path", "channel"))
            if not isinstance(robot, str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,80}", robot):
                raise ValueError("Robot ID must contain only letters, digits, underscores or hyphens.")
            if not isinstance(name, str) or not 1 <= len(name.strip()) <= 80:
                raise ValueError("Robot display name must contain 1 to 80 characters.")
            if not isinstance(path, str):
                raise ValueError("Enter a CNT path.")
            path = path.strip().strip("/")
            if not re.fullmatch(r"[A-Za-z0-9_-]{1,80}(?:/[A-Za-z0-9_-]{1,80}){1,8}", path):
                raise ValueError("Enter a CNT path below the CSE, e.g. forklift_b/status.")
            if not isinstance(channel, str) or channel not in CHANNELS:
                raise ValueError("Select a data type.")
            profiles = self.store.profiles(self.cfg["cse_url"])
            previous = next((p for p in profiles if p["resource_path"] == path), None)
            if previous and (previous["robot_id"] != robot or previous["channel"] != channel):
                raise ValueError("This CNT is already linked to another robot or data type.")
            if any(p["robot_id"] == robot and p["channel"] == channel and p["resource_path"] != path for p in profiles):
                raise ValueError("This data type is already linked for this robot. Choose another type or robot ID.")
            cnt, origin = self.ensure_container(path)
            if cnt.get("ty") != 3 or not isinstance(cnt.get("ri"), str):
                raise ValueError("The subscription target path is not a CNT. Existing resources were not changed.")
            key = "b-" + hashlib.sha256((self.cfg["cse_url"] + "/" + path).encode()).hexdigest()[:20]
            profile = previous or {"id": key, "source": self.cfg["cse_url"], "resource_path": path,
                                   "robot_id": robot, "channel": channel, "fields": [],
                                   "subscription_name": "edgeway-" + key[2:], "origin": origin}
            profile.update(name=name.strip(), container_ri=cnt["ri"], state="pending", error="")
            self.store.put_profile(profile)
            subpath = path + "/" + profile["subscription_name"]
            desired = {"enc": {"net": [3]}, "nct": 1, "nu": [self.cfg["notification_url"]]}
            try:
                existing = self.optional(subpath, profile["origin"])
                if existing:
                    sub = existing.get("m2m:sub", {})
                    if any(sub.get(k) != v for k, v in desired.items()):
                        raise ValueError("An existing subscription with the same name has different settings. It was not overwritten.")
                else:
                    sub = cse_request(self.cfg, "POST", path,
                                      {"m2m:sub": {"rn": profile["subscription_name"], **desired}},
                                      23, profile["origin"]).get("m2m:sub", {})
                if sub.get("ty") != 23 or sub.get("pi") != cnt["ri"] or not sub.get("ri"):
                    raise ValueError("CSE returned an invalid subscription response.")
                profile.update(subscription_ri=sub["ri"], state="active", error="")
                saved = self.store.put_profile(profile)
                # Keep the original CLI's mapping valid after a migrated subscription is recreated.
                if profile["subscription_name"] == self.cfg["subscription_name"]:
                    for robot in self.cfg["robots"]:
                        if resource_path(robot) == path:
                            self.store.bind(self.cfg, robot, cnt["ri"], sub["ri"], profile["origin"])
                return self.public(saved)
            except (CSEError, ValueError) as exc:
                profile.update(state="failed", error=str(exc))
                self.store.put_profile(profile)
                raise

    def ensure_container(self, path):
        """Reuse existing CNTs; create missing AE/CNT ancestors without overwrites."""
        existing = self.optional(path)
        if existing is not None:
            if not isinstance(existing, dict) or not isinstance(existing.get("m2m:cnt"), dict):
                raise ValueError("The subscription target path is not a CNT. Existing resources were not changed.")
            return existing["m2m:cnt"], self.cfg["origin"]

        def ensure(resource_path, parent, wrapper, attributes, ty, origin, create_origin=None):
            value = self.optional(resource_path, origin)
            if value is None:
                try:
                    value = cse_request(self.cfg, "POST", parent, {wrapper: attributes}, ty, create_origin or origin)
                except CSEError as exc:
                    # Another sender may have created the same resource meanwhile.
                    if exc.status != 409 or exc.rsc != "4105":
                        raise
                    value = cse_request(self.cfg, "GET", resource_path, origin=origin)
            resource = value.get(wrapper) if isinstance(value, dict) else None
            if (not isinstance(resource, dict) or resource.get("ty") != ty
                    or not isinstance(resource.get("ri"), str) or not resource["ri"]):
                raise ValueError(f"{resource_path}: Resource type or CSE response is invalid. Existing resources retained.")
            return resource

        parts = path.split("/")
        ae = ensure(parts[0], "", "m2m:ae", {"rn": parts[0], "api": "Norg.edgeway.robot",
                    "rr": False, "srv": [self.cfg["release"]]}, 2, self.cfg["origin"], "C")
        if not isinstance(ae.get("aei"), str) or not ae["aei"]:
            raise ValueError("The ACME AE response has no valid AE-ID.")
        origin, parent, parent_ri = ae["aei"], parts[0], ae["ri"]
        for name in parts[1:]:
            target = parent + "/" + name
            cnt = ensure(target, parent, "m2m:cnt", {"rn": name}, 3, origin)
            if cnt.get("pi") != parent_ri:
                raise ValueError(f"{target}: CNT parent does not match.")
            parent, parent_ri = target, cnt["ri"]
        return cnt, origin

    def check(self, key):
        with self.lock:
            profile = self.get(key)
            try:
                sub = self.optional(profile["resource_path"] + "/" + profile["subscription_name"], profile["origin"])
                if not sub:
                    profile.update(state="inactive", error="CSE has no subscription.")
                else:
                    actual = sub.get("m2m:sub", {})
                    if (actual.get("ri") != profile.get("subscription_ri") or actual.get("pi") != profile["container_ri"]
                            or actual.get("nu") != [self.cfg["notification_url"]] or actual.get("enc") != {"net": [3]} or actual.get("nct") != 1):
                        raise ValueError("Remote subscription differs from the saved connection.")
                    profile.update(state="active", error="")
            except (CSEError, ValueError) as exc:
                profile.update(state="unknown", error=str(exc))
            return self.public(self.store.put_profile(profile))

    def unsubscribe(self, key):
        with self.lock:
            profile = self.get(key)
            path = profile["resource_path"] + "/" + profile["subscription_name"]
            actual = self.optional(path, profile["origin"])
            if actual:
                sub = actual.get("m2m:sub", {})
                if (sub.get("ri") != profile.get("subscription_ri") or sub.get("pi") != profile["container_ri"]
                        or sub.get("nu") != [self.cfg["notification_url"]]):
                    raise ValueError("Remote resource differs from the registered subscription. It was not removed. Check connection details.")
                cse_request(self.cfg, "DELETE", path, origin=profile["origin"])
            profile.update(state="inactive", error="")
            return self.public(self.store.put_profile(profile))

    def fields(self, key, fields):
        with self.lock:
            profile = self.get(key)
            profile["fields"] = validate_fields(fields)
            self.store.put_profile(profile)
            return self.sample(key)
