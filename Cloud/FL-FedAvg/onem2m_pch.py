"""
fl/onem2m_pch.py — oneM2M <pollingChannel> 알림 수신

[이름에 대하여]
pch 는 oneM2M 이 <pollingChannel> 에 붙인 공식 약어다(m2m:pch, ty=15).
"폴링"이라는 일반 명사로 부르면 "주기적으로 긁는다"로 읽히는데,
이건 그것과 다르다. 구독도 알림도 그대로 두고 전달 방향만 뒤집는다.
규격 용어를 그대로 쓰는 편이 오해가 없다.

[왜 필요한가]
클라우드 IN-CSE 가 방화벽 뒤의 우리 AE 를 호출하지 못한다. 구독을 만들 때
CSE 가 알림 주소로 검증 요청(vrq)을 보내는데 그게 막혀서, 직접 알림이면
Read timeout 으로, nu 를 AE-ID 로 준 상태면 "Verification request failed"
로 끝난다.

TS-0001 은 이런 AE 를 "요청 도달 불가(request-unreachable)"로 보고
<pollingChannel> 을 쓰도록 정의한다. 우회가 아니라 규격에 있는 길이다.

  전:  CSE ──POST /notify──▶ AE          방화벽이 버림
  후:  AE ──GET pcu(롱폴링)──▶ CSE
       CSE ──알림을 그 응답으로──▶ AE     AE 가 건 연결이라 막히지 않음

구독도 알림 의미도 그대로다. 전달 경로만 바뀐다.

[이건 전역 설정이 아니다]
경로마다 조건이 다르므로 전달 방식도 경로마다 달라야 한다.

  로봇 → MN-CSE → MN-AE     사업장 내부, 도달 가능   → poa + 직접 알림
  IN-CSE → MN-AE / IN-AE    방화벽 너머, 도달 불가   → <pollingChannel>

한 플래그로 둘 다 정하면 방화벽과 아무 상관없는 사업장 안의 구독까지
같이 죽는다. config.local_notify_mode() / remote_notify_mode() 참고.

[생성 신원 — 실측]
<pollingChannel> 은 그 AE 본인만 만들 수 있다. 관리자도 안 된다.

  acme/resources/AE.py:103-106
    if childResource.ty == ResourceTypes.PCH:
        # Check correct originator. Even the ADMIN is not allowed that
        if self.aei != originator:
            raise ORIGINATOR_HAS_NO_PRIVILEGE('Originator must be the parent <AE>')

그래서 AE 리소스에서 aei 를 읽어 그 신원으로 생성한다. config.USE_AE_IDENTITY
가 꺼져 있어도(= 평소 요청은 CAdmin) 이 요청만은 AE 신원으로 나간다.
<remoteCSE> 밑에도 같은 규칙으로 하나 달 수 있다(CSR.py:126, self.csi 비교).

[롱폴링 응답 — 실측, ACME 기준]
  큐가 비면            504 + {"m2m:dbg": "Request Expiration Timestamp reached.
                              No request queued for originator: CIN-AE"}
                       에러가 아니다. 다시 걸면 된다.
  알림이 있으면        200 + {"m2m:rqp": {"fr","to","op":5,"rqi","pc":{"m2m:sgn":{...}}}}
  pc 안의 모양         기존 HTTP /notify 가 받던 본문과 동일하다.
                       그래서 알림 처리 로직을 그대로 재사용한다.
  채널이 없으면        404 + {"m2m:dbg": "resource not found"}
                       이 응답이 반복되면 채널 확보에 실패한 것이다.
  큐 단위              originator(AE-ID) 별이다.
"""

import json
import os
import threading
import time
from typing import Callable, Optional

import config
import onem2m_utils as om2m


# 롱폴링 한 번이 CSE 에서 열려 있는 시간. ACME 기본이 10초쯤이다.
# 너무 짧으면 재요청이 잦고, 너무 길면 종료가 굼뜨다.
PCH_WAIT_SEC = float(os.getenv("FL_PCH_WAIT_SEC", "20"))

# 연속 실패 시 쉬는 시간. CSE 가 잠깐 흔들릴 때 폭주하지 않게 한다.
PCH_BACKOFF_SEC = float(os.getenv("FL_PCH_BACKOFF_SEC", "3"))

DEFAULT_NAME = os.getenv("FL_PCH_NAME", "pch")

PCH_TY = 15


# ── 준비 단계 ────────────────────────────────────────────

def ae_identity(ae_path: str) -> Optional[str]:
    """
    AE 리소스에서 aei(AE-ID)를 읽는다. 없으면 None.

    AE-ID 를 추측하지 않고 CSE 에 물어보는 이유가 둘 있다.
    하나는 <pollingChannel> 생성이 이 값과 정확히 일치해야 통과하기 때문이고,
    다른 하나는 그 CSE 에 AE 가 등록돼 있는지를 같은 요청으로 알 수 있기
    때문이다. None 이면 "여긴 그 AE 가 없다"는 뜻이다.
    """
    res = om2m.get_resource(ae_path)
    if not res:
        return None
    return (res.get("m2m:ae") or {}).get("aei")


def find_channel(ae_path: str, aei: str) -> Optional[str]:
    """
    AE 밑에 이미 있는 <pollingChannel> 의 이름을 찾는다. 없으면 None.

    이름을 가정하지 않는 이유: AE 당 채널은 하나뿐인데(AE.py:109) 그 하나가
    다른 이름으로 이미 있으면 생성은 400 으로 막히고, 우리가 기대한 이름의
    pcu 를 긁으면 404 만 돌아온다. 있는 것의 실제 이름을 써야 한다.
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
    """<pollingChannel> 을 만든다. 성공하면 그 이름, 실패하면 None."""
    try:
        r = om2m._request_with_retry(
            "post", om2m._url(ae_path),
            json={"m2m:pch": {"rn": name}},
            headers=om2m._headers(PCH_TY, origin=aei),
            timeout=10,
        )
    except Exception as e:
        print(f"  ✗ pollingChannel 생성 오류: {e}")
        return None

    if r.status_code in (201, 409):
        return name
    print(f"  ✗ pollingChannel 생성 실패: {r.status_code} {r.text[:160]}")
    return None


def set_request_unreachable(ae_path: str, aei: str,
                            unreachable: bool = True) -> bool:
    """
    AE 의 rr(requestReachability)을 바꾼다.

    rr=true 면 CSE 가 AE 를 직접 호출하려 한다. 방화벽 뒤에서는 그게 실패하므로
    false 로 두어야 CSE 가 폴링 채널에 알림을 쌓는다.

    주의: 채널을 확보하기 전에 이걸 부르면 안 된다. 그 AE 는 "직접 부를 수도
    없고 가져갈 데도 없는" 상태가 되고, 그 AE 앞으로 만든 구독은 영영
    검증되지 않는다. 호출 순서는 setup() 이 강제한다.
    """
    try:
        r = om2m._request_with_retry(
            "put", om2m._url(ae_path),
            json={"m2m:ae": {"rr": not unreachable}},
            headers=om2m._headers(body=True, origin=aei),
            timeout=10,
        )
    except Exception as e:
        print(f"  ✗ rr 변경 오류: {e}")
        return False

    if r.status_code in (200, 204):
        print(f"  ✓ {om2m.strip_prefix(ae_path)} rr={not unreachable}")
        return True
    print(f"  ✗ rr 변경 실패: {r.status_code} {r.text[:160]}")
    return False


def setup(ae_path: str, notify_url: str,
          label: str = "", name: str = DEFAULT_NAME
          ) -> Optional["PchReceiver"]:
    """
    <pollingChannel> 수신을 통째로 준비한다. 성공하면 뜬 수신기, 실패하면 None.

    순서가 전부다.
      1) AE 의 aei 를 읽는다      — 생성은 AE 본인만 가능하고, 동시에
                                     이 CSE 에 AE 가 있는지 확인이 된다
      2) 채널을 확보한다          — 있으면 그 이름, 없으면 만든다
      3) 그제서야 rr=false        — 2가 실패했는데 3을 하면 AE 가 먹통이 된다
      4) 수신 스레드를 띄운다      — 2가 실패했는데 띄우면 404 만 무한 반복한다

    실패하면 CSE 상태를 아무것도 바꾸지 않고 None 을 돌려준다.
    호출부는 None 을 받으면 그 경로로는 알림을 못 받는다고 보면 된다.
    """
    tag = f" ({label})" if label else ""
    print(f"\n  oneM2M <pollingChannel> 준비{tag}")
    print(f"    대상: {om2m.strip_prefix(ae_path)}")

    aei = ae_identity(ae_path)
    if not aei:
        print(f"    ✗ 이 CSE 에 {om2m.strip_prefix(ae_path)} 가 없다.")
        print(f"      <pollingChannel> 은 AE 의 자식이라 AE 없이는 만들 수 없다.")
        return None
    print(f"    AE-ID: {aei}")

    chan = find_channel(ae_path, aei)
    if chan:
        print(f"    = 이미 있음: {chan}")
    else:
        chan = create_channel(ae_path, aei, name)
        if not chan:
            print(f"    ✗ 채널을 확보하지 못했다. rr 은 건드리지 않는다.")
            return None
        print(f"    ✓ 생성: {chan}")

    if not set_request_unreachable(ae_path, aei):
        print(f"    ✗ rr 을 끄지 못했다. CSE 는 계속 직접 호출을 시도한다.")
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
    받은 알림을 로컬 HTTP 핸들러로 그대로 넘기는 handler 를 만든다.

    알림 처리 로직(구독 검증, net 판정, 결과 수집)은 이미 Flask 라우트 안에
    있다. 그걸 뜯어 고치는 대신, pollingChannel 로 받은 본문을 같은 라우트에
    127.0.0.1 로 POST 한다. 전달 경로만 바뀌고 처리 코드는 그대로다.

    로컬 왕복이 한 번 늘지만 루프백이라 무시할 수준이고,
    검증된 코드를 건드리지 않는 쪽이 낫다.
    """
    import requests

    def _handler(pc: dict) -> None:
        try:
            requests.post(notify_url, json=pc, timeout=10)
        except Exception as e:
            print(f"  ⚠ 로컬 알림 전달 실패 ({notify_url}): {e}")

    return _handler


# ── 수신 ─────────────────────────────────────────────────

class PchReceiver(threading.Thread):
    """
    <pollingChannel> 을 롱폴링해 알림을 받아 handler 로 넘기는 스레드.

    handler 는 기존 HTTP /notify 가 받던 것과 같은 dict 를 받는다.
    즉 {"m2m:sgn": {...}} 형태다. 그래서 알림 처리 로직을 고칠 필요가 없다.

    직접 만들지 말고 setup() 을 쓴다. 채널이 없는데 이걸 띄우면
    404 만 끝없이 찍는다.
    """

    def __init__(self, pcu_path: str, origin: str,
                 handler: Callable[[dict], None]):
        super().__init__(daemon=True, name=f"pch-{origin}")
        self.pcu_path = pcu_path
        self.origin = origin
        self.handler = handler
        self._stop = threading.Event()
        self.received = 0
        self.errors = 0
        self.respond_errors = 0
        # 이미 처리한 rqi. 같은 알림이 다시 내려오는지 판별한다.
        # 처리 자체는 멱등이라 막지 않고, 보이게만 한다.
        self._seen_rqi: set[str] = set()
        self.duplicates = 0

    def stop(self) -> None:
        self._stop.set()

    # ── 내부 ────────────────────────────────────────────

    def _headers(self) -> dict:
        return om2m._headers(origin=self.origin)

    def _respond(self, rqp: dict) -> None:
        """
        응답 프리미티브를 돌려보낸다.

        CSE 는 알림을 보낸 뒤 응답을 기다린다. 안 보내면 그 요청이 미결로 남아
        다음 알림이 내려오지 않을 수 있다. rqi 를 그대로 돌려줘야 CSE 가
        어느 요청에 대한 응답인지 안다.
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
            print(f"  ⚠ [{self.origin}] 응답 프리미티브 전송 실패: {e}")
            return

        # 상태 코드를 반드시 본다. 전에는 예외만 잡고 넘겨서, CSE 가 응답을
        # 거절해도 우리는 몰랐다. CSE 입장에서는 그 알림이 미결로 남아
        # 같은 알림을 다시 내려보낸다. 중복 알림의 원인이 된다.
        #   ACME: PCH_PCU.handleNotifyRequest()
        #         m2m:rsp 가 없거나 검증에 실패하면 BAD_REQUEST
        if r.status_code not in (200, 201, 204):
            self.respond_errors += 1
            print(f"  ⚠ [{self.origin}] 응답 프리미티브 거절됨 "
                  f"{r.status_code}: {r.text[:200]}")

    def _handle(self, body: dict) -> None:
        rqp = body.get("m2m:rqp") or {}
        pc = rqp.get("pc") or {}

        if not pc:
            return

        self.received += 1

        rqi = str(rqp.get("rqi") or "")
        if rqi and rqi in self._seen_rqi:
            self.duplicates += 1
            print(f"  ⚠ [{self.origin}] 같은 알림이 다시 왔다 "
                  f"(rqi={rqi}, 누적 {self.duplicates}건). "
                  f"직전 응답이 CSE 에 닿지 않았다는 뜻이다.")
        elif rqi:
            self._seen_rqi.add(rqi)

        try:
            self.handler(pc)
        except Exception as e:
            print(f"  ⚠ [{self.origin}] 알림 처리 중 오류: {e}")
        finally:
            # 처리가 실패해도 응답은 보낸다. 안 보내면 큐가 막힌다.
            self._respond(rqp)

    # ── 루프 ────────────────────────────────────────────

    def run(self) -> None:
        print(f"  ✓ [{self.origin}] pollingChannel 수신 시작 "
              f"({om2m.strip_prefix(self.pcu_path)})")

        while not self._stop.is_set():
            try:
                r = om2m._request_with_retry(
                    "get", om2m._url(self.pcu_path),
                    headers=self._headers(),
                    timeout=PCH_WAIT_SEC + 10,
                )
            except Exception as e:
                self.errors += 1
                if not self._stop.is_set():
                    print(f"  ⚠ [{self.origin}] 폴링 오류: {e}")
                    time.sleep(PCH_BACKOFF_SEC)
                continue

            if r.status_code == 200:
                try:
                    self._handle(r.json())
                except json.JSONDecodeError:
                    print(f"  ⚠ [{self.origin}] 알림 파싱 실패: {r.text[:160]}")
                continue

            if r.status_code == 504:
                # 큐가 비었다. 정상이다. 바로 다시 건다.
                continue

            self.errors += 1
            print(f"  ⚠ [{self.origin}] 폴링 응답 {r.status_code}: "
                  f"{r.text[:160]}")
            time.sleep(PCH_BACKOFF_SEC)

        print(f"  [{self.origin}] pollingChannel 수신 종료 "
              f"(받은 알림 {self.received}개, 중복 {self.duplicates}건, "
              f"응답 거절 {self.respond_errors}건, 폴링 오류 {self.errors}회)")


def notification_target(ae_id: str) -> str:
    """
    구독의 nu 에 넣을 값.

    URL 대신 AE-ID 를 주면 CSE 가 그 AE 의 도달성을 보고 폴링 채널에 쌓는다.
    rr=false 이고 <pollingChannel> 이 있어야 동작한다.
    """
    return ae_id
