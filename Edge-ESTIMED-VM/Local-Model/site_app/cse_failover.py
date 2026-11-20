"""
Edge-ESTIMED-VM/Local-Model/site_app/cse_failover.py — site MN-CSE handover

If the site MN-CSE stops responding mid-training, find another IoT platform (CSE)
registered in the same MEP's MEC033 (IoT API) and move over to it.

[Detection — by failed responses]
A MEC033 registration only declares "this MEP has such a CSE"; it says nothing about liveness.
The registration can remain after the CSE dies. So liveness is checked by asking the CSE directly.

  In the background, CSEBase is RETRIEVEd every FL_CSE_PROBE_SEC (default 5 s).
  If connection failures, timeouts, 5xx or 404 continue uninterrupted for FL_CSE_FAILOVER_SEC (default 30 s),
  it is considered "dead". 401/403 mean alive but a permission problem, so they do not count.

  Why as short as 30 s: the IN-AE waits up to 600 s for uploads each round
  (FL_COLLECTION_NOTIFY_TIMEOUT), and the site app's data collection budget is
  540 s, 60 s shorter. A long detection window leaves no time after moving over to collect
  the data due for that round.

[Picking a replacement CSE — via MEC033]
  Among the same MEP's registered_iot_platforms candidates, excluding the current CSE,
  the first one whose CSEBase can be retrieved.

[The training app does the switch]
This module only detects and picks a candidate. Recreating resources, subscriptions and updating paths
is done by _maybe_cse_handover() in mn_ae_standard.py at a safe point.
If the background thread switched directly, paths would get out of sync with the collecting loop.

Disable: FL_CSE_HANDOVER=0
"""

import os
import threading
import time
from typing import Optional, Tuple

import config
import mec_iot


def enabled() -> bool:
    return os.getenv("FL_CSE_HANDOVER", "1") not in ("0", "false", "False", "")


PROBE_SEC = float(os.getenv("FL_CSE_PROBE_SEC", "5"))
FAILOVER_SEC = float(os.getenv("FL_CSE_FAILOVER_SEC", "30"))


def is_down(code: Optional[int]) -> bool:
    """Responses that mean the CSE is gone. None means connection failure / timeout."""
    return code is None or code == 404 or code >= 500


class CseWatch:
    """Keep probing the currently attached MN-CSE in the background."""

    def __init__(self, node: str, base: str, cse: str):
        self.node = node
        self._lock = threading.Lock()
        self._base, self._cse = base, cse
        self._down_since: Optional[float] = None
        self._last_code: Optional[int] = None
        self._stop = threading.Event()
        self._th: Optional[threading.Thread] = None

    def start(self) -> None:
        if self._th or not enabled():
            return
        self._th = threading.Thread(target=self._loop, daemon=True,
                                    name=f"cse-watch-{self.node}")
        self._th.start()
        print(f"  ✓ [{self.node}] MN-CSE 감시 시작 "
              f"({PROBE_SEC:.0f}초마다, {FAILOVER_SEC:.0f}초 무응답이면 핸드오버)")

    def stop(self) -> None:
        self._stop.set()

    def target(self) -> Tuple[str, str]:
        with self._lock:
            return self._base, self._cse

    def set_target(self, base: str, cse: str) -> None:
        """After a handover, watch the new CSE."""
        with self._lock:
            self._base, self._cse = base, cse
            self._down_since = None
            self._last_code = None

    def down_for(self) -> float:
        with self._lock:
            return 0.0 if self._down_since is None else time.time() - self._down_since

    def last_code(self) -> Optional[int]:
        with self._lock:
            return self._last_code

    def should_failover(self) -> bool:
        return enabled() and self.down_for() >= FAILOVER_SEC

    def _loop(self) -> None:
        while not self._stop.wait(PROBE_SEC):
            base, cse = self.target()
            code = mec_iot.probe(base, cse)
            with self._lock:
                if (base, cse) != (self._base, self._cse):
                    continue          # A handover happened in the meantime
                self._last_code = code
                if is_down(code):
                    if self._down_since is None:
                        self._down_since = time.time()
                        print(f"  ⚠ [{self.node}] MN-CSE 응답 없음 "
                              f"({code if code is not None else '연결 실패'}) "
                              f"— {base}/{cse}")
                elif self._down_since is not None:
                    gone = time.time() - self._down_since
                    self._down_since = None
                    print(f"  ✓ [{self.node}] MN-CSE 다시 응답 "
                          f"({gone:.0f}초 만에) — 핸드오버 안 함")


def pick_replacement(node: str,
                     current: Tuple[str, str]) -> Optional[Tuple[str, str, str]]:
    """
    From the same MEP's MEC033, pick a responding CSE that is not the current one.
    Returns: (base_url, cse_name, iotPlatformId) or None
    """
    mep = config.MEP_OF_NODE.get(node, "")
    if not mep:
        return None
    for base, cse, pid in mec_iot.candidates(mep):
        if (base, cse) == tuple(current):
            continue
        # Our AE does not exist on the candidate CSE yet. Asking with the AE identity could give 403 as an
        # unregistered originator, so it is checked with the admin identity.
        if mec_iot.cse_answers(base, cse, origin=config.ORIGINATOR):
            return base, cse, pid
        print(f"    · 후보 {base}/{cse} 응답 없음")
    return None
