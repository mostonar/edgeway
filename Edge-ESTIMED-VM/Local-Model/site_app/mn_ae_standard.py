"""
Standard oneM2M FL architecture - MN-AE Trainer (raw signal AE version)
For AMR driving telemetry (38-channel CVAE)

Changes (compared to the previous statistical-feature version):
- AEEdgeNode (conditional 1D CNN VAE)
- Input: raw time-series signal (N, seq_len)
- Loss: MSE reconstruction loss (normal only)
- val_acc → val_auroc (AUROC based on reconstruction error)
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

# Signal meaning "this round cannot train, but there is no reason to kill the node".
# Must be distinguished from a communication failure (None). The former can recover in the next round;
# the latter cannot.
SKIP_ROUND = object()

# Whether to exclude from that round's training the windows that crossed the threshold in the pre-training verdict.
EXCLUDE_FLAGGED = os.getenv("FL_EXCLUDE_FLAGGED", "1") not in ("0", "false", "False")
# Up to which round normal reference windows are collected before freezing. Aligned with the normalization statistics.
REF_FREEZE_ROUND = int(os.getenv(
    "FL_REF_FREEZE_ROUND", os.getenv("AMR_NORM_FREEZE_ROUND", "5")))


class MNAETrainer:
    def __init__(self, node_id: int, notification_port: int, inject_anomaly: bool = False):
        self.node_id          = node_id
        self.node_name        = f"mn{node_id + 1}"

        # Node roles
        #   Observe-only : no training, inference only (COLD_START_NODES)
        #   Local-only   : trains locally but does not upload weights to FedAvg
        #               (LOCAL_ONLY_NODES — site C falls here)
        #   Otherwise    : training + FedAvg contribution
        self.is_cold_start = self.node_name in getattr(
            config, "COLD_START_NODES", []
        )
        self.is_local_only = self.node_name in getattr(
            config, "LOCAL_ONLY_NODES", []
        )
        self.notification_port = notification_port
        self.inject_anomaly   = inject_anomaly

        # This site's normalization statistics. Accumulated until load states are evenly collected,
        # then frozen (amr_schema.NormStats). Freezing on the first batch captured only empty driving,
        # and |z| jumped into the tens to hundreds on loaded segments.
        import amr_schema as _sch
        self.norm = _sch.NormStats()

        # The scorer. Keeps the node that finished training and calibration in the previous round as is.
        # New windows are scored with it 'before training' (_prescore_windows).
        self._detector = None
        # Normal reference windows (raw signal) for setting the reference and threshold. Collected during the warm-up rounds
        # and frozen at REF_FREEZE_ROUND. After freezing, the reference does not rise along
        # with anomalies that come in.
        self._calib_X: list = []
        self._calib_C: list = []
        self._calib_frozen = False
        self._calib_frozen_round = None

        self.ae_name = config.training_ae_name(self.node_name)
        self.ae_id = config.training_ae_id(self.node_name)

        # When on, requests go to the CSE with this site's AE-ID. Only then do ACPs
        # actually take effect. When off, it goes out as CAdmin as before.
        if config.USE_AE_IDENTITY:
            om2m.set_originator(self.ae_id)

        # Bind to this site's MN-CSE. Each site has a different MEP.
        # The address comes from the IoT platform registered in the MEP's MEC033; if unavailable,
        # it falls back to the existing rule (config.mn_endpoint).
        mn_base, mn_cse = mec_iot.resolve(self.node_name)
        om2m.set_endpoint(mn_base, mn_cse)
        om2m.set_in_endpoint(*config.in_endpoint())
        self.mn_cse_name = mn_cse

        # If the MN-CSE disappears mid-training, move to another CSE of the same MEP
        # (cse_failover.py). Monitoring runs in the background; the switch happens in _maybe_cse_handover.
        self.cse_watch = cse_failover.CseWatch(self.node_name, mn_base, mn_cse)
        self._next_handover_try = 0.0
        self.cse_handovers = 0

        # Resources inside the site CSE.
        #
        # Telemetry accumulates under the robot AEs, not the MN-AE. Isaac Sim
        # registers the robot AEs and CREATEs windows while driving.
        #     <MN-CSE>/siteA_AMR_1/telemetry/<cin>
        # One site is one FL node, so the containers of all its robots are
        # gathered here and merged into "one factory's data".
        # Isaac Sim uploads them with labels, so they are found via discovery. At startup
        # there may be nothing yet, so here they are filled once from config
        # and _refresh_telemetry() replaces them with the real list.
        self.robot_names = config.robot_names(self.node_name)
        self.telemetry_discovered = False
        self._telemetry_added = []
        # How far each robot has been read. Each round reads only what comes after.
        # If empty, the first round fetches the recent portion.
        self.telemetry_cursor = {}

        # Windows collected this round. Because of the telemetry container's mni
        # they cannot all be scraped at once, so they pile up here on every fetch.
        self._round_buffer = None
        self._round_buffer_round = None

        # The share this round will use and leave over. Moved to _round_buffer
        # once the round is actually established.
        self._pending_leftover = None
        self.telemetry_paths = [
            f"{mn_cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
            for rb in self.robot_names
        ]
        self._refresh_telemetry(quiet=True)
        self.local_model_cnt   = "cnt-local-model"
        self.local_model_path  = f"{mn_cse}/{self.ae_name}/{self.local_model_cnt}"

        # Resources inside the central CSE. The @in/ prefix sends them to the center.
        self.fl_control_path   = om2m.in_path(f"{config.IN_AE_NAME}/cnt-fl-control")
        self.global_model_path = om2m.in_path(f"{config.IN_AE_NAME}/cnt-global-model")

        # Drop-box (central)
        self.dropbox_cnt_path = om2m.in_path(
            f"{config.IN_AE_NAME}/cnt-local-updates/cnt-{self.node_name}"
        )

        # NOTIFY-based round start
        # Set of subscription ri used to identify notifications from telemetry subscriptions.
        # They must not be mixed up with round command notifications.
        self.telemetry_subs = set()

        # Signal that a robot uploaded a new window. The collection loop waits on this.
        # Notifications wake it up, so there is no need to poke the CSE every 2 seconds.
        self.data_event = threading.Event()
        self.data_notifications = 0

        # There are two notification paths with different conditions. Tying them to one variable
        # lets one side's setting break the other.
        #   local  : site MN-CSE. Robot telemetry notifications.
        #   remote : cloud IN-CSE. Round command notifications.
        # Time this process started. Round commands issued before this
        # are leftovers from a previous run and are not followed.
        self.started_at = time.time()

        self.local_notify_target = None
        self.remote_notify_target = None
        self.local_pch = None
        self.pch_receiver = None

        # Verdicts (analysis app). Results do not leave the site, so they are kept on the MN-CSE.
        # The identity used is the analysis app, not the MN-AE. In the design they are different apps and
        # ACPs are split by role as well.
        self.analysis_ae_id = config.analysis_ae_id(self.node_name)
        self.verdict_cnt_path = (
            f"{self.mn_cse_name}/"
            f"{config.analysis_ae_name(self.node_name)}/"
            f"{config.VERDICT_CNT}"
        )
        # Windows read this round. Verdicts are issued from these after training finishes.
        self.last_windows = None

        # MEC services (MEC011 registration/discovery, MEC028 link, MEC013 location).
        # Link and location records go into this site training app's cnt-net-status.
        self.mec = mec_services.SiteMec(
            self.node_name,
            endpoint=mec_services.mec_app.endpoint_for(self.notification_port),
        )
        self.net_status_path = (
            f"{self.mn_cse_name}/{self.ae_name}/cnt-net-status"
        )
        # Per-window anomaly scores. Kept only inside the site (MN-CSE). The dashboard reads them from here.
        self.window_scores_path = (
            f"{self.mn_cse_name}/{self.ae_name}/{config.WINDOW_SCORES_CNT}"
        )

        self.job_event = threading.Event()
        self.job_lock = threading.Lock()
        self.latest_job_command = None
        self.last_command_source = None

        # Timeout for checking /la once only when a NOTIFY was missed
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

                # For a telemetry notification, just raise "new data available" and return.
                #
                # Reading the CSE on every notification costs one round trip per window.
                # Here it is only flagged; the actual read happens all at once during round
                # collection. The notification decides "when to read", and the cra incremental query decides "what
                # to read".
                sur = str(sgn.get("sur", "")).strip("/").split("/")[-1]
                if sur in self.telemetry_subs:
                    self.data_notifications += 1
                    self.data_event.set()
                    resp = make_response("", 200)
                    resp.headers["X-M2M-RSC"] = "2000"
                    return resp

                # Normal path:
                # read the round command CIN's con from the NOTIFY body
                command = self._parse_cin_con_from_notification(sgn)

                # Only when the CSE did not send rep,
                # retrieve the latest CIN once, triggered by the NOTIFY
                #
                # If rep is missing, just ignore it. Previously /la was read instead,
                # and that started rounds with stale commands.
                #
                # cnt-fl-control still holds CINs from previous runs. Right after startup,
                # a single notification without rep made /la return the previous run's
                # FL_TRAINING round=1, and the MN-AE took it as this run's
                # round command and started training. That actually happened.
                # It trained and uploaded before the IN-AE had even opened the round,
                # and that upload vanished when the IN-AE cleared the drop box.
                #
                # Missed notifications are already handled by the timeout fallback in
                # _wait_for_round_command. That path checks the round number, so it is safe.
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
        Decide which path notifications are received on. There are two paths with different conditions.

          Local   site MN-CSE -> us.  Robot telemetry notifications come this way.
                Inside the same site, so the CSE can call us directly.
                Set poa on the AE and give a URL as the subscription nu.

          Remote  cloud IN-CSE -> us.  Round commands (cnt-fl-control) come this way.
                The campus firewall drops inbound traffic, so the CSE cannot call us.
                Receive via <pollingChannel> and give the AE-ID as the subscription nu.

        Previously one flag decided both. So turning on pch for the remote side
        also skipped the poa code, which lived only in the direct branch,
        and killed the in-site telemetry subscriptions that had nothing to do with the firewall.
        Now the local setup runs the same no matter what the remote side is.
        """
        port = self.notification_port
        poa = f"http://{config.NOTIFY_HOST}:{port}"
        local_notify = f"http://127.0.0.1:{port}/notify"

        self._setup_local_notify()

        # ── Remote: cloud IN-CSE ─────────────────────────
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
        Local (site MN-CSE -> us) notification path. Called again after a CSE handover.
        """
        port = self.notification_port
        poa = f"http://{config.NOTIFY_HOST}:{port}"
        local_notify = f"http://127.0.0.1:{port}/notify"

        # ── Local: site MN-CSE ───────────────────────────
        local_ae = f"{self.mn_cse_name}/{self.ae_name}"
        if config.local_notify_mode() == "pch":
            self.local_pch = onem2m_pch.setup(
                local_ae, local_notify, label=f"{self.ae_name} / 로컬")
            self.local_notify_target = (
                self.local_pch.origin if self.local_pch else None)
        else:
            # Turning on only rr=true with poa left empty means the CSE cannot verify the notification target,
            # and the subscription is rejected ("Verification request failed for: ...").
            # The MN-CSE is on the same machine but inside a container, so 127.0.0.1
            # does not reach us. The host address must be given.
            om2m.set_point_of_access(local_ae, [poa], reachable=True)
            self.local_notify_target = f"{poa}/notify"

    def _is_stale_command(self, command: dict) -> bool:
        """
        Was this command issued before this process started?

        cnt-fl-control is not cleared between runs. The MN-AE is subscribed to it,
        so the container cannot be deleted and recreated. So the previous run's
        FL_TRAINING round=1 stays there and looks identical to this run's.
        They are told apart by issue time.
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

        # Key fix:
        # In a new run, while the MN-AE is waiting for Round 1,
        # if cnt-fl-control/la still holds FL_COMPLETED from a previous run,
        # it mistakes that for this run's completion and exits.
        #
        # So while waiting for Round 1, FL_COMPLETED is not accepted as an expected command.
        # Round 1 must start with FL_TRAINING/currentRound=1.
        if state == "FL_COMPLETED":
            if round_num == 1:
                print(
                    f"  ⚠ {self.node_name}: ignore stale FL_COMPLETED "
                    f"while waiting Round 1"
                )
                return False

            # From Round 2 on it may be a real completion signal, so it is allowed.
            # However, it is ignored if the round info looks wrong.
            return current_round >= round_num

        return (
            state == "FL_TRAINING"
            and current_round == round_num
        )

    def _wait_for_round_command(self, round_num: int):
        """
        NOTIFY is used as the primary path.
        On the first timeout cnt-fl-control/la is retrieved only once,
        and after that it does not poll again and only waits for NOTIFY.
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

            # One-shot fallback RETRIEVE only on the first timeout
            fallback_checked = True
            print(
                f"  ⚠ {self.node_name}: command NOTIFY timeout; "
                "check cnt-fl-control/la once"
            )

            fallback = self._get_current_job_state()
            # /la may still hold commands from a previous run. The notification path
            # already filters by issue time but this path did not, so the FL_TRAINING round=1 left when the previous run
            # stalled in round 1 was taken as this run's
            # command and training/upload ran before the IN-AE even started.
            # The IN-AE was still at round 0 and discarded that upload as a round mismatch,
            # while the MN was already waiting for round 2, so it stalled in round 1 forever.
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
        Subscribe to the telemetry containers of this site's robots.

        Corresponds to sub-sensor-data-X in design doc 9.8.5. The robot AEs (ADN-AE) are
        created by Isaac Sim; we only subscribe to the containers under them.

        The whole path is inside the site. Robot -> MN-CSE -> MN-AE are all on the same network, so
        direct notifications arrive regardless of the campus firewall. We confirmed that the CSE pod
        actually reaches port 5001 on the host.

        Notifications are used only as a "new data has arrived" signal. The content is read all at once
        during round collection with the cra incremental query. That way there is no round trip per
        window.

        Returns: number of successful subscriptions
        """
        if not getattr(config, "FL_SENSOR_FROM_CSE", True):
            return 0

        # Telemetry notifications come from inside the site. Unrelated to the remote path.
        notify_url = self.local_notify_target
        if not notify_url:
            print(f"    ⚠ 로컬 알림 경로가 없다 — 구독을 걸지 않는다.")
            return 0

        ok = 0
        print(f"\n  [{self.node_name}] 로봇 텔레메트리 구독")
        for robot, tel_path in zip(self.robot_names, self.telemetry_paths):
            # When re-discovering mid-round, subscribe only to newly appeared robots. Deleting everything and
            # resubscribing would miss notifications in between and stall the round being collected.
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
        """Round command subscription. The target is the cloud IN-CSE, so the remote path is used."""
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
        # config.CSE_NAME is the mn1 CSE at import time, so if mn2/mn3 used it
        # they would try to create on someone else's CSE and get 404. Use the site's own CSE.
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
    # Sensor data load (AE version)
    # ---------------------------
    def _refresh_telemetry(self, quiet: bool = False) -> bool:
        """
        Re-find this site's telemetry containers via label discovery.

        Why paths are not hard-coded
          Assembling <MN-CSE>/{site}_AMR_{i}/telemetry bakes the robot count and naming
          rule into the code. When robots are added or renamed the code must change,
          and robots that come up late are never seen. Isaac Sim attaches labels to the containers
          (?fu=1&ty=3&lbl=site:siteC&lbl=type:telemetry),
          so they are found on demand.

        If nothing is found, the existing list is kept. If discovery fails briefly and
        working paths get wiped too, the whole round is lost.
        Returns: True if the list changed.
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

        # Check that just one path actually opens. If the discovery response shape was
        # misread and the list replaced anyway, the whole round is lost.
        if not om2m.resource_exists(paths[0]):
            if not quiet:
                print(f"    ⚠ 디스커버리 경로가 안 열린다 ({paths[0]}) "
                      f"-> 기존 경로 유지")
            return False
        changed = (robots != self.robot_names or paths != self.telemetry_paths)
        # Robots that are new or whose path changed. Subscriptions are created only for these.
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
        Wait until Isaac Sim brings up this site's robot AEs and telemetry.

        Right after the sandbox starts there is only the MN-CSE and no robot AEs. Isaac registers them.
        So starting FL immediately leaves nothing to read. Separately from waiting inside a round,
        this waits once, firmly, at startup.

        Even if the expected count is not reached, proceed once the minimum is ready.
        There is no reason to stop the whole site for one missing robot, and robots that come up late
        are naturally included in the next round's collection.
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
            # Re-search every time so late robots are caught on the next query.
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
    # MN-CSE handover (MEC033)
    # ---------------------------
    def _cse_block(self) -> dict:
        """
        'The MN-CSE currently attached' carried in drop-box uploads. The coordinator and dashboard
        use it to know which CSE a site is currently on. After a handover the dashboard
        goes to this address to read that site's window scores.
        """
        return {"mn_cse": self.mn_cse_name,
                "mn_base": om2m.base_url(),
                "cse_handovers": int(self.cse_handovers)}

    def _maybe_cse_handover(self, round_num: int = None) -> bool:
        """
        If the MN-CSE has not responded for FL_CSE_FAILOVER_SEC or more, move to another
        CSE of the same MEP. Returns True if it moved.

        Called only at safe points (round start, collection loop, just before verdicts).
        The MN-CSE is not used during training, so if it dies then, moving at the next point
        is enough.

        What the move does
          1) From MEC033, pick a responding CSE that is not the current one
          2) Switch this process's MN-CSE binding and every path based on that CSE
          3) Wait for the robots (Isaac Sim) to come up on the new CSE
          4) Create our resources on the new CSE
             (TRAINING-AE, ANALYSIS-AE, 2 ACPs, cnt-local-model,
              cnt-net-status, cnt-verdict — same as setup_resources_standard)
             Why wait for robots first: the robot AE-IDs must go into acp-verdict
             so robots can read and subscribe to verdicts on the new CSE too.
          5) Re-set the local notification path (poa) and re-create the telemetry subscriptions
          6) Record the handover in the new cnt-net-status

        What does not change
          The IN-CSE side (round command subscription, drop box, global model) stays as is.
          The model in memory and the windows already collected carry over as well.
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

        # 2) Binding. Places that use config.mn_endpoint (setup_resources_standard) are also
        #    made to see the new CSE by overriding this process's environment variables.
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

        # Our AE does not exist on the new CSE yet. Going out with the AE identity (FL_USE_AE_IDENTITY=1)
        # gets rejected as an unregistered originator, so until all resources are created
        # requests go out with the admin identity. The same goes for ACPs, which sit under CSEBase.
        import setup_resources_standard as srs
        robots_ok = False
        om2m.set_originator(None)
        try:
            # 3) Robots. Wait for Isaac Sim to move over to the new CSE.
            wait = float(os.getenv("FL_CSE_HANDOVER_ROBOT_WAIT_SEC", "120"))
            try:
                robots_ok = self._wait_for_robots(timeout=wait)
            except Exception as e:
                print(f"    ⚠ 새 CSE 에서 로봇 확인 실패: {e}")
            if not robots_ok:
                print(f"    ⚠ 새 CSE 에 로봇이 아직 없다. acp-verdict 에 로봇이 "
                      f"빠지고, 수집 루프가 로봇을 계속 다시 찾는다")

            # 4) Our resources
            try:
                srs.setup_mn_resources(self.node_name)
            except Exception as e:
                print(f"    ⚠ 새 CSE 리소스 생성 중 오류(계속 진행): {e}")
        finally:
            om2m.set_endpoint(base, cse)
            if config.USE_AE_IDENTITY:
                om2m.set_originator(self.ae_id)

        # 5) Local notification path and telemetry subscriptions
        try:
            self._setup_local_notify()
        except Exception as e:
            print(f"    ⚠ 로컬 알림 경로 설정 실패: {e}")
        if robots_ok:
            self._subscribe_to_telemetry()
            if os.getenv("FL_SENSOR_SEED_CURSOR", "1") != "0":
                self._seed_telemetry_cursor()

        # 6) Record
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

    # ct is oneM2M absolute time (UTC). cra accepts only the part before the comma.
    CT_FMT = "%Y%m%dT%H%M%S"

    def _cse_now_ct(self):
        """
        'Now' as the CSE sees it. The newest of the last ct values of the telemetry containers.

        Building the cursor from the local clock skips good data or pulls in stale data when the clocks
        of the CSE and this host drift. Basing it on the ct stamped by the CSE makes it immune to clock
        differences. None if nothing could be read.
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
        Skip only telemetry that is 'too old' among what accumulated before FL started.

        [Why time-based]
        Previously the cursor was set to each container's last ct, discarding everything accumulated.
        Then, if Isaac Sim was started first and FL afterward, all data that came in meanwhile
        was lost. Saying "start Isaac Sim first" and then throwing that data away
        does not add up.

        Conversely, with no cursor at all, stale data from previous runs gets pulled in.

        So the boundary is set by time. What came in just before is used, and what came in
        long ago is skipped.

        Using everything accumulated is the default. The telemetry containers are emptied
        between experiments, so whatever remains there is this experiment's data.
        There is no reason to discard the beginning.

          FL_SENSOR_BACKLOG_SEC = 0   use everything accumulated (default)
                               > 0   use only what accumulated within that many seconds. Use only when
                                     the containers could not be emptied and stale data is mixed in.
                               < 0   skip everything accumulated

        The problem of Round 1 swallowing the whole backlog is not prevented by the cursor.
        take_head in _collect_round_data_once prevents it. Each round takes only FL_SENSOR_MIN_WINDOWS
        from the front in time order and the rest stays in the buffer, so
        even when the backlog is used, each round gets the same 50.
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

        # backlog < 0 : skip everything accumulated so far.
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
        Build the data for this round. Wait until it becomes usable.

        [Why wait]
        Previously, if there was no normal driving segment, the whole round was skipped.
        A skipped node uploads nothing and moves on to the next round, but
        the IN-AE waits for that node's upload and burns the entire collection timeout (600 s).
        That is why, when mn1 missed Round 2 in the logs, the IN-AE sat at (1/2)
        for 10 minutes.

        There is no need to drop out. The robots keep running, so waiting a bit longer produces
        a normal driving segment. The right thing is to join the same round as the other nodes.

        [Budget]
        It is only meaningful if it finishes before the IN-AE's collection timeout. The default is
        FL_COLLECTION_NOTIFY_TIMEOUT minus 60 s, and
        it can be set separately with FL_SENSOR_ROUND_MAX_WAIT_SEC.
        If it still cannot be built in that time, then SKIP_ROUND is unavoidable.
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
                # Telemetry is completely empty. Waiting will not help.
                return None

            if data is not SKIP_ROUND:
                # The round is established. Only now remove what was used from the buffer.
                # If it had failed, the buffer stays untouched so a retry
                # sees the same windows again.
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

            # Wait for new telemetry and look again.
            # The cursor was not advanced, so the next attempt receives the current data plus
            # what is new. The longer it waits, the more it gets.
            if self.telemetry_subs:
                self.data_event.wait(timeout=poll)
                self.data_event.clear()
            else:
                time.sleep(poll)

    def _collect_round_data_once(self, round_num: int,
                                 hard_deadline: float = None):
        """One collection attempt. Returns SKIP_ROUND if usable data cannot be built."""
        """
        Collect the windows to train on this round.

        In the old structure, Isaac Sim uploaded a metadata CIN "round N data ready" to cnt-sensor-data,
        and the MN-AE waited until that round value matched.
        A live stream has no such boundary. The robots keep driving, and
        it is natural for this round to train on the segment that just came in.

        So it changes like this.
          1) Do not wait for round metadata. Scrape the recent windows.
          2) There is one container per robot, so merge them at the site level.
          3) If there are too few windows, wait briefly only until the minimum count accumulates.

        With FL_SENSOR_FROM_CSE=0 it falls back to local pkl (for offline verification).
        """
        site = config.SITE_OF_NODE.get(self.node_name, self.node_name)
        print(f"\n  [{self.node_name}/{site}] round_{round_num} 데이터 수집 ...")

        if not getattr(config, "FL_SENSOR_FROM_CSE", True):
            return self._load_sensor_data_from_pkl(round_num)

        # Re-discover every round. Robots that came up midway join from that round,
        # and robots that dropped out disappear from the list.
        if self._refresh_telemetry() and self._telemetry_added:
            self._subscribe_to_telemetry(only=self._telemetry_added)

        # Each round waits until this many have accumulated per site.
        # It must equal the cap (site_cap) for every round to get exactly the same amount.
        want_min   = int(os.getenv("FL_SENSOR_MIN_WINDOWS", "50"))

        # Per-site cap. Robot counts differ per site (A: 4, B: 2), so
        # with only a per-robot cap the site totals differ 2x, and since FedAvg weights by sample count
        # sites with more robots get more influence.
        # To balance per-site contributions, cut at the site level here.
        # 0 means no cutting (old behavior).
        site_cap = int(os.getenv("FL_SENSOR_MAX_WINDOWS_PER_SITE", "50"))

        # The per-robot query cap is derived from the site cap. Fetching generously and then
        # thinning evenly after merging is better, because it can still fill the site total
        # when one robot is idle.
        n_robots = max(1, len(self.telemetry_paths))
        default_per_robot = (max(50, site_cap // n_robots * 2)
                             if site_cap > 0 else 300)
        per_robot = int(os.getenv("FL_SENSOR_CIN_LIMIT_PER_ROBOT",
                                  str(default_per_robot)))
        # Wait until the floor is reached. The real limit is the hard_deadline (round budget)
        # passed in from outside, so the default here is set larger than that.
        # The old 90 s default woke up before 50 had accumulated and proceeded short.
        wait_total = float(os.getenv("FL_SENSOR_ROUND_WAIT_SEC", "3600"))
        poll       = float(os.getenv("FL_SENSOR_ROUND_POLL_SEC", "2.0"))

        # Re-scraping "the latest N" every round re-trains on the same data when intervals are short.
        # Remember the read position (ct) and receive only what comes after.
        # Turning it off with FL_SENSOR_INCREMENTAL=0 fetches the latest N as before.
        incremental = os.getenv("FL_SENSOR_INCREMENTAL", "1") != "0"

        deadline = time.time() + wait_total
        if hard_deadline is not None:
            deadline = min(deadline, hard_deadline)
        next_log = 0.0

        # The buffer is not cleared when the round changes.
        # Windows this round leaves over are used by the next round.
        # That way every round gets exactly the same amount and nothing is thrown away.
        self._round_buffer_round = round_num

        n_fresh = 0        # Number newly received in the last query
        n_fetched = 0      # Running total received by queries this round
        n_polls = 0

        while True:
            # If the MN-CSE died during collection, move to another CSE of the same MEP
            # and keep collecting. Windows already in the buffer are used as is.
            self._maybe_cse_handover(round_num)
            if incremental:
                fresh, cursors = amr_ingest.fetch_site_windows_since(
                    self.telemetry_paths, self.telemetry_cursor, per_robot)
                n_fresh = len(fresh["signals"])
                n_fetched += n_fresh
                n_polls += 1

                # Advance the cursor as soon as data is received. Otherwise the next query receives
                # the same segment again, and meanwhile the telemetry container deletes old CINs
                # because of mni. So the total stalls at robots × mni.
                # The right thing is to grab them and buffer them before they are deleted.
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
                    # The case where no waiting was needed. Not a single progress line gets printed and
                    # collection looks skipped, so leave one line.
                    print(f"    [{self.node_name}] ✓ 윈도우 {n}/{want_min} "
                          f"— 첫 조회에서 이미 찼다 (쌓여 있던 분량)")
                break

            # If 10 queries (about 20 s) return nothing, print once how many CINs each container has.
            # cni of 0 means nobody is writing (Isaac sending, or the wrong container
            # was picked); cni present but 0 received means the reading side is the problem.
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
                    return SKIP_ROUND   # Wait again on the outside
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
                # Print the newly received count (+N) as well. If it stays +0, the CSE is
                # not returning CINs after the cursor; if it is +N but the left number does not
                # grow, the accumulating side is wrong. The logs alone tell them apart.
                print(f"    [{self.node_name}] ⏳ 윈도우 {n}/{want_min} 수집 중 "
                      f"(직전 +{n_fresh}, 조회누계 {n_fetched}, "
                      f"{n_polls}회, {via}, 남은 {deadline - now:.0f}s)")
                next_log = now + 5.0

            # If subscriptions are in place, wait until a notification wakes us.
            # Reacts the moment new data arrives without poking the CSE periodically.
            # Even if notifications are lost it wakes once every poll seconds, so it does not hang.
            if self.telemetry_subs:
                self.data_event.wait(timeout=poll)
                self.data_event.clear()
            else:
                time.sleep(poll)

        # The cursor was already advanced on every fetch in the loop above.
        # Even if the received windows are unusable they stay in the buffer,
        # so a retry does not need to fetch them again.

        # Line them up in time order and take only want_min from the front.
        #
        # The buffer is fetched per robot and concatenated, so it is grouped like [all of AMR_1][all of AMR_2].
        # Cutting from the front without sorting takes from only one robot.
        # The sort key is t_wall (time measured by the robot), falling back to ct if absent.
        #
        # The rest is not discarded but kept in the buffer. The next round looks at what follows.
        # Previously it sampled the whole range with linspace and discarded the rest, which
        # scattered an anomaly from one moment across several rounds and lost data as well.
        take = want_min if want_min > 0 else len(got["signals"])
        if len(got["signals"]) > take:
            before = len(got["signals"])
            got = amr_ingest.sort_by_time(got)
            got, leftover = amr_ingest.take_head(got, take)
            # The buffer must not be shrunk right here. If _build_round_data below
            # finds no normal segment and returns SKIP_ROUND, the windows just taken
            # would vanish. They are moved outside once the round is established.
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

        # Per-robot shares are not forced to match. Reflecting actual driving volume as is
        # is correct. But if one robot hogs the round, that round learns only that robot's route,
        # so it is printed to help find the cause if the loss looks odd later.
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
        Offline fallback. Split the pkl made by prepare_data_amr.py evenly by round count and
        use only this round's share. Used to run the training logic without a CSE.
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
        Turn windows received from the CSE into a training dict.

        Stopped segments leave almost no room for defects to show, so they are dropped.
        Use the normalization statistics kept by this node; if absent, derive them from the received data.
        What normal looks like differs per site and robot, so shared statistics make normal look anomalous.
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

        # If not frozen yet, recompute the statistics including these windows and standardize with that ruler.
        was_frozen = self.norm.frozen
        if len(X):
            self.norm.update(X, C, round_num)
        Xn = self.norm.apply(X) if len(X) else X
        if not was_frozen:
            print(f"      ◦ {self.norm.describe()}")

        # Training uses normal driving only.
        #
        # Previously, if there was no normal data at all, everything was used for training, which
        # amounts to learning normal patterns from anomalous data and directly contradicts the goal.
        # Worse, the result spreads to other sites through FedAvg.
        # So now it does not fall back and skips this round instead.
        #
        # Excluded nav values (aborted runs, etc.) are dropped by the same criteria as offline preprocessing.
        # If online and offline use different training sets, they cannot be compared.
        # Keep the windows to be used for verdicts.
        #
        # The target is 'all moving windows' before narrowing down to training (normal) data.
        # The goal is to find anomalies; looking only at normal data leaves nothing to find.
        cts = got.get("ct")
        if cts is not None and len(cts) == len(mv):
            cts = cts[mv] if mv.any() else cts

        # Also keep per-window coordinates to put "floor anomaly location" in the verdict CIN.
        # This is the mean (x, y) of the samples in the window, computed by pose_of.
        # The mv mask that filters out stopped segments must be applied the same way as to the other arrays,
        # or the indices drift and the wrong robot's coordinates get attached.
        poses = got.get("pose")
        if poses is not None and len(poses) == len(mv):
            poses = poses[mv] if mv.any() else poses

        self.last_windows = {
            "X": Xn, "C": C, "robot": robot, "ct": cts, "pose": poses,
            # Used by the dashboard to show anomalous segments.
            # All 0 in experiments with anomaly injection turned off.
            "labels": (names != "normal").astype(np.int64),
        }

        navs = got.get("nav")
        if navs is not None and len(navs) == len(mv):
            navs = navs[mv] if mv.any() else navs
        # Keep label-based filters and quality-based filters separate.
        #
        #  label_ok : the 'ground truth' given by cond/nav. Narrowing the training set with it
        #             assumes "we know in advance this segment is normal", which
        #             weakens the unsupervised claim. Not used by default.
        #  nav_ok   : excludes interrupted runs like 'aborted'. It is a fact, not ground truth,
        #             and interrupted segments cannot be used for training either way. Always applied.
        label_ok = names == "normal"

        nav_ok = np.ones(len(Xn), dtype=bool)
        if navs is not None and len(navs) == len(nav_ok):
            nav_ok = ~np.isin(navs, list(sch.EXCLUDE_NAV_TRAIN))

        # With FL_LABEL_FOR_TRAIN=1, only normal-labeled data is trained on, as before.
        # Running each and comparing records numerically "this is how far it gets without labels".
        use_label = os.getenv("FL_LABEL_FOR_TRAIN", "0") not in (
            "0", "false", "False")
        train_mask = (nav_ok & label_ok) if use_label else nav_ok

        # ── Pre-training verdict ─────────────────────────
        # First score with the previous round's scorer (model + reference + threshold).
        # Previously it trained on this round's windows, re-took the reference from them, and then scored
        # those same windows. Anomalous windows went into both training and the reference, and scores got suppressed.
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
            # Excluded from training only after the reference is frozen. Before that the scorer itself
            # is not settled yet and could wrongly exclude normal data.
            if self._calib_frozen and EXCLUDE_FLAGGED and flagged.any():
                rest = train_mask & ~flagged
                if int(rest.sum()) >= 8:
                    train_mask = rest
                    print(f"      ◦ 임계 초과 {int(flagged.sum())}창은 이번 학습에서 뺀다")
                else:
                    # If nearly all of them cross, nothing is left to train on after exclusion. Then
                    # the round is SKIPped, the same windows come back, and it stalls.
                    # Train this round as is (the reference is frozen, so it does not move).
                    print(f"      ⚠ {int(flagged.sum())}/{len(flagged)}창이 임계 초과 — "
                          f"빼면 학습할 창이 모자라 이번엔 빼지 않는다")

        # ── Normal reference windows ─────────────────────
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
            # For the reference and threshold. Normalization is redone with the current ruler (they were collected as raw signals).
            "calib_signals": (self.norm.apply(calib_X)
                              if calib_X is not None else None),
            "calib_cond": calib_C,
        }

    def _prescore_windows(self, Xn, C):
        """Score with the previous round's scorer. (scores, threshold) or None."""
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
        PKL format (output of prepare_data_amr.py):
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

        # Anomalous node simulation (for testing)
        if self.inject_anomaly:
            noise_scale = 2.0  # Small, since it is relative to the normalized signal
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
        Return the whole con of the global model contentInstance.
        Previously only model_path was extracted from con and returned, but now con carries the weights
        themselves (weights_b64), so the caller must be able to choose either.
        """
        cin = None

        if global_model_uri:
            resource_path = str(global_model_uri)
            if resource_path.startswith(config.BASE_URL):
                resource_path = resource_path[len(config.BASE_URL):]
            resource_path = resource_path.lstrip("/")
            cin = om2m.get_resource(resource_path)

        # Check /la once only if the exact URI retrieve failed
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

        # If weights were carried in con, restore them and save to the cache.
        # If this path alone works, FL holds even when the MN is on a different machine.
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
    # Weight chunks (to cope with CSE payload limits)
    # ---------------------------
    def _ensure_weights_container(self, parent_path: str) -> str:
        """
        Ensure the child container for storing chunks.
        Why store in a child instead of the parent: the parent container has a subscription, so
        if dozens of chunks each triggered a notification, the receiver would be flooded.
        Storing in a child does not match the CREATE_OF_DIRECT_CHILD condition.
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
        Split the base64 weights to fit the CIN limit, upload them, and return the manifest.
        If even one chunk fails, return None so the caller falls back.
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
        Build a function that RETRIEVEs chunks directly by name based on the manifest.
        Discovery is not used, so chunk order is guaranteed.
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
        Reconstruction errors and threshold of this round's windows.

        _record_window_scores writes this to cnt-window-scores on its own MN-CSE,
        and the dashboard reads it from there to draw charts. It is not put in the drop box (IN-CSE).

        Unlike verdicts, it is always written regardless of FL_VERDICT.
        The round must continue even if score computation fails, so an empty dict is returned.
        """
        w = getattr(self, "last_windows", None)
        if not w or w.get("X") is None or len(w["X"]) == 0:
            return {}

        node = getattr(self, "edge_node", None)
        if node is None:
            return {}

        # Shows the same scores as the verdict (pre-training, previous scorer). Only when there is no
        # scorer, as in the first round, is this round's model used.
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
        Record this round's per-window anomaly scores on its own MN-CSE (for dashboard charts).

        Scores, ground-truth labels, robots and thresholds are the site's detection results. Like verdicts
        (cnt-verdict), they are not sent outside the site. Only weights and summary numbers (loss, sample count)
        go to the drop box (IN-CSE). The round continues even on failure.
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
        Record this round's robot link (MEC028) and location (MEC013) in cnt-net-status.
        This is the state right after all data was collected. The round continues even on failure.
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
        Issue per-robot anomaly verdicts from the windows read this round and upload them to the CSE.

        Called after training. The threshold must be updated against this round's model
        for verdicts to be meaningful.

        Nothing is uploaded before config.VERDICT_START_ROUND (default 6).

        The round continues even if verdicts fail. The core federated learning must not
        stop because of an auxiliary feature.
        """
        if not getattr(config, "VERDICT_ENABLED", True):
            return 0

        # Early rounds are skipped. The threshold was set with an under-trained model, so
        # verdicts issued then have no basis, right or wrong.
        start = int(getattr(config, "VERDICT_START_ROUND", 1))
        if round_num < start:
            print(f"  · 판정 보류 — R{round_num} < R{start} "
                  f"(FL_VERDICT_START_ROUND)")
            return 0

        w = self.last_windows
        if not w or w["X"] is None or len(w["X"]) == 0:
            return 0

        # Use only scores assigned before training by the previous scorer. Scoring the same windows with the model
        # trained this round lets anomalies be absorbed into training and suppresses the scores.
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
        Upload from a node that does not contribute to FedAvg.

        Both observe-only (cold start) and local-only nodes come here.
        Either way, no weights or model_path are carried, so it is not an aggregation target.
        type is set to fl-inference so the IN-AE can tell it apart,
        and the dashboard just receives and shows the metrics.
        """
        # Per-window scores, labels, robots and thresholds are not carried here. They are site detection results,
        # so they are not sent outside the site (IN-CSE); _record_window_scores writes them to the MN-CSE.
        payload = {
            "type": "fl-inference",
            **self._cse_block(),
            "node": self.node_name,
            "round": round_num,

            # Local-only nodes did train, so real values go in.
            # Observe-only nodes get 0.
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

        # Weights are sent inside con. The IN-AE is on a different machine and cannot read
        # this site's file paths. If they cannot be embedded, only metrics are uploaded and the IN-AE
        # leaves this site out of this round's aggregation.
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
        # Printing only the name cannot show whether the wrong container (status, our AE's container)
        # was picked. Print the full path.
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

        # Subscribe after the robots come up. You cannot subscribe to a container that does not exist.
        self._subscribe_to_telemetry()

        # Register with the MEP as a MEC app and look up MEC013/028 addresses. Proceeds even on failure.
        try:
            self.mec.start()
        except Exception as e:
            print(f"  ⚠ {self.node_name}: MEC 기동 실패(계속 진행) — {e}")

        # Use the accumulated telemetry from the beginning (default).
        # Set a boundary with FL_SENSOR_BACKLOG_SEC only when stale data is mixed in.
        if os.getenv("FL_SENSOR_SEED_CURSOR", "1") != "0":
            self._seed_telemetry_cursor()

        # From here on the MN-CSE is monitored. Failures during startup are already caught above.
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

            # The MN-CSE may have died while waiting for the round command.
            self._maybe_cse_handover(round_num)

            # Load data + create AEEdgeNode
            data_dict = self._load_sensor_data_for_round(round_num)

            if data_dict is SKIP_ROUND:
                # Getting here means that even after waiting through the whole round budget (default 540 s),
                # no trainable segment ever appeared. The robots are not moving at all,
                # so there is nothing more the code can do.
                # Normally _load_sensor_data_for_round waits until data accumulates,
                # so it does not get here.
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

            # Round N uses global model round N-1
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

            # DP settings
            # If not in the command, follow the local setting. DP is off by default.
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
                # No training. Evaluate its own data with the received global model as is,
                # compute only the threshold/AUROC, and upload without weights.
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

            # Verdicts use scores assigned to this round's windows 'before' training by the previous scorer
            # (_prescore_windows). The node trained and calibrated now becomes next round's
            # scorer.
            self._maybe_cse_handover(round_num)
            self._publish_verdicts(round_num)
            self._detector = self.edge_node

            if self.upload_stagger_sec > 0:
                time.sleep(self.upload_stagger_sec)

            self._record_window_scores(round_num)
            if self.is_local_only:
                # Trained, but weights are not uploaded.
                # This is the site that receives the shared model and fine-tunes it on its own data.
                # The center excludes this node from FedAvg and from the aggregation wait.
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