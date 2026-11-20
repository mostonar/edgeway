"""
Edge-ESTIMED-VM/Local-Model/site_app/cse_failover.py — 사업장 MN-CSE 핸드오버

학습 도중 사업장 MN-CSE 가 응답을 멈추면, 같은 MEP 의 MEC033(IoT API)에
등록된 다른 IoT 플랫폼(CSE)을 찾아 그쪽으로 옮겨 간다.

[감지 — 응답 실패로]
MEC033 의 등록은 "이 MEP 에 이런 CSE 가 있다"는 신고일 뿐 생존 여부가 아니다.
CSE 가 죽어도 등록이 남아 있을 수 있다. 그래서 죽었는지는 CSE 에 직접 묻는다.

  뒤에서 FL_CSE_PROBE_SEC(기본 5초)마다 CSEBase 를 RETRIEVE 한다.
  연결 실패·타임아웃·5xx·404 가 FL_CSE_FAILOVER_SEC(기본 30초) 동안
  끊이지 않으면 "죽었다"로 본다. 401·403 은 살아 있는데 권한 문제라 세지 않는다.

  30초로 짧게 잡는 이유: IN-AE 는 라운드마다 업로드를 600초까지 기다리고
  (FL_COLLECTION_NOTIFY_TIMEOUT), 사업장 앱의 데이터 수집 예산은 그보다
  60초 짧은 540초다. 감지를 길게 잡으면 옮겨 간 뒤 그 라운드에 낼 데이터를
  모을 시간이 없다.

[대체 CSE 고르기 — MEC033 으로]
  같은 MEP 의 registered_iot_platforms 후보 중 지금 CSE 를 빼고,
  CSEBase 조회가 되는 첫 번째 것.

[전환은 학습 앱이 한다]
이 모듈은 감지와 후보 선택만 한다. 리소스 재생성·구독·경로 갱신은
mn_ae_standard.py 의 _maybe_cse_handover() 가 안전한 지점에서 한다.
뒤 스레드가 직접 바꾸면 수집 중인 루프와 경로가 엇갈린다.

끄기: FL_CSE_HANDOVER=0
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
    """CSE 가 없는 것으로 볼 응답. None 은 연결 실패·타임아웃."""
    return code is None or code == 404 or code >= 500


class CseWatch:
    """지금 붙어 있는 MN-CSE 를 뒤에서 계속 찔러 본다."""

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
        """핸드오버 뒤 새 CSE 를 감시한다."""
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
                    continue          # 그 사이 핸드오버됐다
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
    같은 MEP 의 MEC033 에서 지금 CSE 가 아닌, 응답하는 CSE 를 고른다.
    반환: (base_url, cse_name, iotPlatformId) 또는 None
    """
    mep = config.MEP_OF_NODE.get(node, "")
    if not mep:
        return None
    for base, cse, pid in mec_iot.candidates(mep):
        if (base, cse) == tuple(current):
            continue
        # 후보 CSE 에는 우리 AE 가 아직 없다. AE 신원으로 물으면 등록 안 된
        # originator 라 403 이 날 수 있어서 관리자 신원으로 확인한다.
        if mec_iot.cse_answers(base, cse, origin=config.ORIGINATOR):
            return base, cse, pid
        print(f"    · 후보 {base}/{cse} 응답 없음")
    return None
