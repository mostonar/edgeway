"""
표준 oneM2M FL 아키텍처 - MN-AE Trainer (Raw Signal AE 버전)
AMR 주행 텔레메트리(38채널 CVAE)용

변경 사항 (이전 통계피처 버전 대비):
- AEEdgeNode(조건부 1D CNN VAE)
- 입력: raw 시계열 신호 (N, seq_len)
- 손실: MSE reconstruction loss (정상만)
- val_acc → val_auroc (재구성 오차 기반 AUROC)
"""
import sys
import calendar

import os
import time
import json
import pickle
import threading
import hashlib

import numpy as np
import torch
from flask import Flask, request, make_response

import amr_ingest
import config
import model_codec
import onem2m_utils as om2m
import onem2m_pch
import verdict
import mec_services
import mec_iot
import cse_failover
from edge_node import AEEdgeNode

# "이번 라운드는 학습할 수 없다. 다만 노드를 죽일 일은 아니다"를 나타내는 신호.
# 통신 실패(None)와 구분해야 한다. 전자는 다음 라운드에 회복될 수 있지만
# 후자는 그렇지 않다.
SKIP_ROUND = object()

# 학습 전 판정에서 임계를 넘은 창을 그 라운드 학습에서 뺄지.
EXCLUDE_FLAGGED = os.getenv("FL_EXCLUDE_FLAGGED", "1") not in ("0", "false", "False")
# 정상 기준 창을 몇 라운드까지 모으고 고정할지. 정규화 통계와 맞춘다.
REF_FREEZE_ROUND = int(os.getenv(
    "FL_REF_FREEZE_ROUND", os.getenv("AMR_NORM_FREEZE_ROUND", "5")))


class MNAETrainer:
    def __init__(self, node_id: int, notification_port: int, inject_anomaly: bool = False):
        self.node_id          = node_id
        self.node_name        = f"mn{node_id + 1}"

        # 노드 역할
        #   관측 전용 : 학습 안 하고 추론만 (COLD_START_NODES)
        #   로컬 전용 : 로컬 학습은 하되 FedAvg에 가중치를 올리지 않음
        #               (LOCAL_ONLY_NODES — 사업장 C가 여기 해당)
        #   그 외     : 학습 + FedAvg 기여
        self.is_cold_start = self.node_name in getattr(
            config, "COLD_START_NODES", []
        )
        self.is_local_only = self.node_name in getattr(
            config, "LOCAL_ONLY_NODES", []
        )
        self.notification_port = notification_port
        self.inject_anomaly   = inject_anomaly

        # 이 사업장의 정규화 통계. 적재 상태가 고루 모일 때까지 누적하다가
        # 고정한다(amr_schema.NormStats). 첫 수집분 고정은 빈 차 주행만
        # 담겨 적재 구간에서 |z| 가 수십~수백으로 튀었다.
        import amr_schema as _sch
        self.norm = _sch.NormStats()

        # 판정기. 직전 라운드에 학습·보정을 마친 노드를 그대로 둔다.
        # 새 창은 이것으로 '학습하기 전에' 채점한다(_prescore_windows).
        self._detector = None
        # 기준·임계값을 잡을 정상 기준 창(원신호). 워밍업 라운드 동안 모으고
        # REF_FREEZE_ROUND 에 고정한다. 고정한 뒤로는 이상이 들어와도 기준이
        # 따라 올라가지 않는다.
        self._calib_X: list = []
        self._calib_C: list = []
        self._calib_frozen = False
        self._calib_frozen_round = None

        self.ae_name = config.training_ae_name(self.node_name)
        self.ae_id = config.training_ae_id(self.node_name)

        # 켜져 있으면 이 사업장의 AE-ID 로 CSE에 요청한다. 그래야 ACP가
        # 실제로 작동한다. 꺼져 있으면 기존처럼 CAdmin 으로 나간다.
        if config.USE_AE_IDENTITY:
            om2m.set_originator(self.ae_id)

        # 이 사업장의 MN-CSE에 바인딩한다. 사업장마다 MEP가 다르다.
        # 주소는 MEP 의 MEC033 에 등록된 IoT 플랫폼에서 받고, 못 받으면
        # 기존 규칙(config.mn_endpoint)으로 간다.
        mn_base, mn_cse = mec_iot.resolve(self.node_name)
        om2m.set_endpoint(mn_base, mn_cse)
        om2m.set_in_endpoint(*config.in_endpoint())
        self.mn_cse_name = mn_cse

        # MN-CSE 가 학습 도중 사라지면 같은 MEP 의 다른 CSE 로 옮긴다
        # (cse_failover.py). 감시는 뒤에서, 전환은 _maybe_cse_handover 에서.
        self.cse_watch = cse_failover.CseWatch(self.node_name, mn_base, mn_cse)
        self._next_handover_try = 0.0
        self.cse_handovers = 0

        # 사업장 CSE 안의 리소스.
        #
        # 텔레메트리는 MN-AE가 아니라 로봇 AE 아래에 쌓인다. Isaac Sim이
        # 로봇 AE를 등록하고 주행하면서 윈도우를 CREATE한다.
        #     <MN-CSE>/siteA_AMR_1/telemetry/<cin>
        # 사업장 하나가 FL 노드 하나이므로 로봇 대수만큼의 컨테이너를
        # 여기서 모아 "한 공장의 데이터"로 합친다.
        # Isaac Sim 이 라벨을 달아 올리므로 디스커버리로 찾는다. 기동
        # 시점에는 아직 아무것도 없을 수 있어서, 여기서는 config 로
        # 한 번 채워 두고 _refresh_telemetry() 가 실제 목록으로 바꾼다.
        self.robot_names = config.robot_names(self.node_name)
        self.telemetry_discovered = False
        self._telemetry_added = []
        # 로봇별로 어디까지 읽었는지. 라운드마다 그 뒤만 읽는다.
        # 비워 두면 첫 라운드는 최근 분량을 받는다.
        self.telemetry_cursor = {}

        # 이번 라운드에 모은 윈도우. telemetry 컨테이너의 mni 때문에
        # 한 번에 다 긁을 수 없어서, 받을 때마다 여기에 쌓는다.
        self._round_buffer = None
        self._round_buffer_round = None

        # 이번 라운드가 쓰고 남길 몫. 라운드가 실제로 성립한 뒤에
        # _round_buffer 로 옮긴다.
        self._pending_leftover = None
        self.telemetry_paths = [
            f"{mn_cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
            for rb in self.robot_names
        ]
        self._refresh_telemetry(quiet=True)
        self.local_model_cnt   = "cnt-local-model"
        self.local_model_path  = f"{mn_cse}/{self.ae_name}/{self.local_model_cnt}"

        # 중앙 CSE 안의 리소스. @in/ 접두사가 중앙으로 보낸다.
        self.fl_control_path   = om2m.in_path(f"{config.IN_AE_NAME}/cnt-fl-control")
        self.global_model_path = om2m.in_path(f"{config.IN_AE_NAME}/cnt-global-model")

        # Drop-box (중앙)
        self.dropbox_cnt_path = om2m.in_path(
            f"{config.IN_AE_NAME}/cnt-local-updates/cnt-{self.node_name}"
        )

        # NOTIFY 기반 round 시작
        # 텔레메트리 구독으로 받은 알림을 구분하기 위한 구독 ri 집합.
        # 라운드 명령 알림과 섞이면 안 된다.
        self.telemetry_subs = set()

        # 로봇이 새 윈도우를 올렸다는 신호. 수집 루프가 이걸 기다린다.
        # 알림이 깨우므로 2초마다 CSE를 찔러볼 필요가 없다.
        self.data_event = threading.Event()
        self.data_notifications = 0

        # 알림 경로는 둘이고 조건이 서로 다르다. 한 변수로 묶으면
        # 한쪽 설정이 다른 쪽을 망가뜨린다.
        #   local  : 사업장 MN-CSE. 로봇 텔레메트리 알림.
        #   remote : 클라우드 IN-CSE. 라운드 명령 알림.
        # 이 프로세스가 뜬 시각. 이보다 먼저 발행된 라운드 명령은
        # 지난 실행의 잔재이므로 따르지 않는다.
        self.started_at = time.time()

        self.local_notify_target = None
        self.remote_notify_target = None
        self.local_pch = None
        self.pch_receiver = None

        # 판정(분석 앱). 결과는 사업장 밖으로 안 나가므로 MN-CSE 에 남긴다.
        # 쓰는 신원은 MN-AE 가 아니라 분석 앱이다. 설계서에서 둘은 다른 앱이고
        # ACP 도 역할별로 나뉜다.
        self.analysis_ae_id = config.analysis_ae_id(self.node_name)
        self.verdict_cnt_path = (
            f"{self.mn_cse_name}/"
            f"{config.analysis_ae_name(self.node_name)}/"
            f"{config.VERDICT_CNT}"
        )
        # 이번 라운드에 읽은 윈도우. 판정은 학습이 끝난 뒤 이걸로 낸다.
        self.last_windows = None

        # MEC 서비스(MEC011 등록·발견, MEC028 링크, MEC013 위치).
        # 링크·위치 기록은 이 사업장 학습 앱의 cnt-net-status 에 남긴다.
        self.mec = mec_services.SiteMec(
            self.node_name,
            endpoint=mec_services.mec_app.endpoint_for(self.notification_port),
        )
        self.net_status_path = (
            f"{self.mn_cse_name}/{self.ae_name}/cnt-net-status"
        )
        # 윈도우별 이상 점수. 사업장 안(MN-CSE)에만 둔다. 대시보드가 여기서 읽는다.
        self.window_scores_path = (
            f"{self.mn_cse_name}/{self.ae_name}/{config.WINDOW_SCORES_CNT}"
        )

        self.job_event = threading.Event()
        self.job_lock = threading.Lock()
        self.latest_job_command = None
        self.last_command_source = None

        # NOTIFY가 누락됐을 때만 /la를 한 번 확인하기 위한 timeout
        self.command_wait_timeout_sec = float(
            os.getenv("FL_COMMAND_NOTIFY_TIMEOUT", "30")
        )

        self.edge_node = None

        self.app = Flask(self.ae_name)
        self._setup_flask()

        self.edge_cache_dir = f"/tmp/fl_models/cache/{self.node_name}"
        os.makedirs(self.edge_cache_dir, exist_ok=True)

        self.local_cache_meta_file = os.path.join(
            self.edge_cache_dir, "cache_meta.json"
        )

        self.round_start_stagger_sec = float(self.node_id) * 2.0
        self.upload_stagger_sec      = float(self.node_id) * 3.0

    # ---------------------------
    # Hash
    # ---------------------------
    @staticmethod
    def _sha256_file(path: str) -> str:
        h = hashlib.sha256()
        with open(path, "rb") as f:
            for chunk in iter(lambda: f.read(8192), b""):
                h.update(chunk)
        return h.hexdigest()

    # ---------------------------
    # Flask / Notify
    # ---------------------------
    def _setup_flask(self):
        @self.app.route("/notify", methods=["POST"])
        def handle_notification():
            try:
                body = request.get_json(silent=True) or {}
                sgn = body.get("m2m:sgn", {})

                # Subscription verification
                if sgn.get("vrq") is True:
                    print(
                        f"  [VERIFY] {self.node_name} "
                        f"subscription verification OK"
                    )

                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                nev = sgn.get("nev", {}) or {}
                net = nev.get("net", 0)
                net_values = net if isinstance(net, list) else [net]

                try:
                    is_cin_created = 3 in {
                        int(value) for value in net_values
                    }
                except (TypeError, ValueError):
                    is_cin_created = False

                if not is_cin_created:
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                # 텔레메트리 알림이면 "새 데이터 있음"만 세우고 끝낸다.
                #
                # 알림마다 CSE를 읽으러 가면 윈도우 수만큼 왕복이 생긴다.
                # 여기서는 표시만 하고, 실제 읽기는 라운드 수집 때 한 번에
                # 한다. 알림은 "언제 읽을지"를, cra 증분 조회는 "무엇을
                # 읽을지"를 정한다.
                sur = str(sgn.get("sur", "")).strip("/").split("/")[-1]
                if sur in self.telemetry_subs:
                    self.data_notifications += 1
                    self.data_event.set()
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                # 정상 경로:
                # round command CIN의 con을 NOTIFY body에서 읽음
                command = self._parse_cin_con_from_notification(sgn)

                # CSE가 rep를 보내지 않은 경우에만
                # NOTIFY를 계기로 최신 CIN을 한 번 조회
                #
                # rep 가 없으면 그냥 무시한다. 전에는 /la 를 대신 읽었는데
                # 그게 묵은 명령으로 라운드를 시작시켰다.
                #
                # cnt-fl-control 에는 지난 실행의 CIN 이 남아 있다. 기동 직후
                # rep 없는 알림이 하나만 와도 /la 가 지난 실행의
                # FL_TRAINING round=1 을 돌려주고, MN-AE 는 그것을 이번
                # 라운드 명령으로 알고 학습을 시작한다. 실제로 그렇게 됐다.
                # IN-AE 가 아직 라운드를 열기도 전에 학습하고 업로드했고,
                # 그 업로드는 IN-AE 가 드롭박스를 비우면서 사라졌다.
                #
                # 알림을 놓친 경우는 _wait_for_round_command 의 타임아웃
                # 폴백이 이미 처리한다. 그쪽은 라운드 번호를 대조하므로 안전하다.
                if command is None:
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                if command:
                    state = str(command.get("jobState", ""))

                    try:
                        current_round = int(
                            command.get("currentRound", 0)
                        )
                    except (TypeError, ValueError):
                        current_round = 0

                    command_type = (
                        str(command.get("type", ""))
                        .lower()
                        .replace("_", "-")
                    )

                    if command_type and command_type != "fl-command":
                        print(
                            f"  ⚠ {self.node_name}: "
                            f"ignore command type={command_type!r}"
                        )

                    elif self._is_stale_command(command):
                        pass

                    elif state in ("FL_TRAINING", "FL_COMPLETED"):
                        with self.job_lock:
                            self.latest_job_command = command
                            self.last_command_source = "NOTIFY"

                        cin_ri = (
                            ((sgn.get("nev") or {}).get("rep") or {})
                            .get("m2m:cin", {})
                            .get("ri", "?")
                        )
                        print(
                            f"  [NOTIFY] {self.node_name} "
                            f"command received: "
                            f"{state}, round={current_round}, cin={cin_ri}"
                        )

                        self.job_event.set()

                resp = make_response("", 200)
                resp.headers["X-M2M-RSC"] = "2000"
                return resp

            except Exception as e:
                print(f"  ✗ notify error: {e}")
                return make_response("", 500)

    def _parse_cin_con_from_notification(self, sgn: dict):
        nev = sgn.get("nev", {}) or {}
        rep = nev.get("rep", {}) or {}

        cin = rep.get("m2m:cin")
        if not isinstance(cin, dict):
            return None

        con = cin.get("con")
        if con is None:
            return None

        try:
            data = json.loads(con) if isinstance(con, str) else con
        except (TypeError, json.JSONDecodeError):
            return None

        return data if isinstance(data, dict) else None

    def start_server(self):
        def run():
            self.app.run(
                host="0.0.0.0", port=self.notification_port,
                threaded=True, use_reloader=False
            )
        th = threading.Thread(target=run, daemon=True)
        th.start()
        time.sleep(1)
        print(f"  ✓ {self.node_name} Notification server on port {self.notification_port}")
        self._setup_notify_path()

    def _setup_notify_path(self):
        """
        알림을 어느 경로로 받을지 정한다. 경로는 둘이고 조건이 다르다.

          로컬  사업장 MN-CSE -> 우리.  로봇 텔레메트리 알림이 이 길로 온다.
                같은 사업장 안이라 CSE 가 우리를 직접 부를 수 있다.
                AE 에 poa 를 박고 구독의 nu 에 URL 을 준다.

          원격  클라우드 IN-CSE -> 우리.  라운드 명령(cnt-fl-control)이 이 길.
                캠퍼스 방화벽이 인바운드를 버리므로 CSE 가 우리를 못 부른다.
                <pollingChannel> 로 받고 구독의 nu 에 AE-ID 를 준다.

        전에는 플래그 하나가 둘 다 정했다. 그래서 원격 때문에 pch 를 켜면
        poa 를 박는 코드가 direct 분기 안에만 있어서 같이 건너뛰었고,
        방화벽과 아무 상관없는 사업장 안의 텔레메트리 구독까지 죽었다.
        이제 로컬 설정은 원격이 무엇이든 그대로 돈다.
        """
        port = self.notification_port
        poa = f"http://{config.NOTIFY_HOST}:{port}"
        local_notify = f"http://127.0.0.1:{port}/notify"

        self._setup_local_notify()

        # ── 원격: 클라우드 IN-CSE ────────────────────────
        if config.remote_notify_mode() == "pch":
            self.pch_receiver = onem2m_pch.setup(
                om2m.in_path(self.ae_name), local_notify,
                label=f"{self.ae_name} / 원격")
            self.remote_notify_target = (
                self.pch_receiver.origin if self.pch_receiver else None)
            if not self.pch_receiver:
                print(f"    ⚠ {self.ae_name} 은 IN-CSE 로부터 알림을 받을 수 "
                      f"없다. 라운드 명령 구독이 실패할 것이다.")
        else:
            self.remote_notify_target = f"{poa}/notify"

    def _setup_local_notify(self):
        """
        로컬(사업장 MN-CSE -> 우리) 알림 경로. CSE 핸드오버 뒤에도 다시 부른다.
        """
        port = self.notification_port
        poa = f"http://{config.NOTIFY_HOST}:{port}"
        local_notify = f"http://127.0.0.1:{port}/notify"

        # ── 로컬: 사업장 MN-CSE ──────────────────────────
        local_ae = f"{self.mn_cse_name}/{self.ae_name}"
        if config.local_notify_mode() == "pch":
            self.local_pch = onem2m_pch.setup(
                local_ae, local_notify, label=f"{self.ae_name} / 로컬")
            self.local_notify_target = (
                self.local_pch.origin if self.local_pch else None)
        else:
            # rr=true 만 켜고 poa 를 비워 두면 CSE 가 알림 대상을 검증하지
            # 못해 구독이 거부된다("Verification request failed for: ...").
            # MN-CSE 는 같은 머신이지만 컨테이너 안이라 127.0.0.1 로는
            # 우리에게 못 온다. 호스트 주소를 알려줘야 한다.
            om2m.set_point_of_access(local_ae, [poa], reachable=True)
            self.local_notify_target = f"{poa}/notify"

    def _is_stale_command(self, command: dict) -> bool:
        """
        이 프로세스가 뜨기 전에 발행된 명령인가.

        cnt-fl-control 은 실행 사이에 비워지지 않는다. MN-AE 가 구독하고
        있어서 컨테이너를 지웠다 다시 만들 수 없기 때문이다. 그래서 지난
        실행의 FL_TRAINING round=1 이 그대로 남아 있고, 이번 실행의 것과
        내용이 똑같아 보인다. 발행 시각으로 가른다.
        """
        try:
            ts = float(command.get("timestamp", 0))
        except (TypeError, ValueError):
            return False
        if ts <= 0 or ts >= self.started_at:
            return False
        print(f"  ⓘ {self.node_name}: 지난 실행의 명령 무시 "
              f"(발행 {time.strftime('%H:%M:%S', time.localtime(ts))}, "
              f"기동 {time.strftime('%H:%M:%S', time.localtime(self.started_at))})")
        return True

    def _is_expected_command(self, command: dict | None, round_num: int) -> bool:
        if not isinstance(command, dict):
            return False

        state = str(command.get("jobState", ""))

        try:
            current_round = int(command.get("currentRound", command.get("round", 0)))
        except (TypeError, ValueError):
            current_round = 0

        # 핵심 수정:
        # 새 실행에서 MN-AE가 Round 1을 기다리는 중인데,
        # cnt-fl-control/la에 이전 실행의 FL_COMPLETED가 남아 있으면
        # 그걸 새 실행 완료로 착각하고 종료하는 문제가 생김.
        #
        # 따라서 Round 1 대기 중에는 FL_COMPLETED를 expected command로 인정하지 않음.
        # Round 1은 반드시 FL_TRAINING/currentRound=1로 시작해야 함.
        if state == "FL_COMPLETED":
            if round_num == 1:
                print(
                    f"  ⚠ {self.node_name}: ignore stale FL_COMPLETED "
                    f"while waiting Round 1"
                )
                return False

            # Round 2 이후에는 실제 완료 신호일 수 있으므로 허용.
            # 단, round 정보가 이상하면 무시함.
            return current_round >= round_num

        return (
            state == "FL_TRAINING"
            and current_round == round_num
        )

    def _wait_for_round_command(self, round_num: int):
        """
        NOTIFY를 주 경로로 사용함.
        최초 timeout 때 cnt-fl-control/la를 한 번만 조회하고,
        그 이후에는 다시 polling하지 않고 NOTIFY만 기다림.
        """
        fallback_checked = False

        while True:
            with self.job_lock:
                command = self.latest_job_command
                source = self.last_command_source

            if self._is_expected_command(command, round_num):
                return command, source or "NOTIFY"

            wait_timeout = (
                None
                if fallback_checked
                else self.command_wait_timeout_sec
            )

            notified = self.job_event.wait(timeout=wait_timeout)
            self.job_event.clear()

            if notified:
                continue

            # 최초 timeout에서만 단발성 fallback RETRIEVE 수행
            fallback_checked = True
            print(
                f"  ⚠ {self.node_name}: command NOTIFY timeout; "
                "check cnt-fl-control/la once"
            )

            fallback = self._get_current_job_state()
            # /la 에는 지난 실행의 명령이 남아 있을 수 있다. 알림 경로는
            # 이미 발행 시각으로 거르는데 여기서는 안 걸러서, 이전 실행이
            # 1라운드에서 멈췄을 때 남긴 FL_TRAINING round=1 을 이번 실행의
            # 명령으로 받아 IN-AE 가 뜨기도 전에 학습·업로드를 해 버렸다.
            # IN-AE 는 아직 0라운드라 그 업로드를 round mismatch 로 버리고,
            # MN 은 이미 2라운드를 기다리므로 1라운드에서 영영 멈춘다.
            if isinstance(fallback, dict) and self._is_stale_command(fallback):
                fallback = None
            if self._is_expected_command(fallback, round_num):
                with self.job_lock:
                    self.latest_job_command = fallback
                    self.last_command_source = "fallback RETRIEVE"
                return fallback, "fallback RETRIEVE"

            print(
                f"  ⚠ {self.node_name}: no matching command in fallback; "
                "continue waiting for NOTIFY"
            )

    # ---------------------------
    # oneM2M ops
    # ---------------------------
    def _subscribe_to_telemetry(self, only=None) -> int:
        """
        이 사업장 로봇들의 telemetry 컨테이너에 구독을 건다.

        설계 문서 9.8.5의 sub-sensor-data-X 에 해당한다. 로봇 AE(ADN-AE)는
        Isaac Sim 이 만들고, 우리는 그 아래 컨테이너에 구독만 건다.

        전 구간이 사업장 안이다. 로봇 -> MN-CSE -> MN-AE 모두 같은 네트워크라
        캠퍼스 방화벽과 무관하게 직접 알림이 닿는다. 실제로 CSE 파드가
        호스트의 5001 포트까지 도달하는 것을 확인했다.

        알림은 "새 데이터가 들어왔다"는 신호로만 쓴다. 내용은 라운드 수집
        때 cra 증분 조회로 한 번에 읽는다. 그래야 윈도우 하나당 왕복이
        생기지 않는다.

        반환: 성공한 구독 수
        """
        if not getattr(config, "FL_SENSOR_FROM_CSE", True):
            return 0

        # 텔레메트리는 사업장 안에서 오는 알림이다. 원격 경로와 무관하다.
        notify_url = self.local_notify_target
        if not notify_url:
            print(f"    ⚠ 로컬 알림 경로가 없다 — 구독을 걸지 않는다.")
            return 0

        ok = 0
        print(f"\n  [{self.node_name}] 로봇 텔레메트리 구독")
        for robot, tel_path in zip(self.robot_names, self.telemetry_paths):
            # 라운드 중 재탐색에서는 새로 생긴 로봇만 건다. 전부 지우고
            # 다시 걸면 그 사이 알림을 놓치고, 수집 중인 라운드가 멈칫한다.
            if only is not None and robot not in only:
                continue
            if not om2m.resource_exists(tel_path):
                print(f"    ⚠ {robot}: telemetry 없음 — 건너뜀")
                continue

            sub_name = f"sub-sensor-data-{self.node_name}"
            om2m.delete_subscription(tel_path, sub_name)
            time.sleep(0.1)

            res = om2m.create_subscription(
                parent_path=tel_path,
                subscription_name=sub_name,
                notification_uri=notify_url,
                event_types=[3],
                use_nct=1,
            )
            if not res:
                print(f"    ✗ {robot}: 구독 실패")
                continue

            ri = (res.get("m2m:sub") or {}).get("ri")
            if ri:
                self.telemetry_subs.add(ri)
            ok += 1
            print(f"    ✓ {robot}")

        if ok == 0:
            print(f"    ⚠ 구독이 하나도 안 걸렸다. 수집은 주기 조회로 돈다.")
        return ok

    def _subscribe_to_fl_control(self):
        """라운드 명령 구독. 대상은 클라우드 IN-CSE 라 원격 경로를 쓴다."""
        if not self.remote_notify_target:
            print(f"  ✗ {self.node_name}: IN-CSE 로부터 알림을 받을 경로가 "
                  f"없어 구독을 만들지 않는다.")
            return None

        sub_name = f"sub_{self.node_name}"
        om2m.delete_subscription(self.fl_control_path, sub_name)
        time.sleep(0.2)
        res = om2m.create_subscription(
            parent_path=self.fl_control_path,
            subscription_name=sub_name,
            notification_uri=self.remote_notify_target,
            event_types=[3],
            use_nct=None,
        )
        if res:
            print(f"  ✓ {self.node_name} subscribed to cnt-fl-control (net=3)")
        return res

    def _ensure_local_model_container(self):
        # config.CSE_NAME 은 import 시점의 mn1 CSE 라 mn2/mn3 가 쓰면
        # 남의 CSE 에 만들려 해서 404 가 난다. 자기 사업장 CSE 를 쓴다.
        parent = f"{self.mn_cse_name}/{self.ae_name}"
        res = om2m.create_container(
            parent_path=parent, container_name=self.local_model_cnt, mni=20
        )
        if res:
            print(f"  ✓ local cache container ready: {self.local_model_path}")

    def _get_current_job_state(self):
        cin = om2m.get_latest_content_instance(self.fl_control_path)
        if cin and "m2m:cin" in cin:
            con = cin["m2m:cin"]["con"]
            return json.loads(con) if isinstance(con, str) else con
        return None

    # ---------------------------
    # Sensor data load (AE 버전)
    # ---------------------------
    def _refresh_telemetry(self, quiet: bool = False) -> bool:
        """
        이 사업장의 telemetry 컨테이너를 라벨 디스커버리로 다시 찾는다.

        왜 경로를 박지 않나
          <MN-CSE>/{site}_AMR_{i}/telemetry 를 조립하면 로봇 대수와 이름
          규칙이 코드에 박힌다. 로봇이 늘거나 이름이 바뀌면 코드를 고쳐야
          하고, 늦게 올라온 로봇은 영영 안 보인다. Isaac Sim 이 컨테이너에
          라벨을 달아 주므로 (?fu=1&ty=3&lbl=site:siteC&lbl=type:telemetry)
          그때그때 찾는다.

        찾은 게 없으면 기존 목록을 그대로 둔다. 디스커버리가 잠깐 실패했을
        때 멀쩡한 경로까지 날리면 그 라운드를 통째로 잃는다.
        반환: 목록이 바뀌었으면 True.
        """
        if not getattr(config, "USE_DISCOVERY", True):
            return False

        site = config.SITE_OF_NODE.get(self.node_name, self.node_name)
        labels = [config.site_telemetry_label(self.node_name)] \
            + config.telemetry_labels(self.node_name)
        try:
            found = om2m.discover_telemetry(
                self.mn_cse_name, site,
                cnt_name=config.ROBOT_TELEMETRY_CNT,
                type_label=config.LABEL_TYPE_TELEMETRY,
                combined_label=config.site_telemetry_label(self.node_name),
            )
        except Exception as e:
            if not quiet:
                print(f"    ⚠ telemetry 디스커버리 실패(기존 경로 유지): {e}")
            return False

        if not found:
            if not quiet and not self.telemetry_discovered:
                codes = ", ".join(str(c) for c in om2m.LAST_DISCOVERY_STATUS)
                print(f"    ⓘ 라벨 {labels} 로 찾은 telemetry 가 없다 "
                      f"(응답 {codes or '없음'}). 경로 조립으로 진행한다")
                if any(c == 403 for c in om2m.LAST_DISCOVERY_STATUS):
                    print(f"      403 이면 {self.ae_id} 에 CSEBase DISCOVERY "
                          f"권한이 없는 것이다")
            return False

        robots = sorted(found)
        paths = [found[r] for r in robots]

        # 경로가 실제로 열리는지 하나만 확인한다. 디스커버리 응답 형태를
        # 잘못 해석했는데 그대로 바꿔치우면 그 라운드를 통째로 잃는다.
        if not om2m.resource_exists(paths[0]):
            if not quiet:
                print(f"    ⚠ 디스커버리 경로가 안 열린다 ({paths[0]}) "
                      f"-> 기존 경로 유지")
            return False
        changed = (robots != self.robot_names or paths != self.telemetry_paths)
        # 새로 생겼거나 경로가 바뀐 로봇. 구독은 이것만 새로 건다.
        before = dict(zip(self.robot_names, self.telemetry_paths))
        self._telemetry_added = [r for r in robots
                                 if before.get(r) != found[r]]
        if changed and not quiet:
            added = [r for r in robots if r not in self.robot_names]
            print(f"    ✓ 디스커버리: {len(robots)}대 "
                  f"({', '.join(robots)})"
                  + (f"  새로 발견 {', '.join(added)}" if added else ""))
        self.robot_names = robots
        self.telemetry_paths = paths
        self.telemetry_discovered = True
        return changed

    def _wait_for_robots(self, timeout: float = None) -> bool:
        """
        Isaac Sim이 이 사업장의 로봇 AE와 telemetry를 올릴 때까지 기다린다.

        샌드박스가 뜬 직후에는 MN-CSE만 있고 로봇 AE는 없다. Isaac이 등록한다.
        그래서 FL을 바로 시작하면 읽을 것이 없다. 라운드 안에서 기다리는 것과
        별개로, 기동 시점에 한 번 확실히 기다린다.

        기대 대수를 다 못 채워도 최소 대수만 준비되면 진행한다.
        로봇 한 대가 빠졌다고 사업장 전체를 멈출 이유는 없고, 뒤늦게 올라온
        로봇은 다음 라운드의 수집에서 자연히 포함된다.
        """
        if timeout is None:
            timeout = float(os.getenv("FL_ROBOT_WAIT_SEC", "300"))
        min_robots = int(os.getenv("FL_MIN_ROBOTS_PER_SITE", "1"))
        poll = float(os.getenv("FL_ROBOT_POLL_SEC", "5"))

        site = config.SITE_OF_NODE.get(self.node_name, self.node_name)
        print(f"\n  [{self.node_name}/{site}] Isaac Sim 대기 "
              f"(최대 {timeout:.0f}s, 최소 {min_robots}대)")

        deadline = time.time() + timeout
        next_log = 0.0

        while True:
            # 늦게 올라온 로봇도 다음 조회에서 잡히도록 매번 다시 찾는다.
            self._refresh_telemetry()
            ready = [
                rb for rb, path in zip(self.robot_names, self.telemetry_paths)
                if om2m.resource_exists(path)
            ]
            now = time.time()

            if len(ready) >= min_robots:
                print(f"    ✓ {len(ready)}/{len(self.robot_names)}대 준비됨 "
                      f"({', '.join(ready)})")
                missing = [r for r in self.robot_names if r not in ready]
                if missing:
                    print(f"    ⓘ 아직 없는 로봇: {', '.join(missing)} "
                          f"(나중에 올라오면 그 라운드부터 포함된다)")
                return True

            if now >= deadline:
                print(f"    ✗ {timeout:.0f}s 동안 로봇이 올라오지 않았다")
                print(f"      기대: {', '.join(self.robot_names)}")
                print(f"      확인: Isaac Sim이 {self.mn_cse_name} 에 "
                      f"등록 중인지")
                return False

            if now >= next_log:
                print(f"    ⏳ {len(ready)}/{min_robots}대 "
                      f"(남은 {deadline - now:.0f}s)")
                next_log = now + 10.0

            time.sleep(poll)

    # ---------------------------
    # MN-CSE 핸드오버 (MEC033)
    # ---------------------------
    def _cse_block(self) -> dict:
        """
        드롭박스 업로드에 싣는 '지금 붙어 있는 MN-CSE'. 코디네이터와 대시보드는
        이것으로 사업장이 지금 어느 CSE 에 붙어 있는지 안다. 대시보드는 핸드오버
        뒤에도 이 주소로 그 사업장의 윈도우 점수를 읽으러 간다.
        """
        return {"mn_cse": self.mn_cse_name,
                "mn_base": om2m.base_url(),
                "cse_handovers": int(self.cse_handovers)}

    def _maybe_cse_handover(self, round_num: int = None) -> bool:
        """
        MN-CSE 가 FL_CSE_FAILOVER_SEC 이상 응답이 없으면 같은 MEP 의 다른
        CSE 로 옮긴다. 옮겼으면 True.

        안전한 지점(라운드 시작, 수집 루프, 판정 직전)에서만 부른다.
        학습 중에는 MN-CSE 를 쓰지 않으므로 그 사이에 죽어도 다음 지점에서
        옮기면 된다.

        옮기면서 하는 일
          1) MEC033 에서 지금 CSE 가 아닌, 응답하는 CSE 를 고른다
          2) 이 프로세스의 MN-CSE 바인딩과 그 CSE 기준 경로를 전부 바꾼다
          3) 로봇(Isaac Sim)이 새 CSE 에 올라오길 기다린다
          4) 새 CSE 에 우리 리소스를 만든다
             (TRAINING-AE, ANALYSIS-AE, ACP 2개, cnt-local-model,
              cnt-net-status, cnt-verdict — setup_resources_standard 와 같다)
             로봇을 먼저 기다리는 이유: acp-verdict 에 로봇 AE-ID 를 넣어야
             로봇이 새 CSE 에서도 판정을 읽고 구독할 수 있다.
          5) 로컬 알림 경로(poa)를 다시 박고 telemetry 구독을 다시 건다
          6) 새 cnt-net-status 에 핸드오버 기록을 남긴다

        바꾸지 않는 것
          IN-CSE 쪽(라운드 명령 구독, 드롭박스, 글로벌 모델)은 그대로다.
          메모리의 모델과 이미 모은 윈도우도 그대로 이어 쓴다.
        """
        if not self.cse_watch.should_failover():
            return False
        now = time.time()
        if now < self._next_handover_try:
            return False
        self._next_handover_try = now + float(
            os.getenv("FL_CSE_HANDOVER_RETRY_SEC", "15"))

        old_base, old_cse = self.cse_watch.target()
        down = self.cse_watch.down_for()
        mep = config.MEP_OF_NODE.get(self.node_name, "?")
        print(f"\n  ⇄ [{self.node_name}] MN-CSE {down:.0f}초 무응답 "
              f"— MEC033 에서 {mep} 의 다른 CSE 를 찾는다")
        rep = cse_failover.pick_replacement(self.node_name, (old_base, old_cse))
        if not rep:
            print(f"    ✗ 옮겨 갈 CSE 가 없다. "
                  f"{os.getenv('FL_CSE_HANDOVER_RETRY_SEC', '15')}초 뒤 다시 본다")
            return False
        base, cse, pid = rep
        print(f"    → {base}/{cse}  (IoT 플랫폼 {pid or '?'})")

        # 2) 바인딩. config.mn_endpoint 를 쓰는 곳(setup_resources_standard)도
        #    새 CSE 를 보도록 이 프로세스의 환경변수로 덮는다.
        key = self.node_name.upper()
        os.environ[f"FL_MN_BASE_URL_{key}"] = base
        os.environ[f"FL_MN_CSE_NAME_{key}"] = cse
        om2m.set_endpoint(base, cse)
        self.mn_cse_name = cse
        self.local_model_path = f"{cse}/{self.ae_name}/{self.local_model_cnt}"
        self.net_status_path = f"{cse}/{self.ae_name}/cnt-net-status"
        self.window_scores_path = f"{cse}/{self.ae_name}/{config.WINDOW_SCORES_CNT}"
        self.verdict_cnt_path = (f"{cse}/{config.analysis_ae_name(self.node_name)}"
                                 f"/{config.VERDICT_CNT}")
        self.telemetry_paths = [f"{cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
                                for rb in self.robot_names]
        self.telemetry_discovered = False
        self.telemetry_subs.clear()
        self.telemetry_cursor = {}
        self.cse_watch.set_target(base, cse)

        # 새 CSE 에는 아직 우리 AE 가 없다. AE 신원(FL_USE_AE_IDENTITY=1)으로
        # 나가면 등록 안 된 originator 라 거절당하므로, 리소스를 다 만들
        # 때까지는 관리자 신원으로 나간다. ACP 도 CSEBase 아래라 마찬가지다.
        import setup_resources_standard as srs
        robots_ok = False
        om2m.set_originator(None)
        try:
            # 3) 로봇. Isaac Sim 이 새 CSE 로 옮겨 오는 것을 기다린다.
            wait = float(os.getenv("FL_CSE_HANDOVER_ROBOT_WAIT_SEC", "120"))
            try:
                robots_ok = self._wait_for_robots(timeout=wait)
            except Exception as e:
                print(f"    ⚠ 새 CSE 에서 로봇 확인 실패: {e}")
            if not robots_ok:
                print(f"    ⚠ 새 CSE 에 로봇이 아직 없다. acp-verdict 에 로봇이 "
                      f"빠지고, 수집 루프가 로봇을 계속 다시 찾는다")

            # 4) 우리 리소스
            try:
                srs.setup_mn_resources(self.node_name)
            except Exception as e:
                print(f"    ⚠ 새 CSE 리소스 생성 중 오류(계속 진행): {e}")
        finally:
            om2m.set_endpoint(base, cse)
            if config.USE_AE_IDENTITY:
                om2m.set_originator(self.ae_id)

        # 5) 로컬 알림 경로와 telemetry 구독
        try:
            self._setup_local_notify()
        except Exception as e:
            print(f"    ⚠ 로컬 알림 경로 설정 실패: {e}")
        if robots_ok:
            self._subscribe_to_telemetry()
            if os.getenv("FL_SENSOR_SEED_CURSOR", "1") != "0":
                self._seed_telemetry_cursor()

        # 6) 기록
        self.cse_handovers += 1
        rec = {
            "type": "cse-handover",
            "node": self.node_name,
            "mep": mep,
            "round": round_num,
            "timestamp": time.time(),
            "down_sec": round(down, 1),
            "from": {"base": old_base, "cse": old_cse},
            "to": {"base": base, "cse": cse, "iotPlatformId": pid},
            "source": "MEC033",
            "robots": len(self.robot_names),
        }
        res = om2m.create_content_instance(
            self.net_status_path, rec,
            labels=["cse-handover"] + ([f"round_{round_num}"] if round_num else []),
        )
        print(f"    {'✓' if res else '⚠'} 핸드오버 완료 "
              f"({old_cse} → {cse}){'' if res else ' — 기록은 실패'}")
        return True

    # ct 는 oneM2M 절대시각(UTC). cra 는 쉼표 앞만 받는다.
    CT_FMT = "%Y%m%dT%H%M%S"

    def _cse_now_ct(self):
        """
        CSE 가 보는 '지금'. 텔레메트리 컨테이너들의 마지막 ct 중 최신값이다.

        커서를 로컬 시계로 만들면 CSE 와 시계가 틀어졌을 때 멀쩡한 데이터를
        건너뛰거나 묵은 것을 끌어온다. 기준을 CSE 가 찍은 ct 로 두면 시계
        차이와 무관해진다. 하나도 못 읽으면 None.
        """
        newest = None
        for path in self.telemetry_paths:
            try:
                cin = om2m.get_latest_content_instance(path)
            except Exception as e:
                print(f"    ⚠ {path} 마지막 ct 조회 실패(건너뜀): {e}")
                continue
            ct = ((cin or {}).get("m2m:cin") or {}).get("ct")
            if ct and (newest is None or str(ct) > str(newest)):
                newest = ct
        return newest

    def _seed_telemetry_cursor(self) -> None:
        """
        FL 시작 전에 쌓인 텔레메트리 중 '너무 오래된 것'만 건너뛴다.

        [왜 시각 기준인가]
        전에는 각 컨테이너의 마지막 ct 를 커서로 놓아 쌓여 있던 것을 통째로
        버렸다. 그러면 Isaac Sim 을 먼저 켜 두고 FL 을 켜는 순간, 그동안
        들어온 데이터가 전부 날아간다. "아이작심을 먼저 켜라"고 해 놓고 그
        데이터를 버리는 것은 앞뒤가 안 맞는다.

        반대로 커서를 아예 안 두면 지난 실행의 묵은 데이터까지 끌려온다.

        그래서 경계를 시각으로 둔다. 방금 전까지 들어온 것은 쓰고, 한참 전
        것은 건너뛴다.

        쌓인 것은 전부 쓰는 것이 기본이다. 실험 사이에 telemetry 컨테이너를
        비우고 시작하므로, 거기 남아 있는 것은 곧 이번 실험의 데이터다.
        앞부분을 버릴 이유가 없다.

          FL_SENSOR_BACKLOG_SEC = 0   쌓인 것 전부 쓴다 (기본)
                               > 0   그 초 이내에 쌓인 것만 쓴다. 컨테이너를
                                     못 비우고 묵은 게 섞였을 때만 쓴다.
                               < 0   쌓인 것 전부 건너뛴다

        Round 1 이 쌓인 걸 통째로 먹던 문제는 커서가 막는 게 아니다.
        _collect_round_data_once 의 take_head 가 막는다. 라운드마다 시간순
        앞에서 FL_SENSOR_MIN_WINDOWS 개만 떼고 나머지는 버퍼에 남으므로,
        백로그를 쓰더라도 라운드당 양은 똑같이 50 개다.
        """
        backlog = float(os.getenv("FL_SENSOR_BACKLOG_SEC", "0"))
        n_cnt = len(self.telemetry_paths)

        if backlog == 0:
            print(f"  ⏱ [{self.node_name}] 쌓여 있는 텔레메트리를 처음부터 "
                  f"전부 쓴다. 라운드당 양은 take_head 가 "
                  f"{os.getenv('FL_SENSOR_MIN_WINDOWS', '50')}개로 맞춘다.")
            return

        if backlog > 0:
            newest = self._cse_now_ct()
            if newest is None:
                print(f"  ⏱ [{self.node_name}] 아직 텔레메트리가 없다. "
                      f"커서 없이 시작한다.")
                return

            stamp = str(newest).split(",")[0]
            try:
                base = calendar.timegm(time.strptime(stamp, self.CT_FMT))
                cutoff = time.strftime(self.CT_FMT,
                                       time.gmtime(base - backlog))
            except ValueError as e:
                print(f"  ⚠ [{self.node_name}] ct 해석 실패({stamp}): {e}. "
                      f"커서 없이 시작한다.")
                return

            for path in self.telemetry_paths:
                self.telemetry_cursor[path] = cutoff

            print(f"  ⏱ [{self.node_name}] 최근 {backlog:.0f}초 안에 쌓인 "
                  f"텔레메트리부터 쓴다 (cra={cutoff}, 최신={stamp}, "
                  f"컨테이너 {n_cnt}개).")
            return

        # backlog < 0 : 지금까지 쌓인 것은 전부 건너뛴다.
        seeded, empty = 0, 0
        for path in self.telemetry_paths:
            try:
                cin = om2m.get_latest_content_instance(path)
            except Exception as e:
                print(f"    ⚠ {path} 마지막 ct 조회 실패(건너뜀): {e}")
                continue

            ct = ((cin or {}).get("m2m:cin") or {}).get("ct")
            if ct:
                self.telemetry_cursor[path] = ct
                seeded += 1
            else:
                empty += 1

        print(f"  ⏱ [{self.node_name}] 쌓여 있던 텔레메트리는 건너뛴다 "
              f"(커서 설정 {seeded}개 / 비어 있음 {empty}개). "
              f"지금부터 쌓이는 것만 학습한다.")

    def _load_sensor_data_for_round(self, round_num: int,
                                    max_attempts: int = 3):
        """
        이 라운드에 쓸 데이터를 만든다. 쓸 만해질 때까지 기다린다.

        [왜 기다리나]
        예전에는 정상 주행 구간이 없으면 이 라운드를 통째로 건너뛰었다.
        건너뛴 노드는 아무것도 올리지 않고 다음 라운드로 가버리는데,
        IN-AE 는 그 노드의 업로드를 기다리느라 수집 타임아웃(600초)을
        통째로 버린다. 로그에서 mn1 이 Round 2 를 빠지자 IN-AE 가 (1/2)
        에서 10분을 멈춰 있던 것이 이 때문이다.

        빠질 일이 아니다. 로봇은 계속 돌고 있으니 조금 더 기다리면 정상
        주행 구간이 생긴다. 다른 노드와 같은 라운드에 참여하는 것이 맞다.

        [예산]
        IN-AE 의 수집 타임아웃보다 먼저 끝나야 의미가 있다. 기본값은
        FL_COLLECTION_NOTIFY_TIMEOUT 에서 60초를 뺀 값이고,
        FL_SENSOR_ROUND_MAX_WAIT_SEC 로 따로 줄 수 있다.
        그 안에 끝내 못 만들면 그때는 어쩔 수 없이 SKIP_ROUND 다.
        """
        in_timeout = float(os.getenv("FL_COLLECTION_NOTIFY_TIMEOUT", "600"))
        budget = float(os.getenv(
            "FL_SENSOR_ROUND_MAX_WAIT_SEC",
            str(max(60.0, in_timeout - 60.0))))
        poll = float(os.getenv("FL_SENSOR_ROUND_POLL_SEC", "2.0"))

        deadline = time.time() + budget
        attempt = 0
        next_log = 0.0

        while True:
            attempt += 1
            data = self._collect_round_data_once(
                round_num, hard_deadline=deadline)

            if data is None:
                # 텔레메트리가 아예 비었다. 기다려서 될 일이 아니다.
                return None

            if data is not SKIP_ROUND:
                # 라운드가 성립했다. 이제서야 쓴 만큼을 버퍼에서 덜어낸다.
                # 실패했다면 버퍼는 손대지 않은 채로 남아 재시도가
                # 같은 윈도우를 다시 본다.
                self._round_buffer = self._pending_leftover
                return data

            left = deadline - time.time()
            if left <= 0:
                print(f"    ✗ {budget:.0f}초 동안 학습할 수 있는 구간이 "
                      f"생기지 않았다 (시도 {attempt}회).")
                print(f"      로봇이 주행 중인지 확인하라. "
                      f"이번 라운드는 빠진다.")
                return SKIP_ROUND

            now = time.time()
            if now >= next_log:
                print(f"    ⏳ 쓸 수 있는 구간이 없다. 라운드를 빠지지 않고 "
                      f"기다린다 (시도 {attempt}회, 남은 {left:.0f}s)")
                next_log = now + 15.0

            # 새 텔레메트리가 올 때까지 기다렸다 다시 본다.
            # 커서를 안 밀었으므로 다음 시도에는 지금 것 + 새로 온 것을
            # 같이 받는다. 기다릴수록 양이 늘어난다.
            if self.telemetry_subs:
                self.data_event.wait(timeout=poll)
                self.data_event.clear()
            else:
                time.sleep(poll)

    def _collect_round_data_once(self, round_num: int,
                                 hard_deadline: float = None):
        """수집 1회 시도. 쓸 데이터를 못 만들면 SKIP_ROUND 를 준다."""
        """
        이번 라운드에 학습할 윈도우를 모은다.

        예전 구조에서는 Isaac Sim이 cnt-sensor-data에 "라운드 N 데이터 준비됨"
        메타데이터 CIN을 올리고, MN-AE가 그 round 값이 맞을 때까지 기다렸다.
        실시간 스트림에는 그런 경계가 없다. 로봇은 계속 주행하고 있고,
        이번 라운드에는 방금까지 들어온 구간으로 학습하는 것이 자연스럽다.

        그래서 이렇게 바뀐다.
          1) 라운드 메타데이터를 기다리지 않는다. 최근 윈도우를 긁는다.
          2) 컨테이너가 로봇 대수만큼 있으므로 사업장 단위로 합친다.
          3) 윈도우가 너무 적으면 최소 개수가 모일 때까지만 잠깐 기다린다.

        FL_SENSOR_FROM_CSE=0 이면 로컬 pkl로 되돌아간다(오프라인 검증용).
        """
        site = config.SITE_OF_NODE.get(self.node_name, self.node_name)
        print(f"\n  [{self.node_name}/{site}] round_{round_num} 데이터 수집 ...")

        if not getattr(config, "FL_SENSOR_FROM_CSE", True):
            return self._load_sensor_data_from_pkl(round_num)

        # 라운드마다 다시 찾는다. 중간에 올라온 로봇이 그 라운드부터
        # 들어오고, 빠진 로봇은 목록에서 사라진다.
        if self._refresh_telemetry() and self._telemetry_added:
            self._subscribe_to_telemetry(only=self._telemetry_added)

        # 라운드마다 사업장당 이만큼 모일 때까지 기다린다.
        # 상한(site_cap)과 같은 값이어야 라운드마다 정확히 같은 양이 된다.
        want_min   = int(os.getenv("FL_SENSOR_MIN_WINDOWS", "50"))

        # 사업장 단위 상한. 로봇 수가 사업장마다 다르므로(A 4대, B 2대)
        # 로봇당 상한만 두면 사업장 총량이 2배 차이 나고, FedAvg가 샘플 수로
        # 가중하기 때문에 로봇이 많은 사업장이 더 큰 영향력을 갖는다.
        # 사업장별 기여를 맞추려면 여기서 사업장 단위로 자른다.
        # 0으로 두면 자르지 않는다(예전 동작).
        site_cap = int(os.getenv("FL_SENSOR_MAX_WINDOWS_PER_SITE", "50"))

        # 로봇별 조회 상한은 사업장 상한에서 역산한다. 넉넉히 받아 두고
        # 합친 뒤에 고르게 줄이는 편이, 로봇 하나가 쉬고 있을 때도
        # 사업장 총량을 채울 수 있어 낫다.
        n_robots = max(1, len(self.telemetry_paths))
        default_per_robot = (max(50, site_cap // n_robots * 2)
                             if site_cap > 0 else 300)
        per_robot = int(os.getenv("FL_SENSOR_CIN_LIMIT_PER_ROBOT",
                                  str(default_per_robot)))
        # 하한이 찰 때까지 기다린다. 실제 상한은 바깥에서 넘겨주는
        # hard_deadline(라운드 예산)이므로 여기 기본값은 그보다 크게 둔다.
        # 예전 기본 90초는 50개가 쌓이기 전에 깨어나 모자란 채로 진행했다.
        wait_total = float(os.getenv("FL_SENSOR_ROUND_WAIT_SEC", "3600"))
        poll       = float(os.getenv("FL_SENSOR_ROUND_POLL_SEC", "2.0"))

        # 라운드마다 "최근 N개"를 다시 긁으면 간격이 짧을 때 같은 데이터를
        # 반복 학습하게 된다. 읽은 지점(ct)을 기억해 그 뒤만 받는다.
        # FL_SENSOR_INCREMENTAL=0 으로 끄면 예전처럼 최근 N개를 받는다.
        incremental = os.getenv("FL_SENSOR_INCREMENTAL", "1") != "0"

        deadline = time.time() + wait_total
        if hard_deadline is not None:
            deadline = min(deadline, hard_deadline)
        next_log = 0.0

        # 버퍼는 라운드가 바뀌어도 비우지 않는다.
        # 이번 라운드가 쓰고 남긴 윈도우를 다음 라운드가 이어서 쓴다.
        # 그래야 라운드마다 정확히 같은 양이 되고, 버려지는 것이 없다.
        self._round_buffer_round = round_num

        n_fresh = 0        # 직전 조회에서 새로 받은 수
        n_fetched = 0      # 이 라운드에 조회로 받은 누계
        n_polls = 0

        while True:
            # 수집 중에 MN-CSE 가 죽었으면 같은 MEP 의 다른 CSE 로 옮기고
            # 이어서 모은다. 이미 버퍼에 쌓인 윈도우는 그대로 쓴다.
            self._maybe_cse_handover(round_num)
            if incremental:
                fresh, cursors = amr_ingest.fetch_site_windows_since(
                    self.telemetry_paths, self.telemetry_cursor, per_robot)
                n_fresh = len(fresh["signals"])
                n_fetched += n_fresh
                n_polls += 1

                # 받은 즉시 커서를 민다. 안 밀면 다음 조회가 같은 구간을
                # 다시 받는데, telemetry 컨테이너는 mni 때문에 그 사이
                # 오래된 CIN 을 지운다. 그래서 총량이 로봇 수 × mni 에서
                # 멈춘다. 지워지기 전에 챙겨 버퍼에 쌓는 것이 맞다.
                self.telemetry_cursor = cursors
                self._round_buffer = amr_ingest.merge_windows(
                    self._round_buffer, fresh)
                got = self._round_buffer
            else:
                got = amr_ingest.fetch_site_windows(
                    self.telemetry_paths, per_robot)
            n = len(got["signals"])

            if n >= want_min:
                if n_polls <= 1:
                    # 기다릴 필요가 없었던 경우. 진행 줄이 하나도 안 찍혀서
                    # 수집이 빠진 것처럼 보이므로 한 줄 남긴다.
                    print(f"    [{self.node_name}] ✓ 윈도우 {n}/{want_min} "
                          f"— 첫 조회에서 이미 찼다 (쌓여 있던 분량)")
                break

            # 조회를 10번(약 20초) 해도 하나도 없으면 컨테이너마다 CIN 이 몇 개 있는지
            # 한 번 찍는다. cni 가 0 이면 아무도 안 쓰고 있는 것(Isaac 전송
            # 또는 잘못 잡은 컨테이너)이고, cni 가 있는데 0 개면 읽는 쪽 문제다.
            if (n == 0 and n_polls >= 10
                    and not getattr(self, "_empty_diag_done", False)):
                self._empty_diag_done = True
                print(f"    [{self.node_name}] ⓘ 받은 윈도우가 없다. 컨테이너 상태:")
                for _p in self.telemetry_paths:
                    _r = (om2m.get_resource(_p) or {}).get("m2m:cnt") or {}
                    print(f"      {_p}  cni={_r.get('cni', '?')}  "
                          f"lbl={_r.get('lbl', [])}")

            now = time.time()
            if now >= deadline:
                if n == 0 and incremental and self.telemetry_cursor:
                    print(f"    ⊘ 지난 라운드 이후 새 윈도우가 없다.")
                    print(f"      로봇이 주행 중인지, 라운드 간격이 너무 "
                          f"짧지 않은지 확인하라.")
                    return SKIP_ROUND   # 바깥에서 다시 기다린다
                if n == 0:
                    print(f"    ✗ {site} 로봇 텔레메트리가 비어 있다.")
                    print(f"      확인: {len(self.telemetry_paths)}개 컨테이너 "
                          f"({', '.join(self.robot_names)})")
                    print("      Isaac Sim이 이 MN-CSE에 등록·전송 중인지 보라.")
                    return None
                print(f"    ⚠ 윈도우 {n}개로 진행한다 (최소 {want_min}개 기대)")
                break

            if now >= next_log:
                via = "구독" if self.telemetry_subs else "주기 조회"
                # 새로 받은 수(+N)를 같이 찍는다. 계속 +0 이면 CSE 가
                # 커서 이후 CIN 을 안 주는 것이고, +N 인데 왼쪽 숫자가
                # 안 늘면 쌓는 쪽이 잘못된 것이다. 로그만 보고 가려진다.
                print(f"    [{self.node_name}] ⏳ 윈도우 {n}/{want_min} 수집 중 "
                      f"(직전 +{n_fresh}, 조회누계 {n_fetched}, "
                      f"{n_polls}회, {via}, 남은 {deadline - now:.0f}s)")
                next_log = now + 5.0

            # 구독이 걸려 있으면 알림이 깨울 때까지 기다린다.
            # 주기적으로 CSE를 찌르지 않아도 새 데이터가 오는 순간 반응한다.
            # 알림이 유실돼도 poll 초마다 한 번은 깨어나므로 멈추지 않는다.
            if self.telemetry_subs:
                self.data_event.wait(timeout=poll)
                self.data_event.clear()
            else:
                time.sleep(poll)

        # 커서는 위 루프에서 받을 때마다 이미 밀었다.
        # 받은 윈도우가 쓸 수 없는 것이어도 버퍼에 남아 있으므로,
        # 재시도할 때 다시 받을 필요가 없다.

        # 시간순으로 줄 세우고 앞에서 want_min 개만 떼어 쓴다.
        #
        # 버퍼는 로봇별로 받아 이어 붙인 것이라 [AMR_1 전부][AMR_2 전부]
        # 처럼 묶여 있다. 정렬 없이 앞에서 자르면 로봇 하나만 가져간다.
        # 정렬 키는 t_wall(로봇이 측정한 시각)이고, 없으면 ct 로 떨어진다.
        #
        # 남은 것은 버리지 않고 버퍼에 둔다. 다음 라운드가 그 뒤를 본다.
        # 예전에는 전 구간에서 linspace 로 뽑고 나머지를 버렸는데, 그러면
        # 한 시점에 생긴 이상이 여러 라운드로 흩어지고 데이터도 사라졌다.
        take = want_min if want_min > 0 else len(got["signals"])
        if len(got["signals"]) > take:
            before = len(got["signals"])
            got = amr_ingest.sort_by_time(got)
            got, leftover = amr_ingest.take_head(got, take)
            # 여기서 바로 버퍼를 줄이면 안 된다. 아래 _build_round_data
            # 에서 정상 구간이 하나도 없어 SKIP_ROUND 가 나면 방금 뗀
            # 윈도우가 사라진다. 라운드가 성립한 뒤에 바깥에서 옮긴다.
            self._pending_leftover = leftover
            print(f"    [{self.node_name}] ⓘ 시간순 {before}개 중 앞 "
                  f"{len(got['signals'])}개 사용, "
                  f"{len(leftover['signals'])}개는 다음 라운드로")
        else:
            got = amr_ingest.sort_by_time(got)
            self._pending_leftover = None

        if self.telemetry_subs:
            print(f"    [{self.node_name}] ⓘ 이번 수집까지 받은 알림 "
                  f"{self.data_notifications}건 "
                  f"(구독 {len(self.telemetry_subs)}개)")

        robots_seen = sorted(set(got["robot"].tolist()))
        print(f"    [{self.node_name}] ✓ 윈도우 {len(got['signals'])}개 "
              f"/ 로봇 {len(robots_seen)}대 "
              f"(CIN {got['n_raw']}개"
              f"{', 증분' if incremental else ', 최근 N개'})")
        if len(robots_seen) < len(self.robot_names):
            missing = [r for r in self.robot_names if r not in robots_seen]
            print(f"    ⚠ 데이터가 없는 로봇: {', '.join(missing)}")

        # 로봇별 몫을 강제로 맞추지 않는다. 실제 주행량이 그대로 반영되는
        # 것이 맞다. 다만 한 로봇이 독식하면 그 라운드는 그 로봇의 경로만
        # 학습하므로, 나중에 손실이 이상할 때 원인을 알 수 있게 찍어둔다.
        counts = amr_ingest.robot_counts(got)
        if counts:
            print(f"    [{self.node_name}] ⓘ 로봇별: "
                  + "  ".join(f"{r.split('_')[-1]}={c}"
                              for r, c in sorted(counts.items())))

        gaps = amr_ingest.seq_gaps(got)
        if gaps:
            print(f"    [{self.node_name}] ⚠ 윈도우 유실(seq 기준): "
                  + "  ".join(f"{r.split('_')[-1]}={n}개"
                              for r, n in sorted(gaps.items())))

        return self._build_round_data_from_cse(got, round_num)

    def _load_sensor_data_from_pkl(self, round_num: int):
        """
        오프라인 폴백. prepare_data_amr.py가 만든 pkl을 라운드 수로 등분해
        이번 라운드 몫만 쓴다. CSE 없이 학습 로직만 돌려볼 때 쓴다.
        """
        path = config.DATA_PATHS.get(self.node_name)
        if not path or not os.path.exists(path):
            print(f"    ✗ pkl 없음: {path}")
            return None

        with open(path, "rb") as f:
            data_dict = pickle.load(f)

        train_sigs = data_dict.get("train_signals", np.array([]))
        total = len(train_sigs)
        if total == 0:
            print("    ✗ pkl에 train_signals가 없다")
            return None

        if config.GLOBAL_ROUNDS > 1:
            per = max(1, total // config.GLOBAL_ROUNDS)
            start = (round_num - 1) * per
            end = total if round_num >= config.GLOBAL_ROUNDS else start + per
        else:
            start, end = 0, total

        start = min(start, total - 1)
        end = max(end, start + 1)

        data_dict = dict(data_dict)
        data_dict["train_signals"] = train_sigs[start:end]
        if data_dict.get("train_cond") is not None:
            data_dict["train_cond"] = data_dict["train_cond"][start:end]
        data_dict["active_round"] = int(round_num)
        data_dict["round_start_idx"] = int(start)
        data_dict["round_end_idx"] = int(end)
        data_dict["sensor_meta"] = {"source": "pkl", "round": int(round_num)}

        print(f"    ✓ pkl 폴백 [{start}:{end}] "
              f"({end - start}/{total} 윈도우)")
        return data_dict


    def _build_round_data_from_cse(self, got: dict, round_num: int = 1) -> dict:
        """
        CSE에서 받은 윈도우를 학습용 dict로 만든다.

        정지 구간은 결함이 드러날 여지가 거의 없어 뺀다.
        정규화 통계는 이 노드가 보관한 것을 쓰고, 없으면 받은 데이터로 잡는다.
        사업장·로봇마다 정상의 모습이 달라 공용 통계를 쓰면 정상도 이상으로 보인다.
        """
        import amr_schema as sch

        X, C = got["signals"], got["cond"]
        mv = sch.moving_mask(X)
        if mv.any():
            X, C = X[mv], C[mv]
            robot = got["robot"][mv]
            names = got["cond_name"][mv]
        else:
            robot, names = got["robot"], got["cond_name"]

        # 고정 전이면 이번 창까지 넣어 통계를 다시 잡고, 그 자로 표준화한다.
        was_frozen = self.norm.frozen
        if len(X):
            self.norm.update(X, C, round_num)
        Xn = self.norm.apply(X) if len(X) else X
        if not was_frozen:
            print(f"      ◦ {self.norm.describe()}")

        # 학습은 정상 주행만 쓴다.
        #
        # 예전에는 정상이 하나도 없으면 전부 학습 대상으로 돌렸는데, 그건
        # 이상 데이터로 정상 패턴을 배우는 꼴이라 목적과 정면으로 충돌한다.
        # 더 나쁜 것은 그 결과가 FedAvg를 타고 다른 사업장까지 번진다는 점이다.
        # 그래서 지금은 되돌리지 않고 이 라운드를 건너뛴다.
        #
        # 제외 nav(중단된 주행 등)도 오프라인 전처리와 같은 기준으로 뺀다.
        # 온라인과 오프라인이 다른 학습 집합을 쓰면 비교가 안 된다.
        # 판정에 쓸 윈도우를 남겨 둔다.
        #
        # 학습 대상(정상)만 추리기 전의 '이동 중인 윈도우 전부'가 대상이다.
        # 이상을 찾는 것이 목적인데 정상만 보면 찾을 것이 없다.
        cts = got.get("ct")
        if cts is not None and len(cts) == len(mv):
            cts = cts[mv] if mv.any() else cts

        # 판정 CIN 에 "바닥 이상 위치"를 싣기 위해 윈도우별 좌표도 남긴다.
        # pose_of 가 윈도우 안 샘플의 평균 (x, y) 로 만들어 둔 값이다.
        # 정지 구간을 걸러낸 mv 마스크를 다른 배열과 똑같이 적용해야
        # 인덱스가 어긋나 엉뚱한 로봇의 좌표가 붙지 않는다.
        poses = got.get("pose")
        if poses is not None and len(poses) == len(mv):
            poses = poses[mv] if mv.any() else poses

        self.last_windows = {
            "X": Xn, "C": C, "robot": robot, "ct": cts, "pose": poses,
            # 대시보드가 이상 구간을 표시할 때 쓴다.
            # 이상 주입을 끈 실험에서는 전부 0 이다.
            "labels": (names != "normal").astype(np.int64),
        }

        navs = got.get("nav")
        if navs is not None and len(navs) == len(mv):
            navs = navs[mv] if mv.any() else navs
        # 라벨 기반 필터와 품질 기반 필터를 나눠 둔다.
        #
        #  label_ok : cond/nav 가 알려주는 '정답'. 이걸로 학습 집합을
        #             추리면 "이 구간이 정상임을 미리 안다"는 가정이 되어
        #             비지도 주장이 약해진다. 기본으로 쓰지 않는다.
        #  nav_ok   : 'aborted' 같은 끊긴 주행 제외. 정답이 아니라 사실이고,
        #             끊긴 구간은 어느 쪽이든 학습에 못 쓴다. 항상 적용한다.
        label_ok = names == "normal"

        nav_ok = np.ones(len(Xn), dtype=bool)
        if navs is not None and len(navs) == len(nav_ok):
            nav_ok = ~np.isin(navs, list(sch.EXCLUDE_NAV_TRAIN))

        # FL_LABEL_FOR_TRAIN=1 이면 예전처럼 정상 라벨만 학습한다.
        # 둘을 각각 돌려 비교하면 "라벨 없이도 이만큼 된다"가 수치로 남는다.
        use_label = os.getenv("FL_LABEL_FOR_TRAIN", "0") not in (
            "0", "false", "False")
        train_mask = (nav_ok & label_ok) if use_label else nav_ok

        # ── 학습 전 판정 ─────────────────────────────────
        # 직전 라운드의 판정기(모델 + 기준 + 임계값)로 먼저 채점한다.
        # 예전에는 이번 창으로 학습하고, 그 창으로 기준을 다시 잡고, 그 창을
        # 채점했다. 이상 창이 학습과 기준에 다 들어가 점수가 눌렸다.
        pre = self._prescore_windows(Xn, C)
        flagged = np.zeros(len(Xn), dtype=bool)
        if pre is not None:
            scores, thr = pre
            flagged = scores > thr
            self.last_windows["pre_scores"] = scores
            self.last_windows["pre_threshold"] = thr
            print(f"      ◦ 학습 전 판정: 직전 판정기로 {int(flagged.sum())}/"
                  f"{len(flagged)}창이 임계 {thr:.2f} 초과 "
                  f"(최대 {float(scores.max()):.2f})")
            # 기준이 고정된 뒤에만 학습에서 뺀다. 그 전에는 판정기 자체가
            # 아직 덜 잡혀 있어서 정상을 잘못 뺄 수 있다.
            if self._calib_frozen and EXCLUDE_FLAGGED and flagged.any():
                rest = train_mask & ~flagged
                if int(rest.sum()) >= 8:
                    train_mask = rest
                    print(f"      ◦ 임계 초과 {int(flagged.sum())}창은 이번 학습에서 뺀다")
                else:
                    # 거의 전부가 넘으면 빼고 나서 학습할 창이 없다. 그러면
                    # 라운드가 SKIP 되고 같은 창이 다시 들어와 멈춘다.
                    # 이 라운드만 그대로 학습한다(기준은 고정이라 안 움직인다).
                    print(f"      ⚠ {int(flagged.sum())}/{len(flagged)}창이 임계 초과 — "
                          f"빼면 학습할 창이 모자라 이번엔 빼지 않는다")

        # ── 정상 기준 창 ────────────────────────────────
        if not self._calib_frozen:
            keep = train_mask & ~flagged
            if keep.any():
                self._calib_X.append(X[keep])
                self._calib_C.append(C[keep])
            if round_num >= REF_FREEZE_ROUND:
                self._calib_frozen = True
                self._calib_frozen_round = round_num
                n_ref = sum(len(a) for a in self._calib_X)
                print(f"      ◦ 정상 기준 창 고정 (R{round_num}, {n_ref}창). "
                      f"이후 기준·임계값은 이 창으로만 잡는다")
        calib_X = (np.concatenate(self._calib_X) if self._calib_X else None)
        calib_C = (np.concatenate(self._calib_C) if self._calib_C else None)

        n_train = int(train_mask.sum())
        n_label_anom = int((~label_ok).sum())

        print(f"      이동 {len(Xn)}개 중 학습 대상 {n_train}개 "
              f"(라벨 이상 {n_label_anom}개"
              f"{', 학습에서 제외' if use_label else ', 학습에 포함'})")

        if n_train == 0:
            print("      ✗ 학습에 쓸 윈도우가 없다. 이 라운드는 학습하지 않는다.")
            return SKIP_ROUND

        return {
            "train_signals": Xn[train_mask],
            "train_cond": C[train_mask],
            "val_signals": Xn,
            "val_cond": C,
            "val_labels": (names != "normal").astype(np.int64),
            "from_cse": True,
            "robots": sorted(set(robot.tolist())),
            # 기준·임계값용. 정규화는 지금 자로 다시 한다(원신호로 모았다).
            "calib_signals": (self.norm.apply(calib_X)
                              if calib_X is not None else None),
            "calib_cond": calib_C,
        }

    def _prescore_windows(self, Xn, C):
        """직전 라운드 판정기로 채점한다. (점수, 임계값) 또는 None."""
        det = self._detector
        if det is None or len(Xn) == 0:
            return None
        thr = getattr(det, "threshold", None)
        if thr is None:
            return None
        try:
            return np.asarray(det.compute_anomaly_scores(Xn, C)), float(thr)
        except Exception as e:
            print(f"      ⚠ 학습 전 판정 실패(학습은 계속): {e}")
            return None

    def _build_ae_node_from_data_dict(self, data_dict: dict) -> AEEdgeNode | None:
        """
        PKL dict → AEEdgeNode
        PKL 형식 (prepare_data_amr.py 출력):
          train_signals, val_signals, val_labels, norm_mean, norm_std, ...
        """
        train_sigs = data_dict.get("train_signals")
        val_sigs   = data_dict.get("val_signals")
        val_labels = data_dict.get("val_labels")

        if train_sigs is None or len(train_sigs) == 0:
            print(f"    ✗ train_signals 없음")
            return None

        if val_sigs is None:
            val_sigs   = np.empty((0, train_sigs.shape[1]), dtype=np.float32)
            val_labels = np.empty((0,), dtype=np.int64)

        # 이상 노드 시뮬레이션 (테스트용)
        if self.inject_anomaly:
            noise_scale = 2.0  # 정규화된 신호 기준이므로 작게
            print(f"  ⚠ [ANOMALY] 센서 데이터 노이즈 주입 (scale={noise_scale})")
            train_sigs = train_sigs + np.random.randn(*train_sigs.shape).astype(
                np.float32
            ) * noise_scale

        train_cfg = getattr(config, "TRAIN_CFG", config.TrainConfig())
        ae_cfg    = getattr(config, "AE_CFG",    config.AEConfig())

        edge_node = AEEdgeNode(
            node_id       = self.node_id,
            train_signals = train_sigs,
            val_signals   = val_sigs,
            val_labels    = val_labels,
            ae_cfg        = ae_cfg,
            train_cfg     = train_cfg,
            device        = "cpu",
            train_cond    = data_dict.get("train_cond"),
            val_cond      = data_dict.get("val_cond"),
            train_run_id  = data_dict.get("train_run_id"),
            val_run_id    = data_dict.get("val_run_id"),
            calib_signals = data_dict.get("calib_signals"),
            calib_cond    = data_dict.get("calib_cond"),
        )
        return edge_node

    # ---------------------------
    # Local cache metadata
    # ---------------------------
    def _read_local_cache_meta(self):
        if not os.path.exists(self.local_cache_meta_file):
            return None
        try:
            with open(self.local_cache_meta_file, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            return None

    def _write_local_cache_meta(self, cached_round: int, local_path: str, src_uri: str):
        payload = {
            "type":       "GLOBAL_MODEL_CACHE",
            "round":      cached_round,
            "model_path": local_path,
            "sha256":     (
                self._sha256_file(local_path) if os.path.exists(local_path) else ""
            ),
            "src_uri":    src_uri,
            "timestamp":  time.time(),
        }
        try:
            with open(self.local_cache_meta_file, "w", encoding="utf-8") as f:
                json.dump(payload, f, ensure_ascii=False, indent=2)
            return payload
        except Exception as e:
            print(f"    ⚠ local cache meta write failed: {e}")
            return None

    # ---------------------------
    # Edge cache
    # ---------------------------
    def _read_cache_meta_la(self):
        local = self._read_local_cache_meta()
        if local:
            return local
        cin = om2m.get_latest_content_instance(self.local_model_path)
        if not cin or "m2m:cin" not in cin:
            return None
        con = cin["m2m:cin"]["con"]
        try:
            obj = json.loads(con) if isinstance(con, str) else con
            return obj if isinstance(obj, dict) else None
        except Exception:
            return None

    def _write_cache_meta(
        self,
        cached_round: int,
        local_path: str,
        src_uri: str
    ):
        payload = self._write_local_cache_meta(
            cached_round,
            local_path,
            src_uri
        )

        if not payload:
            return None

        payload["type"] = "global-cache"

        result = om2m.create_content_instance(
            self.local_model_path,
            payload
        )

        if result:
            print(
                f"    ✓ cached meta -> {self.local_model_path} "
                f"(round {cached_round})"
            )
        else:
            print(
                f"    ⚠ cache meta mirror failed "
                f"(local OK, round {cached_round})"
            )
        return payload

    def _try_load_global_from_cache(
        self,
        expected_round: int,
        expected_src_uri: str | None = None
    ):
        meta = self._read_cache_meta_la()
        if not meta:
            return None
        try:
            cached_round = int(meta.get("round", -1))
        except Exception:
            return None
        if cached_round != expected_round:
            return None

        cached_src_uri = str(meta.get("src_uri", ""))
        if expected_src_uri and cached_src_uri != str(expected_src_uri):
            print(
                "    ⚠ cache source URI differs from current command "
                "-> refresh cache"
            )
            return None

        path = meta.get("model_path")
        if not path or not os.path.exists(path):
            return None
        sha = meta.get("sha256", "")
        if sha:
            try:
                if sha != self._sha256_file(path):
                    print("    ⚠ cache sha256 mismatch -> ignore cache")
                    return None
            except Exception:
                return None
        return path

    def _remove_bad_cache(self, model_path: str):
        try:
            if model_path and os.path.exists(model_path):
                os.remove(model_path)
        except OSError:
            pass

        try:
            if os.path.exists(self.local_cache_meta_file):
                os.remove(self.local_cache_meta_file)
        except OSError:
            pass

    def _load_global_state_safely(self, model_path: str) -> bool:
        try:
            state_dict = torch.load(model_path, map_location="cpu")
            self.edge_node.set_state_dict(state_dict)
            return True
        except RuntimeError as exc:
            print(f"    ⚠ incompatible model checkpoint: {model_path}")
            print(f"    ⚠ {exc}")
            self._remove_bad_cache(model_path)
            return False
        except Exception as exc:
            print(f"    ⚠ failed to load model {model_path}: {exc}")
            return False

    def _get_global_model_meta_for_expected_round(
        self,
        expected_round: int,
        global_model_uri: str | None = None
    ):
        """
        글로벌 모델 contentInstance의 con 전체를 돌려준다.
        예전에는 con 안의 model_path만 꺼내 돌려줬지만, 이제는 con에 가중치
        자체(weights_b64)가 실려 오므로 호출측이 둘 중 하나를 고를 수 있어야 한다.
        """
        cin = None

        if global_model_uri:
            resource_path = str(global_model_uri)
            if resource_path.startswith(config.BASE_URL):
                resource_path = resource_path[len(config.BASE_URL):]
            resource_path = resource_path.lstrip("/")
            cin = om2m.get_resource(resource_path)

        # exact URI 조회가 실패한 경우에만 /la를 한 번 확인
        if not cin:
            cin = om2m.get_latest_content_instance(
                self.global_model_path
            )

        if not cin or "m2m:cin" not in cin:
            return None

        con = cin["m2m:cin"].get("con")
        try:
            model_data = (
                json.loads(con)
                if isinstance(con, str)
                else con
            )
        except (TypeError, json.JSONDecodeError):
            return None

        if not isinstance(model_data, dict):
            return None

        try:
            global_round = int(model_data.get("global_round", -1))
        except (TypeError, ValueError):
            return None

        if global_round != expected_round:
            print(
                f"    ⚠ global model round mismatch: "
                f"expected={expected_round}, got={global_round}"
            )
            return None

        return model_data

    def _pull_and_cache_global_model(
        self,
        expected_round: int,
        global_model_uri: str | None = None
    ):
        meta = self._get_global_model_meta_for_expected_round(
            expected_round,
            global_model_uri=global_model_uri
        )

        if not meta:
            return None

        dst_path = os.path.join(
            self.edge_cache_dir,
            f"global_round{expected_round}.pt"
        )

        # con에 가중치가 실려 왔으면 복원해서 캐시에 저장한다.
        # 이 경로만으로 동작하면 MN이 다른 머신에 있어도 FL이 성립한다.
        state_dict = None
        try:
            state_dict = model_codec.decode_state_dict(
                meta,
                fetch_chunk=self._make_chunk_fetcher(meta),
            )
        except Exception as e:
            print(
                f"    ⚠ con 가중치 복원 실패 "
                f"({type(e).__name__}: {e})"
            )

        if state_dict is not None:
            try:
                torch.save(state_dict, dst_path)
                print("    ↓ global weights from con(base64)")
            except Exception as e:
                print(f"    ⚠ 캐시 저장 실패: {e}")
                return None
        else:
            print("    ✗ con 에 글로벌 가중치가 없음")
            return None

        self._write_cache_meta(
            cached_round=expected_round,
            local_path=dst_path,
            src_uri=(
                global_model_uri
                or f"{self.global_model_path}/la"
            ),
        )

        return dst_path

    # ---------------------------
    # 가중치 조각 (CSE 페이로드 상한 대응)
    # ---------------------------
    def _ensure_weights_container(self, parent_path: str) -> str:
        """
        조각을 쌓을 자식 컨테이너를 보장한다.
        부모가 아니라 자식에 쌓는 이유: 부모 컨테이너에는 구독이 걸려 있어서
        조각 수십 개가 전부 알림을 발생시키면 수신측이 폭주한다.
        자식에 쌓으면 CREATE_OF_DIRECT_CHILD 조건에 걸리지 않는다.
        """
        om2m.create_container(
            parent_path,
            "cnt-weights",
            mni=1000,
            mbs=60_000_000,
        )
        return f"{parent_path}/cnt-weights"

    def _upload_weight_chunks(self, round_num: int, enc: dict):
        """
        가중치 base64를 CIN 상한에 맞게 조각내어 올리고, 매니페스트를 돌려준다.
        조각 하나라도 실패하면 None을 돌려 호출측이 폴백하도록 한다.
        """
        chunks, manifest = model_codec.split_for_cin(
            enc,
            getattr(config, "FL_MAX_CIN_BYTES", 48 * 1024),
        )

        cnt_path = self._ensure_weights_container(self.dropbox_cnt_path)

        for i, part in enumerate(chunks):
            res = om2m.create_content_instance(
                cnt_path,
                {"i": i, "d": part},
                resource_name=model_codec.chunk_resource_name(round_num, i),
            )
            if not res:
                print(
                    f"    ✗ 가중치 조각 {i + 1}/{len(chunks)} 업로드 실패"
                )
                return None

        manifest["weights_chunk_container"] = cnt_path
        manifest["weights_chunk_round"] = round_num
        return manifest

    def _make_chunk_fetcher(self, meta: dict):
        """
        매니페스트를 보고 조각을 이름으로 직접 RETRIEVE 하는 함수를 만든다.
        discovery를 쓰지 않으므로 조각 순서가 보장된다.
        """
        cnt_path = meta.get("weights_chunk_container")
        if not cnt_path:
            return None

        chunk_round = meta.get(
            "weights_chunk_round",
            meta.get("global_round", meta.get("round")),
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

    # ---------------------------
    # Drop-box upload
    # ---------------------------
    def _window_scores_block(self) -> dict:
        """
        이번 라운드 윈도우들의 복원오차와 임계값.

        _record_window_scores 가 이것을 자기 MN-CSE 의 cnt-window-scores 에 쓰고,
        대시보드가 거기서 읽어 그래프를 그린다. 드롭박스(IN-CSE)에는 싣지 않는다.

        판정(verdict)과 달리 FL_VERDICT 와 무관하게 항상 쓴다.
        점수 계산이 실패해도 라운드는 계속돼야 하므로 빈 dict 를 준다.
        """
        w = getattr(self, "last_windows", None)
        if not w or w.get("X") is None or len(w["X"]) == 0:
            return {}

        node = getattr(self, "edge_node", None)
        if node is None:
            return {}

        # 판정과 같은 점수를 보여 준다(학습 전, 직전 판정기). 첫 라운드처럼
        # 판정기가 없을 때만 이번 모델로 매긴다.
        if w.get("pre_scores") is not None:
            scores = w["pre_scores"]
            thr = w.get("pre_threshold")
        else:
            try:
                scores = node.compute_anomaly_scores(w["X"], w["C"])
            except Exception as e:
                print(f"    ⚠ 윈도우 점수 계산 실패(업로드는 계속): {e}")
                return {}
            thr = getattr(node, "threshold", None)
        labels = w.get("labels")
        robots = w.get("robot")

        block = {
            "window_scores": [round(float(v), 6) for v in scores],
            "threshold": (float(thr) if thr is not None else None),
        }
        if labels is not None and len(labels) == len(scores):
            block["window_labels"] = [int(v) for v in labels]
        if robots is not None and len(robots) == len(scores):
            block["window_robots"] = [str(v) for v in robots]
        return block

    def _record_window_scores(self, round_num: int) -> None:
        """
        이번 라운드 윈도우별 이상 점수를 자기 MN-CSE 에 남긴다(대시보드 그래프용).

        점수, 정답 라벨, 로봇, 문턱값은 사업장의 탐지 결과다. 판정(cnt-verdict)과
        마찬가지로 사업장 밖으로 내보내지 않는다. 드롭박스(IN-CSE)에는 가중치와
        요약 숫자(손실, 샘플 수)만 간다. 실패해도 라운드는 계속된다.
        """
        block = self._window_scores_block()
        if not block:
            return
        payload = {
            "type": "window-scores",
            "node": self.node_name,
            "round": round_num,
            "timestamp": time.time(),
            **block,
        }
        res = om2m.create_content_instance(
            self.window_scores_path, payload,
            labels=[f"round_{round_num}", "window-scores"],
        )
        if not res:
            print(f"  ⚠ {config.WINDOW_SCORES_CNT} 기록 실패")

    def _record_net_status(self, round_num: int) -> None:
        """
        이번 라운드 로봇 링크(MEC028)·위치(MEC013)를 cnt-net-status 에 남긴다.
        데이터를 다 모은 직전 시점의 상태다. 실패해도 라운드는 계속된다.
        """
        try:
            snap = self.mec.link_snapshot(round_num)
        except Exception as e:
            print(f"  ⚠ {self.node_name}: MEC 링크·위치 조회 실패 — {e}")
            return
        if not snap:
            return
        bad = [r.get("ae") for r in snap["robots"]
               if r.get("verdict") in ("DEGRADED", "LOST")]
        zones = sorted({str(r["zone"]) for r in snap["robots"] if r.get("zone")})
        print(f"  ◦ MEC 링크·위치 ({'+'.join(snap['sources'])}): "
              f"{snap['summary'] or '-'}"
              + (f"  나쁨 {', '.join(bad)}" if bad else "")
              + (f"  zone {', '.join(zones)}" if zones else ""))
        res = om2m.create_content_instance(
            self.net_status_path, snap,
            labels=[f"round_{round_num}", "net-status"],
        )
        if not res:
            print(f"  ⚠ cnt-net-status 기록 실패")

    def _publish_verdicts(self, round_num: int,
                          model_uri: str = None) -> int:
        """
        이번 라운드에 읽은 윈도우로 로봇별 이상 판정을 내고 CSE 에 올린다.

        학습이 끝난 뒤에 부른다. 임계값이 이번 라운드 모델 기준으로
        갱신돼 있어야 판정이 의미를 갖는다.

        config.VERDICT_START_ROUND(기본 6) 전에는 아무것도 올리지 않는다.

        판정이 실패해도 라운드는 계속된다. 연합학습 본체가 부수 기능 때문에
        멈추면 안 된다.
        """
        if not getattr(config, "VERDICT_ENABLED", True):
            return 0

        # 초반 라운드는 건너뛴다. 임계값이 덜 학습된 모델로 잡혀 있어서
        # 이때 내는 판정은 맞든 틀리든 근거가 없다.
        start = int(getattr(config, "VERDICT_START_ROUND", 1))
        if round_num < start:
            print(f"  · 판정 보류 — R{round_num} < R{start} "
                  f"(FL_VERDICT_START_ROUND)")
            return 0

        w = self.last_windows
        if not w or w["X"] is None or len(w["X"]) == 0:
            return 0

        # 학습 전에 직전 판정기로 매긴 점수만 쓴다. 이번 라운드에 학습한
        # 모델로 같은 창을 채점하면 이상이 학습에 흡수돼 점수가 눌린다.
        if w.get("pre_scores") is None:
            print("  · 판정 보류 — 직전 라운드 판정기가 없다")
            return 0
        scores = w["pre_scores"]
        threshold = w["pre_threshold"]

        try:
            return verdict.publish(
                node_name=self.node_name,
                cnt_path=self.verdict_cnt_path,
                origin=self.analysis_ae_id,
                scores=scores,
                robots=w["robot"],
                threshold=threshold,
                round_num=round_num,
                cts=w["ct"],
                poses=w.get("pose"),
                model_uri=model_uri,
                enrich=(self.mec.robot_context
                        if (self.mec.loc.enabled or self.mec.link.enabled)
                        else None),
            )
        except Exception as e:
            print(f"  ⚠ {self.node_name}: 판정 실패 — {e}")
            return 0

    def _upload_inference_result(
        self,
        round_num,
        val_loss,
        val_auroc,
        train_loss: float = 0.0,
        num_samples: int = 0,
        local_only: bool = False,
    ):
        """
        FedAvg에 기여하지 않는 노드의 업로드.

        관측 전용(cold start)과 로컬 전용 둘 다 여기로 온다.
        어느 쪽이든 가중치와 model_path를 싣지 않으므로 집계 대상이 아니다.
        type을 fl-inference로 두어 IN-AE가 구분하고,
        대시보드는 지표만 받아 표시한다.
        """
        # 윈도우별 점수·라벨·로봇·문턱값은 여기 싣지 않는다. 사업장 탐지 결과라
        # 사업장 밖(IN-CSE)으로 내보내지 않고 _record_window_scores 가 MN-CSE 에 쓴다.
        payload = {
            "type": "fl-inference",
            **self._cse_block(),
            "node": self.node_name,
            "round": round_num,

            # 로컬 전용은 학습을 했으므로 실제 값이 들어간다.
            # 관측 전용은 0이다.
            "train_loss": float(train_loss),
            "num_samples": int(num_samples),

            "val_loss": val_loss,
            "val_auroc": val_auroc,
            "timestamp": time.time(),
        }

        result = om2m.create_content_instance(
            self.dropbox_cnt_path,
            payload
        )

        if result:
            print(
                f"  [{self.node_name}] ✓ inference result -> "
                f"{self.dropbox_cnt_path} "
                f"(round {round_num}, 가중치 없음)"
            )

        return result

    def _upload_to_dropbox(
        self,
        round_num,
        train_loss,
        val_loss,
        val_auroc,
        num_samples,
        model_path
    ):
        payload = {
            "type": "fl-update",
            **self._cse_block(),
            "node": self.node_name,
            "round": round_num,

            "train_loss": train_loss,
            "val_loss": val_loss,
            "val_auroc": val_auroc,
            "num_samples": num_samples,
            "timestamp": time.time(),
        }

        # 가중치는 con 안에 실어 보낸다. IN-AE 는 다른 머신이라 이 사업장의
        # 파일 경로로는 읽을 수 없다. 싣지 못하면 지표만 올라가고, IN-AE 가
        # 이번 라운드 집계에서 이 사업장을 뺀다.
        try:
            enc = model_codec.encode_state_dict(
                torch.load(model_path, map_location="cpu"),
                fp16=getattr(config, "FL_WEIGHTS_FP16", True),
            )
            limit = getattr(config, "FL_MAX_CIN_BYTES", 48 * 1024)

            if enc["weights_bytes"] <= limit:
                payload.update(enc)
                print(
                    f"    ↑ weights in con: {model_codec.describe(enc)}"
                )
            elif getattr(config, "FL_CHUNK_WEIGHTS", True):
                manifest = self._upload_weight_chunks(round_num, enc)
                if manifest:
                    payload.update(manifest)
                    print(
                        f"    ↑ weights in con: "
                        f"{model_codec.describe(manifest)} "
                        f"-> {manifest['weights_chunk_container']}"
                    )
                else:
                    print(
                        "    ⚠ 조각 업로드 실패 "
                        "-> 이번 라운드 집계에서 빠진다"
                    )
            else:
                print(
                    f"    ⚠ weights {enc['weights_bytes'] / 1024:.0f}KB "
                    f"> limit {limit / 1024:.0f}KB, 조각 전송 꺼짐 "
                    f"-> 이번 라운드 집계에서 빠진다"
                )
        except Exception as exc:
            print(
                f"    ⚠ weight embedding 실패 ({type(exc).__name__}: {exc}) "
                f"-> 이번 라운드 집계에서 빠진다"
            )

        result = om2m.create_content_instance(
            self.dropbox_cnt_path,
            payload
        )

        if result:
            print(
                f"  ✓ uploaded -> {self.dropbox_cnt_path} "
                f"(round {round_num})"
            )

        return result

    # ---------------------------
    # Main loop
    # ---------------------------
    def run(self):
        print(f"\n=== {self.node_name.upper()} Trainer start (AE 버전) ===")
        print(f"  AE:           {self.ae_name}")
        print(f"  telemetry:    {len(self.telemetry_paths)}개 컨테이너 "
              f"({'라벨 디스커버리' if self.telemetry_discovered else '경로 조립'})")
        # 이름만 찍으면 엉뚱한 컨테이너(status, 우리 AE 의 컨테이너)를
        # 잡았는지 알 수 없다. 경로를 그대로 찍는다.
        for _p in self.telemetry_paths:
            print(f"                {_p}")
        print(f"  local-model:  {self.local_model_path} (Edge cache)")
        print(f"  fl-control:   {self.fl_control_path}")
        print(f"  global-model: {self.global_model_path}")
        print(f"  drop-box:     {self.dropbox_cnt_path}")
        if self.is_cold_start:
            print(
                "  ❄ 관측 전용: 학습하지 않음 "
                "(글로벌 모델 수신 → 추론만 → 가중치 미전송)"
            )
        elif self.is_local_only:
            print(
                "  ◐ 로컬 전용: 자기 데이터로 학습하되 FedAvg에 기여하지 않음 "
                "(글로벌 모델 수신 → 로컬 보정 → 가중치 미전송)"
            )
        if self.inject_anomaly:
            print("  ⚠ ANOMALY MODE: 센서 데이터에 노이즈 주입 예정")

        self.start_server()

        if not self._subscribe_to_fl_control():
            print(f"  ✗ {self.node_name}: failed to subscribe to fl-control")
            return

        self._ensure_local_model_container()

        if not self._wait_for_robots():
            print(f"  ✗ {self.node_name}: 로봇 텔레메트리 없이는 학습할 수 없다")
            return

        # 로봇이 올라온 뒤에 구독을 건다. 없는 컨테이너에는 못 건다.
        self._subscribe_to_telemetry()

        # MEP 에 MEC 앱으로 등록하고 MEC013·028 주소를 찾는다. 실패해도 진행한다.
        try:
            self.mec.start()
        except Exception as e:
            print(f"  ⚠ {self.node_name}: MEC 기동 실패(계속 진행) — {e}")

        # 쌓여 있는 텔레메트리를 처음부터 쓴다(기본).
        # 묵은 게 섞였을 때만 FL_SENSOR_BACKLOG_SEC 으로 경계를 준다.
        if os.getenv("FL_SENSOR_SEED_CURSOR", "1") != "0":
            self._seed_telemetry_cursor()

        # 여기서부터 MN-CSE 를 감시한다. 기동 중 실패는 위에서 이미 걸러진다.
        self.cse_watch.start()

        for round_num in range(1, config.GLOBAL_ROUNDS + 1):
            print("\n" + "─" * 50)
            print(f"[{self.node_name}] wait Round {round_num}")
            print("─" * 50)

            job_state, command_source = self._wait_for_round_command(
                round_num
            )

            state = str(job_state.get("jobState", ""))
            if state == "FL_COMPLETED":
                print(f"  ✓ {self.node_name} done: FL_COMPLETED")
                return

            print(
                f"  ✓ Round {round_num} command received "
                f"via {command_source} (jobState={state})"
            )

            if self.round_start_stagger_sec > 0:
                time.sleep(self.round_start_stagger_sec)

            # 라운드 명령을 기다리는 동안 MN-CSE 가 죽었을 수 있다.
            self._maybe_cse_handover(round_num)

            # 데이터 로드 + AEEdgeNode 생성
            data_dict = self._load_sensor_data_for_round(round_num)

            if data_dict is SKIP_ROUND:
                # 여기까지 왔다는 건 라운드 예산(기본 540초) 내내 기다렸는데도
                # 학습할 수 있는 구간이 끝내 안 생겼다는 뜻이다. 로봇이 아예
                # 안 움직이는 상태라 코드가 더 할 수 있는 게 없다.
                # 평소에는 _load_sensor_data_for_round 가 데이터가 모일 때까지
                # 기다리므로 여기로 오지 않는다.
                print(
                    f"  ⊘ Round {round_num}: 학습 가능한 정상 구간이 없어 "
                    f"이번 라운드는 빠진다 ({self.node_name})"
                )
                continue

            if data_dict is None:
                print(
                    f"  ✗ failed to load data for round {round_num}; "
                    "stop MN-AE"
                )
                return

            self._record_net_status(round_num)

            self.edge_node = self._build_ae_node_from_data_dict(data_dict)
            if self.edge_node is None:
                print(
                    f"  ✗ failed to build AEEdgeNode for round {round_num}; "
                    "stop MN-AE"
                )
                return

            # round N은 global model round N-1을 사용
            expected_round = round_num - 1
            global_model_uri = job_state.get("globalModelUri")

            model_loaded = False
            cached_path = self._try_load_global_from_cache(
                expected_round,
                expected_src_uri=global_model_uri
            )

            if cached_path:
                model_loaded = self._load_global_state_safely(
                    cached_path
                )
                if model_loaded:
                    print(
                        f"    ✓ global model from EDGE cache "
                        f"(round {expected_round})"
                    )

            if not model_loaded:
                pulled_path = self._pull_and_cache_global_model(
                    expected_round,
                    global_model_uri=global_model_uri,
                )

                if not pulled_path:
                    print(
                        f"  ✗ global model unavailable "
                        f"for round {expected_round}; stop MN-AE"
                    )
                    return

                model_loaded = self._load_global_state_safely(
                    pulled_path
                )
                if not model_loaded:
                    print(
                        f"  ✗ incompatible global model "
                        f"for round {expected_round}; stop MN-AE"
                    )
                    return

                print(
                    f"    ✓ global model from IN-AE "
                    f"(round {expected_round})"
                )

            # DP 설정
            # 명령에 없으면 로컬 설정을 따른다. 기본은 DP 끔.
            security_mode = job_state.get(
                "securityMode",
                "DP" if getattr(config, "DP_ENABLED", False) else "NONE")
            privacy_params = job_state.get("privacyParams", {})
            dp_epsilon = float(
                privacy_params.get(
                    "epsilon",
                    getattr(config, "DP_EPSILON", 8.0)
                )
            )
            dp_max_grad_norm = float(
                privacy_params.get(
                    "max_grad_norm",
                    getattr(config, "DP_MAX_GRAD_NORM", 1.5)
                )
            )
            dp_delta = float(
                privacy_params.get(
                    "delta",
                    getattr(config, "DP_DELTA", 1e-5)
                )
            )

            if self.is_cold_start:
                # 학습하지 않는다. 받은 글로벌 모델 그대로 자기 데이터를
                # 평가해서 임계값/AUROC만 구하고, 가중치 없이 올린다.
                print(
                    f"  [{self.node_name}] Round {round_num} "
                    f"COLD START — 학습 없음, 글로벌 모델로 추론만"
                )

                val_loss, val_auroc = self.edge_node._evaluate_val()

                print(
                    f"  ✓ inference only"
                    f"  val_loss={val_loss:.6f}"
                    f"  val_auroc={val_auroc:.4f}"
                )

                if self.upload_stagger_sec > 0:
                    time.sleep(self.upload_stagger_sec)

                self._record_window_scores(round_num)
                if not self._upload_inference_result(
                    round_num=round_num,
                    val_loss=val_loss,
                    val_auroc=val_auroc,
                ):
                    print(
                        f"  ✗ inference result upload failed "
                        f"in round {round_num}; stop MN-AE"
                    )
                    return

                continue

            if security_mode == "DP":
                print(
                    f"  [{self.node_name}] Round {round_num} training..."
                    f" DP (ε={dp_epsilon}, C={dp_max_grad_norm})"
                )
            else:
                print(
                    f"  [{self.node_name}] Round {round_num} training..."
                    f" {security_mode} (DP 비활성)"
                )
                dp_epsilon = None

            train_loss, val_loss, val_auroc = self.edge_node.train_local(
                dp_epsilon=dp_epsilon,
                dp_delta=dp_delta,
                dp_max_grad_norm=dp_max_grad_norm,
            )

            os.makedirs(
                f"/tmp/fl_models/local/{self.node_name}",
                exist_ok=True
            )
            model_path = (
                f"/tmp/fl_models/local/{self.node_name}/"
                f"round{round_num}.pt"
            )
            torch.save(self.edge_node.get_state_dict(), model_path)

            print(
                f"  ✓ train_loss={train_loss:.6f}"
                f"  val_loss={val_loss:.6f}"
                f"  val_auroc={val_auroc:.4f}"
            )

            # 판정은 이번 창을 학습 '전에' 직전 판정기로 매긴 점수로 한다
            # (_prescore_windows). 이번에 학습·보정한 노드는 다음 라운드의
            # 판정기가 된다.
            self._maybe_cse_handover(round_num)
            self._publish_verdicts(round_num)
            self._detector = self.edge_node

            if self.upload_stagger_sec > 0:
                time.sleep(self.upload_stagger_sec)

            self._record_window_scores(round_num)
            if self.is_local_only:
                # 학습은 했지만 가중치는 올리지 않는다.
                # 공통 모델을 받아 자기 데이터로 보정하는 사업장이다.
                # 중앙은 이 노드를 FedAvg와 집계 대기에서 제외한다.
                print(
                    f"  [{self.node_name}] ◐ 로컬 전용: 가중치 미전송 "
                    f"(지표만 보고, 임계값={self.edge_node.threshold})"
                )
                upload_result = self._upload_inference_result(
                    round_num=round_num,
                    val_loss=val_loss,
                    val_auroc=val_auroc,
                    train_loss=train_loss,
                    num_samples=self.edge_node.num_train_samples,
                    local_only=True,
                )
            else:
                upload_result = self._upload_to_dropbox(
                    round_num=round_num,
                    train_loss=train_loss,
                    val_loss=val_loss,
                    val_auroc=val_auroc,
                    num_samples=self.edge_node.num_train_samples,
                    model_path=model_path,
                )

            if not upload_result:
                print(
                    f"  ✗ local update upload failed in round {round_num}; "
                    "stop MN-AE"
                )
                return


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python3 mn_ae_standard.py <node_id> <port> [--anomaly]")
        print("Example: python3 mn_ae_standard.py 0 5001")
        print("Example: python3 mn_ae_standard.py 2 5003 --anomaly")
        raise SystemExit(1)

    node_id        = int(sys.argv[1])
    port           = int(sys.argv[2])
    inject_anomaly = "--anomaly" in sys.argv

    if inject_anomaly:
        print(f"⚠ [ANOMALY MODE] node_id={node_id}")

    MNAETrainer(node_id, port, inject_anomaly=inject_anomaly).run()