"""
표준 oneM2M FL 아키텍처 - IN-AE Coordinator (Drop-box aligned)
- IN-AE/cnt-fl-control : jobState + securityMode + privacyParams publish
- IN-AE/cnt-global-model: global model metadata publish
- IN-AE/cnt-local-updates : Drop-box root
- Loss Z-score 기반 이상치 탐지 (Poisoning Attack 방어)
- ACP 접근 제어 검증 로그
"""

import sys

import time
import os
import json
import threading
import requests as _req
import torch
from flask import Flask, request, make_response

import config
import valset

# IN-AE 의 AE-ID. ACP의 acor 와 X-M2M-Origin 에 쓰는 값이다.
ORIGIN_IN_AE = config.IN_AE_ID
import model_codec
import onem2m_utils as om2m
import onem2m_pch
from aggregator import Aggregator
from model_cvae import CVAE1D


class INAECoordinator:
    def __init__(self, max_rounds=config.GLOBAL_ROUNDS):
        self.max_rounds = max_rounds
        self.current_round = 0

        # IN-AE는 중앙 CSE에 붙는다. 사업장 CSE는 각 MN-AE가 담당한다.
        om2m.set_endpoint(*config.in_endpoint())
        om2m.set_in_endpoint(*config.in_endpoint())

        # 켜져 있으면 IN-AE 자신의 AE-ID 로 요청한다.
        if config.USE_AE_IDENTITY:
            om2m.set_originator(ORIGIN_IN_AE)
        self.in_cse_name = om2m.cse_name()

        # IN-AE 의 리소스는 전부 IN-CSE 에 있다. 경로가 하나라 원격뿐이다.
        self.notify_target = None
        self.pch_receiver = None

        self.fl_control_path   = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-fl-control"
        self.global_model_path = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-global-model"
        self.dropbox_root      = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-local-updates"
        # 고정 검증셋으로 잰 라운드별 성능을 여기에 남긴다.
        # 대시보드가 이걸 읽어 로그로 띄운다.
        self.fl_inference_path = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-fl-inference"

        # 리소스/구독은 모든 노드에 대해 만든다.
        # cold start 노드도 라운드 명령과 글로벌 모델은 받아야 추론을 할 수 있다.
        self.node_names = list(
            getattr(config, "ALL_NODES", None)
            or [f"mn{i}" for i in range(1, config.NUM_CLIENTS + 1)]
        )

        # 집계 대기 대상은 'FedAvg에 가중치를 올리는 노드'만이다.
        # 관측 전용(cold start)과 로컬 전용(local only) 둘 다 빼야 한다.
        # 예전에는 cold start만 뺐는데 그 목록이 보통 비어 있어서,
        # 로컬 전용 mn3가 집계 대기 수에 남아 3번째 가중치를
        # 영원히 기다리다 라운드가 멈췄다.
        self.non_contributing_nodes = list(
            getattr(config, "NON_CONTRIBUTING_NODES", None)
            or getattr(config, "COLD_START_NODES", [])
        )
        self.training_nodes = [
            n for n in self.node_names
            if n not in self.non_contributing_nodes
        ]
        self.expected_nodes = len(self.training_nodes)

        # 미기여 노드의 추론 결과는 따로 모은다(대시보드/로그용).
        self.cold_start_results = {}

        if self.non_contributing_nodes:
            print(
                f"  [FedAvg] 집계 참여: {self.training_nodes} / "
                f"미기여(추론 결과만 올림): {self.non_contributing_nodes}"
            )
        self.dropbox_by_node = {
            node: f"{self.dropbox_root}/cnt-{node}" for node in self.node_names
        }

        self.aggregator = Aggregator()

        self.app = Flask("IN-AE")
        self.notification_port = 6000
        self._setup_flask()

        self.collected_results = {}
        self.collection_lock = threading.Lock()
        self.collection_event = threading.Event()

        self.collection_timeout_sec = float(
            os.getenv("FL_COLLECTION_NOTIFY_TIMEOUT", "600")
        )
        self.command_subscription_wait_sec = float(
            os.getenv("FL_COMMAND_SUBSCRIPTION_WAIT", "60")
        )
        self.model_publish_settle_sec = float(
            os.getenv("FL_MODEL_PUBLISH_SETTLE_SEC", "1.0")
        )
        self.current_global_model_uri = None

        # -----------------------
        # 추가: 글로벌 모델/평가 경로
        # -----------------------
        self.global_model_dir = os.getenv(
            "FL_GLOBAL_MODEL_DIR",
            "/tmp/fl_models/global"
        )
        os.makedirs(self.global_model_dir, exist_ok=True)

        self.latest_global_model_path = None

    @staticmethod
    def _decode_cin_content(cin_obj):
        if not isinstance(cin_obj, dict):
            return None

        cin = cin_obj.get("m2m:cin", cin_obj)

        if not isinstance(cin, dict):
            return None

        con = cin.get("con")
        if con is None:
            return None

        try:
            data = (
                json.loads(con)
                if isinstance(con, str)
                else con
            )
        except (TypeError, json.JSONDecodeError):
            return None
        return data if isinstance(data, dict) else None

    def _map_notified_node(
        self,
        sur: str,
        result_data: dict | None
    ):
        node_from_sur = None

        for node, cnt_path in self.dropbox_by_node.items():
            if cnt_path in sur or f"cnt-{node}" in sur:
                node_from_sur = node
                break

        node_from_con = None

        if result_data:
            raw_node = str(
                result_data.get("node", "")
            ).strip().lower()
            # 별칭은 cold start 노드를 포함한 '모든 노드'에 대해 만들어야 한다.
            # (그 노드도 알림을 보내므로 이름을 해석할 수 있어야 한다)
            aliases = {
                f"mn-ae-{i}": f"mn{i}"
                for i in range(
                    1,
                    len(self.node_names) + 1
                )
            }
            node_from_con = aliases.get(
                raw_node,
                raw_node
            )
            if node_from_con not in self.dropbox_by_node:
                node_from_con = None

        if (
            node_from_sur
            and node_from_con
            and node_from_sur != node_from_con
        ):
            print(
                f"  ⚠ notification node mismatch: "
                f"sur={node_from_sur}, "
                f"con={node_from_con}"
            )
            return None
        return node_from_con or node_from_sur

    # -----------------------
    # Flask notify handler
    # -----------------------
    def _setup_flask(self):
        @self.app.route("/notify", methods=["POST"])
        def handle_notification():
            try:
                body = request.get_json(silent=True) or {}
                sgn = body.get("m2m:sgn", {})

                # Subscription verification
                if sgn.get("vrq") is True:
                    print(
                        "  [VERIFY] IN-AE "
                        "subscription verification OK"
                    )
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                nev = sgn.get("nev", {}) or {}
                sur = str(sgn.get("sur", ""))
                net = nev.get("net", 0)
                net_values = (
                    net if isinstance(net, list)
                    else [net]
                )

                try:
                    is_cin_created = 3 in {
                        int(value) for value in net_values
                    }
                except (TypeError, ValueError):
                    is_cin_created = False

                print(f"\n  [NOTIFY] sur={sur}, net={net}")

                if not is_cin_created:
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                # 정상 수집 경로:
                # NOTIFY body의 rep.m2m:cin.con을 직접 사용
                rep = nev.get("rep", {}) or {}
                result_data = self._decode_cin_content(rep)
                fired_node = self._map_notified_node(
                    sur,
                    result_data
                )

                if not fired_node:
                    print(
                        "  ⚠ cannot map notification "
                        "to an expected node"
                    )
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                # CSE가 rep를 생략한 경우에만
                # 해당 컨테이너의 최신 CIN을 한 번 조회
                if result_data is None:
                    cin_obj = om2m.get_latest_content_instance(
                        self.dropbox_by_node[fired_node]
                    )
                    result_data = self._decode_cin_content(
                        cin_obj
                    )

                if not result_data:
                    print(
                        f"  ⚠ {fired_node}: "
                        f"local update con is missing"
                    )
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                update_type = (
                    str(result_data.get("type", ""))
                    .lower()
                    .replace("_", "-")
                )

                # fl-update   : 가중치 포함, FedAvg 대상
                # fl-inference : 지표만, 미기여 노드(관측 전용/로컬 전용)
                if update_type not in ("fl-update", "fl-inference"):
                    print(
                        f"  ⚠ {fired_node}: "
                        f"unexpected type={update_type!r}"
                    )
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                try:
                    cin_round = int(
                        result_data.get("round", -1)
                    )
                except (TypeError, ValueError):
                    cin_round = -1

                if cin_round != self.current_round:
                    print(
                        f"  ⚠ {fired_node}: "
                        f"round mismatch "
                        f"(cin={cin_round}, "
                        f"expected={self.current_round})"
                    )
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                with self.collection_lock:
                    if (update_type == "fl-inference"
                            or fired_node
                            in self.non_contributing_nodes):
                        # 가중치 없는 업로드. FedAvg 대상이 아니므로
                        # 집계 대기 수에 넣지 않는다.
                        self.cold_start_results[fired_node] = result_data
                        print(
                            f"  [NO-AGG/NOTIFY] "
                            f"{fired_node} Round {cin_round} "
                            f"val_auroc={result_data.get('val_auroc')} "
                            f"(FedAvg 미기여)"
                        )
                    else:
                        self.collected_results[
                            fired_node
                        ] = result_data
                        collected = len(
                            self.collected_results
                        )

                        print(
                            f"  [COLLECT/NOTIFY] "
                            f"{fired_node} Round {cin_round} "
                            f"({collected}/{self.expected_nodes})"
                        )

                        if collected >= self.expected_nodes:
                            print(
                                "  ✓ all training nodes collected "
                                "by notification -> set event"
                            )
                            self.collection_event.set()

                resp = make_response("", 200)
                resp.headers["X-M2M-RSC"] = "2000"
                return resp

            except Exception as e:
                print(f"  ✗ Notification error: {e}")
                import traceback
                traceback.print_exc()
                return make_response("", 500)

    def start_server(self):
        def run():
            self.app.run(host="0.0.0.0", port=self.notification_port, threaded=True, use_reloader=False)

        th = threading.Thread(target=run, daemon=True)
        th.start()
        time.sleep(1)
        print(f"  ✓ IN-AE Notification server on port {self.notification_port}")
        self._setup_notify_path()

    def _setup_notify_path(self):
        """
        알림을 어느 경로로 받을지 정한다.

        IN-AE 가 구독하는 것은 전부 클라우드 IN-CSE 안에 있다(드롭박스).
        그 CSE 는 캠퍼스 방화벽 뒤의 우리를 호출하지 못하므로 기본이 pch 다.
        MN-AE 와 달리 IN-AE 는 그 CSE 에 정식 등록된 AE 라서
        <pollingChannel> 을 가질 자격이 있다.

        direct 로 두면 구독 생성이 CSE 쪽 검증 요청에서 멈췄다가
        우리 read timeout 으로 끝난다. 복구용으로만 쓴다.
        """
        port = self.notification_port
        ae_path = f"{om2m.cse_name()}/{config.IN_AE_NAME}"

        if config.remote_notify_mode() != "pch":
            # rr=true 만으로는 부족하다. poa 로 수신 주소를 알려야
            # CSE가 알림 대상을 검증할 수 있다.
            poa = f"http://{config.NOTIFY_HOST}:{port}"
            om2m.set_point_of_access(ae_path, [poa], reachable=True)
            self.notify_target = f"{poa}/notify"
            return

        self.pch_receiver = onem2m_pch.setup(
            ae_path, f"http://127.0.0.1:{port}/notify", label="IN-AE")
        if not self.pch_receiver:
            self.notify_target = None
            print("    ⚠ IN-AE 는 알림을 받을 수 없다. 드롭박스 구독을 "
                  "만들지 않는다.")
            return
        self.notify_target = self.pch_receiver.origin

    # -----------------------
    # ACP 검증
    # -----------------------
    def _verify_acp(self):
        """
        ACP가 의도대로 걸렸는지 점검한다.

        리소스마다 사는 CSE가 다르다. IN-AE의 제어·모델·드롭박스는 클라우드
        IN-CSE에, MN-AE의 캐시는 각 사업장 MN-CSE에 있다. 그래서 점검할 때마다
        해당 CSE로 바인딩을 옮긴다. 끝나면 IN-CSE로 되돌린다.

        404(리소스 없음)와 403(접근 거부)은 다른 문제다. 전자는 설정이 덜 된
        것이고 후자가 정책 문제다. 섞어서 보고하면 진단이 안 된다.
        """
        print("\n=== ACP 접근 제어 검증 ===")

        in_cse = config.in_endpoint()[1]

        def bind(node=None):
            """node가 None이면 IN-CSE, 아니면 그 사업장 MN-CSE로."""
            if node is None:
                om2m.set_endpoint(*config.in_endpoint())
                return in_cse
            base, cse = config.mn_endpoint(node)
            om2m.set_endpoint(base, cse)
            return cse

        def probe(path, origin, node=None):
            """
            RETRIEVE 한 번. (status_code, 본문 일부)를 돌려준다.

            헤더는 om2m._headers()를 쓴다. 직접 만들면 Content-Type 같은 것이
            어긋나고, 그러면 ACP가 아니라 요청 형식 때문에 실패한다.
            """
            bind(node)
            h = om2m._headers()
            h["X-M2M-Origin"] = origin
            # 이 점검은 원래 RVI 2a 로 보냈다. IN-AE 가 srv=["2a","3"] 으로
            # 등록돼 있어 둘 다 유효하다. 바꿀 이유가 없으므로 그대로 둔다.
            h["X-M2M-RVI"] = "2a"
            url = om2m._url(path)
            try:
                r = om2m._request_with_retry("get", url, headers=h, timeout=5)
                return r.status_code, r.text[:120]
            except Exception as e:
                return None, f"{type(e).__name__}: {e}"

        def check(label, path, origin, expect_allow, node=None):
            code, body = probe(path, origin, node)
            short = path.split("/", 1)[-1]

            if code is None:
                print(f"    ⚠ 확인 못함  {short}  (Origin={origin})  {body}")
                return
            if code == 404:
                # 정책이 아니라 리소스가 없는 것이다. 버그로 보고하지 않는다.
                print(f"    ⚠ 리소스 없음  {short}  (Origin={origin})")
                return

            allowed = code in (200, 201)
            blocked = code == 403 or "no privilege" in body

            if expect_allow:
                mark = "✓ 허용" if allowed else f"✗ 차단됨(버그) HTTP {code}"
            else:
                mark = "✓ 차단" if blocked else f"✗ 허용됨(버그) HTTP {code}"
            print(f"    {mark}  {short}  (Origin={origin})")

        # ── [1] acpi 연결 확인 ──────────────────────────
        print("\n  [1] acpi 연결 확인")
        ae = config.IN_AE_NAME

        in_paths = [
            (f"{in_cse}/{ae}/cnt-fl-control", None),
            (f"{in_cse}/{ae}/cnt-global-model", None),
            (f"{in_cse}/{ae}/cnt-local-updates", None),
        ]
        for node in config.ALL_NODES:
            in_paths.append((f"{in_cse}/{ae}/cnt-local-updates/cnt-{node}", None))
            mn_cse = config.mn_endpoint(node)[1]
            tr = config.training_ae_name(node)
            in_paths.append((f"{mn_cse}/{tr}/cnt-local-model", node))
            in_paths.append((f"{mn_cse}/{tr}/cnt-net-status", node))

        for path, node in in_paths:
            bind(node)
            res = om2m.get_resource(path)
            short = path.split("/", 1)[-1]
            if res is None:
                print(f"    ⚠ 없음  {short}")
                continue
            acpi = res.get("m2m:cnt", {}).get("acpi", [])
            print(f"    {'✓' if acpi else '⚠ acpi 없음'}  {short}  acpi={acpi}")

        # ── [2] 차단돼야 하는 접근 ──────────────────────
        # 한 사업장이 다른 사업장의 것을 들여다보려는 시도.
        print("\n  [2] 차단돼야 하는 접근")
        mn_a = config.mn_endpoint("mn1")[1]
        mn_b = config.mn_endpoint("mn2")[1]

        # 학습 앱의 이름과 AE-ID. 박아 쓰면 이름을 바꿀 때 여기만 남는다.
        tr1, tr2, tr3 = (config.training_ae_name(n)
                         for n in ("mn1", "mn2", "mn3"))
        id1, id2, id3 = (config.training_ae_id(n)
                         for n in ("mn1", "mn2", "mn3"))

        # 원래 학습앱2/cnt-sensor-data 를 봤는데 그 컨테이너는 없어졌다.
        # 텔레메트리가 로봇 AE 아래로 갔고 거기엔 우리 ACP 가 없다.
        # 같은 의도(남의 사업장 것은 못 본다)를 cnt-net-status 로 확인한다.
        check("", f"{mn_b}/{tr2}/cnt-net-status", id1, False, "mn2")
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn2", id1, False)
        check("", f"{mn_a}/{tr1}/cnt-local-model", id2, False, "mn1")
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", id3, False)
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", id1, False)

        # ── [3] 허용돼야 하는 접근 ──────────────────────
        print("\n  [3] 허용돼야 하는 접근")
        mn_c = config.mn_endpoint("mn3")[1]

        check("", f"{in_cse}/{ae}/cnt-fl-control", id1, True)
        check("", f"{in_cse}/{ae}/cnt-global-model", id2, True)
        # 원래 학습앱3/cnt-sensor-data 자리. 위와 같은 이유로 옮긴다.
        check("", f"{mn_c}/{tr3}/cnt-net-status", id3, True, "mn3")
        check("", f"{mn_c}/{tr3}/cnt-local-model", id3, True, "mn3")
        check("", f"{in_cse}/{ae}/cnt-fl-control", "CIN-AE", True)
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", "CIN-AE", True)

        # 점검이 끝나면 반드시 IN-CSE로 되돌린다.
        # 여기서 빠뜨리면 이후 라운드 명령이 엉뚱한 CSE로 간다.
        bind(None)
        print()

    @staticmethod
    def _read_acpi(path: str) -> list:
        res = om2m.get_resource(path)
        if res and "m2m:cnt" in res:
            return res["m2m:cnt"].get("acpi", [])
        return []

    def _prepare_dropbox(self):
        print("\n  Prepare drop-box containers (delete & recreate to clear old CINs)...")
        dropbox_path = f"{om2m.cse_name()}/{config.IN_AE_NAME}/cnt-local-updates"

        # 삭제 전 ACP 정보 저장
        root_acpi = self._read_acpi(dropbox_path)
        node_acpis = {
            node: self._read_acpi(f"{self.dropbox_root}/cnt-{node}")
            for node in self.node_names
        }

        om2m.delete_resource(dropbox_path)
        om2m.create_container(f"{om2m.cse_name()}/{config.IN_AE_NAME}", "cnt-local-updates",
                              mni=5000, mbs=50_000_000)
        if root_acpi:
            om2m.update_acpi(dropbox_path, root_acpi)

        for node in self.node_names:
            parent = self.dropbox_root
            om2m.delete_resource(f"{parent}/cnt-{node}")
            time.sleep(0.5)
            om2m.create_container(parent, f"cnt-{node}", mni=2000, mbs=20_000_000)
            if node_acpis[node]:
                om2m.update_acpi(f"{parent}/cnt-{node}", node_acpis[node])
            print(f"    ✓ {parent}/cnt-{node}")

        # CSE가 컨테이너 생성을 커밋할 시간 확보
        time.sleep(2.0)

    def _subscribe_dropbox(self):
        print("\n  Subscribe drop-box per-node containers (net=3 CIN create)...")
        if not self.notify_target:
            print("    ✗ 알림 경로가 없어 구독을 만들지 않는다. "
                  "만들어도 CSE 가 검증하지 못한다.")
            return
        for node, cnt_path in self.dropbox_by_node.items():
            sub_name = f"sub_in_dropbox_{node}"
            om2m.delete_subscription(cnt_path, sub_name)
            time.sleep(0.2)

            res = om2m.create_subscription(
                parent_path=cnt_path,
                subscription_name=sub_name,
                notification_uri=self.notify_target,
                event_types=[3],
                use_nct=None,
            )
            if res:
                print(f"    ✓ {node} subscribed -> {cnt_path}")

    @staticmethod
    def _subscription_present(sub_path: str) -> bool:
        """
        구독이 있는지만 본다. 내용은 읽지 않는다.

        403 도 '있다' 로 친다. 구독은 학습 앱이 자기 AE-ID 로 만들고 acpi 가
        없어서, 만든 쪽이 아닌 CIN-AE 가 읽으면 ACME 가 403 을 돌려준다.
        ACME 는 리소스를 먼저 찾고 권한을 나중에 보므로, 없으면 404 다.

        CSE 가 권한 없는 리소스를 404 로 숨길 수도 있으므로, 404 면 관리자
        신원으로 한 번 더 본다. 존재 확인만 하고 내용은 쓰지 않는다.
        """
        for origin in (None, config.ORIGINATOR):
            try:
                r = om2m._request_with_retry(
                    "get", om2m._url(sub_path),
                    headers=om2m._headers(origin=origin), timeout=10)
            except Exception:
                continue
            if r.status_code in (200, 403):
                return True
        return False

    def _wait_for_mn_command_subscriptions(self) -> bool:
        """
        Round 1 command를 발행하기 전에 모든 MN-AE의 cnt-fl-control
        subscription이 생성됐는지 확인함.

        이 확인은 FL round polling이 아니라 시작 시점의 readiness barrier임.
        """
        deadline = time.time() + self.command_subscription_wait_sec
        expected = {
            node: f"{self.fl_control_path}/sub_{node}"
            for node in self.node_names
        }
        ready = set()

        print("\n  Wait for MN-AE command subscriptions...")

        while time.time() < deadline:
            for node, sub_path in expected.items():
                if node in ready:
                    continue
                if self._subscription_present(sub_path):
                    ready.add(node)
                    print(f"    ✓ {node} command subscription ready")

            # 구독 준비는 '모든 노드'를 기다린다.
            # cold start 노드도 라운드 명령을 받아야 추론을 할 수 있으므로,
            # 집계 대기 수(expected_nodes = 학습 노드 수)와는 기준이 다르다.
            if len(ready) == len(self.node_names):
                return True

            time.sleep(1.0)

        missing = [node for node in self.node_names if node not in ready]
        print(
            f"  ✗ command subscription readiness timeout; "
            f"missing={missing}"
        )
        return False

    # -----------------------
    # FL publish
    # -----------------------
    def _publish_job_state(
        self,
        state: str,
        round_num: int
    ):
        data = {
            "type": "fl-command",
            "jobState": state,
            "currentRound": round_num,
            "maxRounds": self.max_rounds,
            "globalModelUri": (
                self.current_global_model_uri
                or f"{self.global_model_path}/la"
            ),
            # DP 는 기본으로 끈다(config.DP_ENABLED 주석 참고).
            "securityMode": "DP" if config.DP_ENABLED else "NONE",
            "privacyParams": {
                "epsilon": config.DP_EPSILON,
                "delta": config.DP_DELTA,
                "max_grad_norm": config.DP_MAX_GRAD_NORM,
            },
            "timestamp": time.time(),
        }

        result = om2m.create_content_instance(
            self.fl_control_path,
            data
        )

        if result:
            print(
                f"  ✓ jobState: {state} "
                f"(Round {round_num}, {data['securityMode']}, "
                f"model={data['globalModelUri']})"
            )
        else:
            print(
                f"  ✗ failed to publish jobState={state} "
                f"for round {round_num}"
            )

        return result

    @staticmethod
    def _state_dict_matches_model(model: torch.nn.Module, state_dict: dict) -> bool:
        if not isinstance(state_dict, dict):
            return False

        expected = model.state_dict()
        if set(expected) != set(state_dict):
            return False

        return all(
            tuple(expected[key].shape) == tuple(state_dict[key].shape)
            for key in expected
        )

    def _ensure_initial_global_model(self):
        """
        MN-AE와 동일한 CVAE1D 구조로 Round 0 모델을 준비한다.
        모든 노드가 같은 초기값에서 출발해야 FedAvg가 성립한다.
        기존 파일이 현재 설정과 호환되지 않으면 자동으로 다시 생성함.
        """
        os.makedirs(self.global_model_dir, exist_ok=True)
        model_path = os.path.join(
            self.global_model_dir,
            "global_round0.pt"
        )

        ae_cfg = getattr(config, "AE_CFG", config.AEConfig())
        model = CVAE1D(
            n_channels=ae_cfg.n_channels,
            cond_dim=getattr(ae_cfg, "cond_dim", 0),
            latent_dim=ae_cfg.latent_dim,
            hidden=getattr(ae_cfg, "hidden", 48),
            seq_len=ae_cfg.seq_len,
        )

        reuse_existing = False
        if os.path.exists(model_path):
            try:
                existing = torch.load(model_path, map_location="cpu")
                reuse_existing = self._state_dict_matches_model(
                    model,
                    existing
                )
            except Exception as exc:
                print(
                    f"  ⚠ failed to inspect existing Round 0 model: {exc}"
                )

        if reuse_existing:
            print(
                f"  ✓ Initial global model is compatible: {model_path}"
            )
        else:
            if os.path.exists(model_path):
                print(
                    "  ⚠ incompatible Round 0 checkpoint detected; "
                    "recreate it"
                )
            torch.save(model.state_dict(), model_path)
            print(
                f"  ✓ Initial global model saved: {model_path} "
                f"(channels={ae_cfg.n_channels}, "
                f"latent={ae_cfg.latent_dim}, "
                f"seq_len={ae_cfg.seq_len})"
            )

        self.latest_global_model_path = model_path
        return model_path

    def _publish_global_model(self, round_num: int):
        model_path = os.path.join(
            self.global_model_dir,
            f"global_round{round_num}.pt"
        )

        if not os.path.exists(model_path):
            print(
                f"  ✗ global model file does not exist: {model_path}"
            )
            return None

        data = {
            "type": "global-model",
            "global_round": round_num,
            "model_ready": True,
            "timestamp": time.time(),
        }

        # 글로벌 가중치는 con 에 실어 배포한다(내려주는 방향도 oneM2M 경유).
        # 사업장 앱은 다른 머신이라 이것 말고는 받을 길이 없으므로,
        # 싣지 못하면 배포하지 않는다.
        try:
            enc = model_codec.encode_state_dict(
                torch.load(model_path, map_location="cpu"),
                fp16=getattr(config, "FL_WEIGHTS_FP16", True),
            )
        except Exception as exc:
            print(
                f"  ✗ global weight embedding 실패 "
                f"({type(exc).__name__}: {exc})"
            )
            return None

        limit = getattr(config, "FL_MAX_CIN_BYTES", 48 * 1024)
        if enc["weights_bytes"] <= limit:
            data.update(enc)
            print(
                f"    ↓ global weights in con: "
                f"{model_codec.describe(enc)}"
            )
        elif getattr(config, "FL_CHUNK_WEIGHTS", True):
            manifest = self._publish_weight_chunks(round_num, enc)
            if not manifest:
                print("  ✗ 글로벌 가중치 조각 업로드 실패")
                return None
            data.update(manifest)
            print(
                f"    ↓ global weights in con: "
                f"{model_codec.describe(manifest)} "
                f"-> {manifest['weights_chunk_container']}"
            )
        else:
            print(
                f"  ✗ global weights "
                f"{enc['weights_bytes'] / 1024:.0f}KB > "
                f"limit {limit / 1024:.0f}KB, 조각 전송 꺼짐 "
                f"(FL_CHUNK_WEIGHTS=1 필요)"
            )
            return None

        result = om2m.create_content_instance(
            self.global_model_path,
            data
        )

        if not result:
            print(
                f"  ✗ failed to publish global model Round {round_num}"
            )
            return None

        cin = result.get("m2m:cin", {}) if isinstance(result, dict) else {}
        resource_name = cin.get("rn")
        self.current_global_model_uri = (
            f"{self.global_model_path}/{resource_name}"
            if resource_name
            else f"{self.global_model_path}/la"
        )

        self.latest_global_model_path = model_path
        print(
            f"  ✓ Global model published Round {round_num} "
            f"-> {self.current_global_model_uri}"
        )

        # CSE가 CIN을 반영한 뒤 command NOTIFY를 보내도록 짧게 대기
        if self.model_publish_settle_sec > 0:
            time.sleep(self.model_publish_settle_sec)

        return result

    # -----------------------
    #def _collect_results_polling(self, round_num: int) -> dict:
    #    print(f"\n  Round {round_num} collect results (Polling fallback)...")
    #    results = {}
    #    round_label = f"round_{round_num}"

    #    for node, cnt_path in self.dropbox_by_node.items():
    #        cin_obj = om2m.retrieve_first_cin_by_label(cnt_path, round_label)
    #        if cin_obj and "m2m:cin" in cin_obj:
    #            con = cin_obj["m2m:cin"]["con"]
    #            data = json.loads(con) if isinstance(con, str) else con
    #            if data.get("round") == round_num:
    #                results[node] = data
    #                print(f"    ✓ {node}: train={data['train_loss']:.4f}, n={data['num_samples']}")
    #    return results
    # -----------------------

    def _collect_results_latest(
        self,
        round_num: int,
        nodes=None
    ) -> dict:
        """
        NOTIFY 누락 복구용 단발성 조회.
        반복 polling에는 사용하지 않음.
        """
        target_nodes = (
            list(nodes)
            if nodes is not None
            else self.training_nodes
        )

        results = {}

        for node in target_nodes:
            cnt_path = self.dropbox_by_node[node]

            cin_obj = om2m.get_latest_content_instance(
                cnt_path
            )

            data = self._decode_cin_content(cin_obj)

            if not data:
                continue

            update_type = (
                str(data.get("type", ""))
                .lower()
                .replace("_", "-")
            )

            try:
                update_round = int(
                    data.get("round", -1)
                )
            except (TypeError, ValueError):
                update_round = -1

            payload_node = str(
                data.get("node", "")
            ).lower()

            if (
                update_type == "fl-update"
                and update_round == round_num
                and payload_node == node
            ):
                results[node] = data

                print(
                    f"    ✓ fallback {node}: "
                    f"train={data['train_loss']:.4f}, "
                    f"n={data['num_samples']}"
                )

        return results

    # -----------------------
    # Anomaly Detection
    # -----------------------
    def _detect_anomaly(self, results: dict, round_num: int,
                        z_threshold: float = 2.0,
                        abs_threshold: float = 1.5,
                        bypass_rounds: int = 3) -> list:
        import math

        nodes = list(results.keys())
        if len(nodes) < 2:
            print("  [Anomaly Detection] 노드 수 < 2, 스킵")
            return []

        if round_num <= bypass_rounds:
            print(f"  [Anomaly Detection] Round {round_num} <= {bypass_rounds} (early bypass), 스킵")
            return []

        train_losses = {n: results[n]["train_loss"] for n in nodes}
        val_losses   = {n: results[n]["val_loss"]   for n in nodes}

        def z_scores(loss_dict):
            vals = list(loss_dict.values())
            mean = sum(vals) / len(vals)
            std  = math.sqrt(sum((v - mean) ** 2 for v in vals) / len(vals))
            if std < 1e-8:
                return {n: 0.0 for n in loss_dict}
            return {n: abs(v - mean) / std for n, v in loss_dict.items()}

        train_z = z_scores(train_losses)
        val_z   = z_scores(val_losses)

        print(f"\n  [Anomaly Detection] Loss Z-score (z>{z_threshold} AND loss>{abs_threshold}):")
        anomalies = []
        for n in nodes:
            tz = train_z[n]
            vz = val_z[n]
            tl = train_losses[n]
            vl = val_losses[n]
            is_anomaly = ((tz > z_threshold and tl > abs_threshold) or
                          (vz > z_threshold and vl > abs_threshold))
            status = "✗ ANOMALY" if is_anomaly else "✓ normal"
            print(f"    {n}: train_loss={tl:.4f} (z={tz:.2f}), "
                  f"val_loss={vl:.4f} (z={vz:.2f})  {status}")
            if is_anomaly:
                anomalies.append(n)

        return anomalies

    # -----------------------
    # 가중치 조각 (CSE 페이로드 상한 대응)
    # -----------------------
    def _ensure_weights_container(self, parent_path: str) -> str:
        om2m.create_container(
            parent_path,
            "cnt-weights",
            mni=1000,
            mbs=60_000_000,
        )
        return f"{parent_path}/cnt-weights"

    def _make_chunk_fetcher(self, meta: dict):
        """매니페스트를 보고 조각을 이름으로 직접 RETRIEVE 하는 함수를 만든다."""
        cnt_path = meta.get("weights_chunk_container")
        if not cnt_path:
            return None

        chunk_round = meta.get(
            "weights_chunk_round",
            meta.get("round", meta.get("global_round")),
        )

        def fetch(index: int):
            res = om2m.get_resource(
                f"{cnt_path}/"
                f"{model_codec.chunk_resource_name(chunk_round, index)}"
            )
            if not res:
                return None
            con = (res.get("m2m:cin") or {}).get("con")
            try:
                obj = json.loads(con) if isinstance(con, str) else con
            except (TypeError, ValueError):
                return None
            return (obj or {}).get("d")

        return fetch

    def _publish_weight_chunks(self, round_num: int, enc: dict):
        """글로벌 모델 가중치를 조각내어 자식 컨테이너에 올리고 매니페스트를 돌려준다."""
        chunks, manifest = model_codec.split_for_cin(
            enc,
            getattr(config, "FL_MAX_CIN_BYTES", 48 * 1024),
        )

        cnt_path = self._ensure_weights_container(self.global_model_path)

        for i, part in enumerate(chunks):
            res = om2m.create_content_instance(
                cnt_path,
                {"i": i, "d": part},
                resource_name=model_codec.chunk_resource_name(round_num, i),
            )
            if not res:
                print(f"    ✗ 글로벌 가중치 조각 {i + 1}/{len(chunks)} 실패")
                return None

        manifest["weights_chunk_container"] = cnt_path
        manifest["weights_chunk_round"] = round_num
        return manifest

    # -----------------------
    # Aggregation
    # -----------------------
    def _aggregate(self, results: dict, round_num: int):
        print(f"\n  FedAvg (Round {round_num})...")

        state_dicts = {}
        num_samples = {}
        for node, data in results.items():
            # 가중치는 con 에 실려 온다. 없으면 이번 라운드 집계에서 뺀다.
            sd = None
            src = "con(base64)"
            try:
                sd = model_codec.decode_state_dict(
                    data,
                    fetch_chunk=self._make_chunk_fetcher(data),
                )
            except Exception as exc:
                print(
                    f"    ⚠ {node}: con 가중치 사용 불가 "
                    f"({type(exc).__name__}: {exc})"
                )

            if sd is None:
                print(
                    f"    ✗ {node}: con 에 가중치가 없음 "
                    f"-> 이번 라운드 제외"
                )
                continue

            state_dicts[node] = sd
            num_samples[node] = data["num_samples"]
            print(f"    ← {node} (n={data['num_samples']}, via {src})")

        anomalies = self._detect_anomaly(results, round_num)

        if anomalies:
            print(f"  ⚠ 이상 노드 제외: {anomalies}")
            for n in anomalies:
                state_dicts.pop(n, None)
                num_samples.pop(n, None)

        if not state_dicts:
            print("  ✗ 정상 노드 없음, aggregation 스킵")
            return 0.0, None

        client_states = [(sd, num_samples[n]) for n, sd in state_dicts.items()]

        # 어떤 가중치로 합쳤는지 남긴다. 사업장 균등인지 샘플 수 비례인지가
        # 결과 해석을 바꾸므로 로그에 보여야 한다.
        print(f"    집계 가중: {self.aggregator.describe(client_states)}")
        print(f"      노드 순서: {', '.join(state_dicts.keys())}")

        global_state = self.aggregator.aggregate(client_states)

        out_path = f"{self.global_model_dir}/global_round{round_num}.pt"
        torch.save(global_state, out_path)
        self.latest_global_model_path = out_path

        normal_nodes = [n for n in results if n not in anomalies]
        total = sum(results[n]["num_samples"] for n in normal_nodes)
        avg_loss = sum(results[n]["train_loss"] * results[n]["num_samples"]
                       for n in normal_nodes) / max(total, 1)

        print(f"  ✓ Aggregation done! Global Train Loss: {avg_loss:.4f}, Total: {total}")
        print(f"  ✓ Global state saved: {out_path}")
        if anomalies:
            print(f"  ⚠ (이상 노드 {anomalies} 제외됨)")
        return avg_loss, out_path

    # -----------------------
    # 고정 검증셋 평가
    # -----------------------
    def _evaluate_global(self, round_num: int, model_path: str):
        """
        이번 라운드 글로벌 모델을 고정 검증셋으로 잰다.

        라운드마다 같은 데이터·같은 자로 재야 "갈수록 좋아진다"를 말할 수
        있다. 그동안의 val_auroc 는 그 라운드에 받은 윈도우로 계산해서
        라운드 간 비교가 안 됐고, 라이브 스트림에 라벨이 없어 늘 0.5 였다.

        결과는 cnt-fl-inference 에 CIN 으로 남긴다. 대시보드가 읽어 띄운다.
        평가가 실패해도 FL 은 계속한다.
        """
        try:
            m = valset.evaluate(model_path)
        except Exception as e:
            print(f"  ⚠ 검증셋 평가 실패(계속 진행): {e}")
            return

        if not m:
            return

        print(f"  [EVAL] Round {round_num}  {valset.summary_line(m)}")

        payload = {
            "type": "global-eval",
            "round": round_num,
            "model": model_path,
            "timestamp": time.time(),
        }
        payload.update(m)

        try:
            om2m.create_content_instance(
                self.fl_inference_path, payload,
                labels=[f"round_{round_num}", "global-eval"],
            )
        except Exception as e:
            print(f"  ⚠ 평가 결과 업로드 실패(계속 진행): {e}")

    # -----------------------
    # Main loop
    # -----------------------
    def run(self):
        print("\n=== IN-AE Coordinator start ===")
        print(f"  fl-control:   {self.fl_control_path}")
        print(f"  global-model: {self.global_model_path}")
        print(f"  drop-box:     {self.dropbox_root}")

        self.start_server()
        self._prepare_dropbox()
        self._subscribe_dropbox()

        # ACP 검증
        if getattr(config, "VERIFY_ACP", True):
            self._verify_acp()
        else:
            print("\n=== ACP 접근 제어 검증 — 건너뜀 (FL_VERIFY_ACP=0) ===")
            print("  이 진단은 사업장 MN-CSE 에 직접 접속한다.")
            print("  IN-AE 가 중앙에서 돌면 그쪽이 방화벽 뒤라 닿지 않는다.")
            print("  연합학습 동작과는 무관하다.\n")

        # Round 1 command를 놓치지 않도록 MN-AE 구독이 모두 준비될 때까지 대기
        if not self._wait_for_mn_command_subscriptions():
            print("  ✗ FL start aborted: MN-AE subscriptions are not ready")
            return

        self._ensure_initial_global_model()
        if not self._publish_global_model(0):
            print("  ✗ FL start aborted: failed to publish Round 0 model")
            return

        if not self._publish_job_state("FL_READY", 0):
            print("  ✗ FL start aborted: failed to publish FL_READY")
            return

        print("  ✓ Initial global model ready (Round 0)")

        for r in range(1, self.max_rounds + 1):
            self.current_round = r

            print("\n" + "=" * 60)
            print(f"GLOBAL ROUND {r}/{self.max_rounds}")
            print("=" * 60)

            self.collection_event.clear()
            with self.collection_lock:
                self.collected_results = {}
                self.cold_start_results = {}

            # 현재 global model URI를 포함한 round command CIN 생성
            if not self._publish_job_state("FL_TRAINING", r):
                print(f"  ✗ failed to start round {r}; abort FL")
                return

            print(
                f"  ⏳ Wait local-update NOTIFY "
                f"(timeout: {self.collection_timeout_sec:.0f}s)..."
            )

            all_notified = self.collection_event.wait(
                timeout=self.collection_timeout_sec
            )

            with self.collection_lock:
                results = dict(self.collected_results)

            if all_notified:
                print(
                    "  ✓ All results received by NOTIFY "
                    "-> aggregate now"
                )
            else:
                missing_nodes = [
                    node
                    for node in self.training_nodes
                    if node not in results
                ]
                print(
                    f"  ⚠ NOTIFY timeout: collected "
                    f"{len(results)}/{self.expected_nodes}; "
                    f"single fallback RETRIEVE for {missing_nodes}"
                )
                results.update(
                    self._collect_results_latest(
                        r,
                        nodes=missing_nodes
                    )
                )

            if len(results) < self.expected_nodes:
                print(
                    f"  ✗ Not enough results "
                    f"({len(results)}/{self.expected_nodes}); "
                    "abort FL"
                )
                return

            if not self._publish_job_state("FL_AGGREGATING", r):
                print(f"  ✗ failed to publish aggregation state for round {r}")
                return

            _, final_path = self._aggregate(results, r)
            if not final_path or not os.path.exists(final_path):
                print(
                    f"  ✗ Aggregation failed in round {r}; "
                    "do not publish global model"
                )
                return

            if not self._publish_global_model(r):
                print(
                    f"  ✗ failed to publish global model for round {r}; "
                    "abort FL"
                )
                return

            self._evaluate_global(r, final_path)

        print("\nFederated learning completed!")
        self._publish_job_state("FL_COMPLETED", self.max_rounds)


if __name__ == "__main__":
    coordinator = INAECoordinator(max_rounds=config.GLOBAL_ROUNDS)
    coordinator.run()