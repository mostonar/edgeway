"""
fl/onem2m_pch.py — receiving notifications via oneM2M <pollingChannel>

[About the name]
pch is the official abbreviation oneM2M gives to <pollingChannel> (m2m:pch, ty=15).
Calling it by the generic word "polling" reads as "scrape periodically",
which this is not. Subscriptions and notifications stay as they are; only the delivery direction is reversed.
Using the spec term as is avoids confusion.

[Why it is needed]
The cloud IN-CSE cannot call our AE behind the firewall. When creating a subscription
the CSE sends a verification request (vrq) to the notification address, which gets blocked: with direct notification
it ends in a read timeout, and with nu set to the AE-ID it ends in "Verification request failed"
instead.

TS-0001 treats such AEs as "request-unreachable" and
defines <pollingChannel> for them. It is not a workaround; it is a path in the spec.

  Before:  CSE ──POST /notify──▶ AE          dropped by the firewall
  After:   AE ──GET pcu (long poll)──▶ CSE
       CSE ──notification in that response──▶ AE     the AE opened the connection, so it is not blocked

Subscriptions and notification semantics are unchanged. Only the delivery path changes.

[This is not a global setting]
Each path has different conditions, so the delivery mode must differ per path too.

  Robot → MN-CSE → MN-AE     inside the site, reachable     → poa + direct notification
  IN-CSE → MN-AE / IN-AE    across a firewall, unreachable → <pollingChannel>

Setting both with one flag also kills in-site subscriptions that have nothing to do
with the firewall. See config.local_notify_mode() / remote_notify_mode().

[Creator identity — measured]
A <pollingChannel> can only be created by the AE itself. Not even the admin can.

  acme/resources/AE.py:103-106
    if childResource.ty == ResourceTypes.PCH:
        # Check correct originator. Even the ADMIN is not allowed that
        if self.aei != originator:
            raise ORIGINATOR_HAS_NO_PRIVILEGE('Originator must be the parent <AE>')

So aei is read from the AE resource and the channel is created with that identity. Even when
config.USE_AE_IDENTITY is off (= normal requests go as CAdmin), this one request goes out as the AE.
One can also be attached under <remoteCSE> by the same rule (CSR.py:126, compares self.csi).

[Long-poll responses — measured, ACME]
  Queue empty          504 + {"m2m:dbg": "Request Expiration Timestamp reached.
                              No request queued for originator: CIN-AE"}
                       Not an error. Just poll again.
  Notification present 200 + {"m2m:rqp": {"fr","to","op":5,"rqi","pc":{"m2m:sgn":{...}}}}
  Shape inside pc      Identical to the body the existing HTTP /notify received.
                       So the notification handling logic is reused as is.
  No channel           404 + {"m2m:dbg": "resource not found"}
                       If this response repeats, securing the channel failed.
  Queue granularity    per originator (AE-ID).
"""

import json
import os
import threading
import time
from typing import Callable, Optional

import config
import onem2m_utils as om2m


# How long one long poll stays open at the CSE. ACME's default is about 10 s.
# Too short means frequent re-requests; too long makes shutdown sluggish.
PCH_WAIT_SEC = float(os.getenv("FL_PCH_WAIT_SEC", "20"))

# Back-off after consecutive failures. Keeps things from spinning when the CSE briefly wobbles.
PCH_BACKOFF_SEC = float(os.getenv("FL_PCH_BACKOFF_SEC", "3"))

DEFAULT_NAME = os.getenv("FL_PCH_NAME", "pch")

PCH_TY = 15


# ── Setup ────────────────────────────────────────────────

def ae_identity(ae_path: str) -> Optional[str]:
    """
    Read aei (AE-ID) from the AE resource. None if absent.

    There are two reasons to ask the CSE instead of guessing the AE-ID.
    One is that <pollingChannel> creation only passes if it matches this value exactly;
    the other is that the same request tells us whether the AE is registered on that CSE.
    None means "that AE does not exist here".
    """
    res = om2m.get_resource(ae_path)
    if not res:
        return None
    return (res.get("m2m:ae") or {}).get("aei")


def find_channel(ae_path: str, aei: str) -> Optional[str]:
    """
    Find the name of a <pollingChannel> already under the AE. None if absent.

    Why not assume the name: there is only one channel per AE (AE.py:109), so if it already
    exists under a different name, creation is blocked with 400, and scraping the pcu under the
    name we expected only returns 404. The actual name of the existing one must be used.
    """
    url = om2m._url(f"{ae_path}?fu=1&ty={PCH_TY}&lim=5")
    try:
        r = om2m._request_with_retry(
            "get", url, headers=om2m._headers(origin=aei), timeout=10)
    except Exception:
        return None
    if r.status_code != 200:
        return None
    uril = r.json().get("m2m:uril") or []
    if not isinstance(uril, list) or not uril:
        return None
    return str(uril[0]).rstrip("/").split("/")[-1]


def create_channel(ae_path: str, aei: str,
                   name: str = DEFAULT_NAME) -> Optional[str]:
    """Create the <pollingChannel>. Returns its name on success, None on failure."""
    try:
        r = om2m._request_with_retry(
            "post", om2m._url(ae_path),
            json={"m2m:pch": {"rn": name}},
            headers=om2m._headers(PCH_TY, origin=aei),
            timeout=10,
        )
    except Exception as e:
        print(f"  ✗ pollingChannel creation error: {e}")
        return None

    if r.status_code in (201, 409):
        return name
    print(f"  ✗ pollingChannel creation failed: {r.status_code} {r.text[:160]}")
    return None


def set_request_unreachable(ae_path: str, aei: str,
                            unreachable: bool = True) -> bool:
    """
    Change the AE's rr (requestReachability).

    With rr=true the CSE tries to call the AE directly. That fails behind a firewall,
    so it must be false for the CSE to queue notifications on the polling channel.

    Caution: do not call this before the channel is secured. The AE would end up in a state where it
    "cannot be called directly and has nowhere to fetch from", and subscriptions created for it would
    never be verified. setup() enforces the call order.
    """
    try:
        r = om2m._request_with_retry(
            "put", om2m._url(ae_path),
            json={"m2m:ae": {"rr": not unreachable}},
            headers=om2m._headers(body=True, origin=aei),
            timeout=10,
        )
    except Exception as e:
        print(f"  ✗ rr update error: {e}")
        return False

    if r.status_code in (200, 204):
        print(f"  ✓ {om2m.strip_prefix(ae_path)} rr={not unreachable}")
        return True
    print(f"  ✗ rr update failed: {r.status_code} {r.text[:160]}")
    return False


def setup(ae_path: str, notify_url: str,
          label: str = "", name: str = DEFAULT_NAME
          ) -> Optional["PchReceiver"]:
    """
    Prepare <pollingChannel> reception end to end. Returns the running receiver on success, None on failure.

    Order is everything.
      1) Read the AE's aei          — only the AE itself can create it, and this also
                                     confirms the AE exists on this CSE
      2) Secure the channel          — use the existing name if any, otherwise create it
      3) Only then rr=false          — doing 3 after 2 failed leaves the AE dead
      4) Start the receiver thread   — starting it after 2 failed loops on 404 forever

    On failure, nothing on the CSE is changed and None is returned.
    When the caller gets None, it should assume notifications cannot be received on that path.
    """
    tag = f" ({label})" if label else ""
    print(f"\n  oneM2M <pollingChannel> setup{tag}")
    print(f"    target: {om2m.strip_prefix(ae_path)}")

    aei = ae_identity(ae_path)
    if not aei:
        print(f"    ✗ {om2m.strip_prefix(ae_path)} does not exist on this CSE.")
        print(f"      <pollingChannel> is a child of the AE, so it cannot be created without the AE.")
        return None
    print(f"    AE-ID: {aei}")

    chan = find_channel(ae_path, aei)
    if chan:
        print(f"    = already exists: {chan}")
    else:
        chan = create_channel(ae_path, aei, name)
        if not chan:
            print(f"    ✗ Could not obtain the channel. Leaving rr unchanged.")
            return None
        print(f"    ✓ created: {chan}")

    if not set_request_unreachable(ae_path, aei):
        print(f"    ✗ Could not turn rr off. The CSE will keep trying direct calls.")
        return None

    recv = PchReceiver(
        pcu_path=f"{ae_path}/{chan}/pcu",
        origin=aei,
        handler=relay_to_local(notify_url),
    )
    recv.start()
    return recv


def relay_to_local(notify_url: str) -> Callable[[dict], None]:
    """
    Build a handler that forwards received notifications as is to the local HTTP handler.

    The notification logic (subscription verification, net decisions, result collection) already lives inside
    the Flask route. Instead of tearing it apart, the body received via pollingChannel is POSTed to the same route
    on 127.0.0.1. Only the delivery path changes; the handling code stays the same.

    It adds one local round trip, but over loopback that is negligible, and
    leaving verified code untouched is better.
    """
    import requests

    def _handler(pc: dict) -> None:
        try:
            requests.post(notify_url, json=pc, timeout=10)
        except Exception as e:
            print(f"  ⚠ local notification delivery failed ({notify_url}): {e}")

    return _handler


# ── Receive ──────────────────────────────────────────────

class PchReceiver(threading.Thread):
    """
    Thread that long-polls the <pollingChannel> and passes notifications to handler.

    handler receives the same dict the existing HTTP /notify received,
    i.e. {"m2m:sgn": {...}}. So the notification logic needs no changes.

    Do not construct this directly; use setup(). Starting it with no channel
    just prints 404 forever.
    """

    def __init__(self, pcu_path: str, origin: str,
                 handler: Callable[[dict], None]):
        super().__init__(daemon=True, name=f"pch-{origin}")
        self.pcu_path = pcu_path
        self.origin = origin
        self.handler = handler
        self._stop_evt = threading.Event()
        self.received = 0
        self.errors = 0
        self.respond_errors = 0
        # rqi values already handled. Detects whether the same notification comes down again.
        # Handling itself is idempotent, so it is not blocked, only made visible.
        self._seen_rqi: set[str] = set()
        self.duplicates = 0

    def stop(self) -> None:
        self._stop_evt.set()

    # ── Internal ────────────────────────────────────────

    def _headers(self) -> dict:
        return om2m._headers(origin=self.origin)

    def _respond(self, rqp: dict) -> None:
        """
        Send back the response primitive.

        After sending a notification the CSE waits for a response. Without one the request stays pending
        and the next notification may not come down. rqi must be echoed back so the CSE
        knows which request the response is for.
        """
        rqi = rqp.get("rqi")
        if not rqi:
            return
        rsp = {
            "m2m:rsp": {
                "fr": self.origin,
                "to": rqp.get("fr", ""),
                "rqi": rqi,
                "rsc": 2000,                      # 2000 = OK
                "rvi": config.HEADERS.get("X-M2M-RVI", "3"),
            }
        }
        try:
            r = om2m._request_with_retry(
                "post", om2m._url(self.pcu_path),
                json=rsp, headers=om2m._headers(body=True, origin=self.origin),
                timeout=10,
            )
        except Exception as e:
            self.respond_errors += 1
            print(f"  ⚠ [{self.origin}] response primitive send failed: {e}")
            return

        # Always check the status code. Previously only exceptions were caught, so if the CSE
        # rejected the response we never knew. From the CSE's side the notification stayed pending
        # and it sent the same notification again. That causes duplicate notifications.
        #   ACME: PCH_PCU.handleNotifyRequest()
        #         BAD_REQUEST if m2m:rsp is missing or fails validation
        if r.status_code not in (200, 201, 204):
            self.respond_errors += 1
            print(f"  ⚠ [{self.origin}] response primitive rejected "
                  f"{r.status_code}: {r.text[:200]}")

    def _dispatch(self, body: dict) -> None:
        rqp = body.get("m2m:rqp") or {}
        pc = rqp.get("pc") or {}

        if not pc:
            return

        self.received += 1

        rqi = str(rqp.get("rqi") or "")
        if rqi and rqi in self._seen_rqi:
            self.duplicates += 1
            print(f"  ⚠ [{self.origin}] same notification arrived again "
                  f"(rqi={rqi}, total {self.duplicates}). "
                  f"The previous response did not reach the CSE.")
        elif rqi:
            self._seen_rqi.add(rqi)

        try:
            self.handler(pc)
        except Exception as e:
            print(f"  ⚠ [{self.origin}] error while handling notification: {e}")
        finally:
            # Send a response even if handling fails. Otherwise the queue gets stuck.
            self._respond(rqp)

    # ── Loop ────────────────────────────────────────────

    def run(self) -> None:
        print(f"  ✓ [{self.origin}] pollingChannel receive started "
              f"({om2m.strip_prefix(self.pcu_path)})")

        while not self._stop_evt.is_set():
            try:
                r = om2m._request_with_retry(
                    "get", om2m._url(self.pcu_path),
                    headers=self._headers(),
                    timeout=PCH_WAIT_SEC + 10,
                )
            except Exception as e:
                self.errors += 1
                if not self._stop_evt.is_set():
                    print(f"  ⚠ [{self.origin}] polling error: {e}")
                    time.sleep(PCH_BACKOFF_SEC)
                continue

            if r.status_code == 200:
                try:
                    self._dispatch(r.json())
                except json.JSONDecodeError:
                    print(f"  ⚠ [{self.origin}] notification parse failed: {r.text[:160]}")
                continue

            if r.status_code == 504:
                # Queue is empty. That is normal. Poll again right away.
                continue

            self.errors += 1
            print(f"  ⚠ [{self.origin}] polling response {r.status_code}: "
                  f"{r.text[:160]}")
            time.sleep(PCH_BACKOFF_SEC)

        print(f"  [{self.origin}] pollingChannel receive stopped "
              f"(notifications {self.received}, duplicates {self.duplicates}, "
              f"responses rejected {self.respond_errors}, polling errors {self.errors})")


def notification_target(ae_id: str) -> str:
    """
    Value to put in the subscription's nu.

    Giving an AE-ID instead of a URL makes the CSE check that AE's reachability and queue on the polling channel.
    Works only with rr=false and an existing <pollingChannel>.
    """
    return ae_id
