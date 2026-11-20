"""
Standard oneM2M FL architecture - IN-AE Coordinator (drop-box aligned)
- IN-AE/cnt-fl-control : jobState + securityMode + privacyParams publish
- IN-AE/cnt-global-model: global model metadata publish
- IN-AE/cnt-local-updates : Drop-box root
- Loss z-score based outlier detection (defense against poisoning attacks)
- ACP access control verification log
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

# AE-ID of the IN-AE. The value used in ACP acor and X-M2M-Origin.
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

        # The IN-AE attaches to the central CSE. Site CSEs are handled by each MN-AE.
        om2m.set_endpoint(*config.in_endpoint())
        om2m.set_in_endpoint(*config.in_endpoint())

        # When on, requests are sent with the IN-AE's own AE-ID.
        if config.USE_AE_IDENTITY:
            om2m.set_originator(ORIGIN_IN_AE)
        self.in_cse_name = om2m.cse_name()

        # All IN-AE resources are on the IN-CSE. There is only one path, so remote only.
        self.notify_target = None
        self.pch_receiver = None

        self.fl_control_path   = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-fl-control"
        self.global_model_path = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-global-model"
        self.dropbox_root      = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-local-updates"
        # Per-round performance measured on the fixed validation set is recorded here.
        # The dashboard reads it and shows it as a log.
        self.fl_inference_path = f"{self.in_cse_name}/{config.IN_AE_NAME}/cnt-fl-inference"

        # Resources/subscriptions are created for every node.
        # Cold-start nodes also need round commands and the global model to run inference.
        self.node_names = list(
            getattr(config, "ALL_NODES", None)
            or [f"mn{i}" for i in range(1, config.NUM_CLIENTS + 1)]
        )

        # Only 'nodes that upload weights to FedAvg' are waited on for aggregation.
        # Both observe-only (cold start) and local-only nodes must be excluded.
        # Previously only cold start was excluded, but that list is usually empty,
        # so the local-only mn3 stayed in the aggregation wait count and the round
        # stalled forever waiting for a third set of weights.
        self.non_contributing_nodes = list(
            getattr(config, "NON_CONTRIBUTING_NODES", None)
            or getattr(config, "COLD_START_NODES", [])
        )
        self.training_nodes = [
            n for n in self.node_names
            if n not in self.non_contributing_nodes
        ]
        self.expected_nodes = len(self.training_nodes)

        # Inference results from non-contributing nodes are collected separately (for dashboard/logs).
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
        # Added: global model / evaluation paths
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
            # Aliases must be created for 'all nodes', including cold-start nodes.
            # (Those nodes send notifications too, so their names must resolve)
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

                # Normal collection path:
                # Use rep.m2m:cin.con from the NOTIFY body directly
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

                # Only when the CSE omitted rep,
                # retrieve the latest CIN of that container once
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

                # fl-update   : includes weights, FedAvg target
                # fl-inference : metrics only, non-contributing nodes (observe-only / local-only)
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
                        # Upload without weights. Not a FedAvg target, so
                        # it is not counted in the aggregation wait.
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
        Decide which path notifications are received on.

        Everything the IN-AE subscribes to is inside the cloud IN-CSE (drop box).
        That CSE cannot call us behind the campus firewall, so the default is pch.
        Unlike the MN-AE, the IN-AE is an AE formally registered on that CSE,
        so it is entitled to own a <pollingChannel>.

        With direct, subscription creation stalls on the CSE-side verification request and
        ends with our read timeout. Use only for recovery.
        """
        port = self.notification_port
        ae_path = f"{om2m.cse_name()}/{config.IN_AE_NAME}"

        if config.remote_notify_mode() != "pch":
            # rr=true alone is not enough. The receiving address must be announced via poa
            # so the CSE can verify the notification target.
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
    # ACP verification
    # -----------------------
    def _verify_acp(self):
        """
        Check that ACPs are applied as intended.

        Each resource lives on a different CSE. The IN-AE's control, model and drop box are on the cloud
        IN-CSE; the MN-AE caches are on each site's MN-CSE. So for each check the binding
        is moved to that CSE, and moved back to the IN-CSE when done.

        404 (no resource) and 403 (access denied) are different problems. The former means setup is
        incomplete; the latter is a policy problem. Reporting them together makes diagnosis impossible.
        """
        print("\n=== ACP 접근 제어 검증 ===")

        in_cse = config.in_endpoint()[1]

        def bind(node=None):
            """If node is None, the IN-CSE; otherwise that site's MN-CSE."""
            if node is None:
                om2m.set_endpoint(*config.in_endpoint())
                return in_cse
            base, cse = config.mn_endpoint(node)
            om2m.set_endpoint(base, cse)
            return cse

        def probe(path, origin, node=None):
            """
            One RETRIEVE. Returns (status_code, part of the body).

            Headers come from om2m._headers(). Building them by hand gets things like Content-Type
            wrong, and then it fails because of the request format, not the ACP.
            """
            bind(node)
            h = om2m._headers()
            h["X-M2M-Origin"] = origin
            # This check was originally sent with RVI 2a. The IN-AE is registered with srv=["2a","3"],
            # so both are valid. There is no reason to change it, so it stays.
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
                # It is not a policy issue; the resource does not exist. Not reported as a bug.
                print(f"    ⚠ 리소스 없음  {short}  (Origin={origin})")
                return

            allowed = code in (200, 201)
            blocked = code == 403 or "no privilege" in body

            if expect_allow:
                mark = "✓ 허용" if allowed else f"✗ 차단됨(버그) HTTP {code}"
            else:
                mark = "✓ 차단" if blocked else f"✗ 허용됨(버그) HTTP {code}"
            print(f"    {mark}  {short}  (Origin={origin})")

        # ── [1] Check acpi links ──────────────────────────
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

        # ── [2] Access that must be blocked ───────────────
        # An attempt by one site to look at another site's data.
        print("\n  [2] 차단돼야 하는 접근")
        mn_a = config.mn_endpoint("mn1")[1]
        mn_b = config.mn_endpoint("mn2")[1]

        # Training app name and AE-ID. Hard-coding them means this spot gets left behind on renames.
        tr1, tr2, tr3 = (config.training_ae_name(n)
                         for n in ("mn1", "mn2", "mn3"))
        id1, id2, id3 = (config.training_ae_id(n)
                         for n in ("mn1", "mn2", "mn3"))

        # This used to check training-app-2/cnt-sensor-data, but that container is gone.
        # Telemetry moved under the robot AE, and there is no ACP of ours there.
        # The same intent (cannot see another site's data) is checked with cnt-net-status.
        check("", f"{mn_b}/{tr2}/cnt-net-status", id1, False, "mn2")
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn2", id1, False)
        check("", f"{mn_a}/{tr1}/cnt-local-model", id2, False, "mn1")
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", id3, False)
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", id1, False)

        # ── [3] Access that must be allowed ───────────────
        print("\n  [3] 허용돼야 하는 접근")
        mn_c = config.mn_endpoint("mn3")[1]

        check("", f"{in_cse}/{ae}/cnt-fl-control", id1, True)
        check("", f"{in_cse}/{ae}/cnt-global-model", id2, True)
        # Formerly training-app-3/cnt-sensor-data. Moved for the same reason as above.
        check("", f"{mn_c}/{tr3}/cnt-net-status", id3, True, "mn3")
        check("", f"{mn_c}/{tr3}/cnt-local-model", id3, True, "mn3")
        check("", f"{in_cse}/{ae}/cnt-fl-control", "CIN-AE", True)
        check("", f"{in_cse}/{ae}/cnt-local-updates/cnt-mn1", "CIN-AE", True)

        # Always switch back to the IN-CSE after the check.
        # If missed here, later round commands go to the wrong CSE.
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

        # Save ACP info before deletion
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

        # Give the CSE time to commit the container creation
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
        Only check whether the subscription exists. Its content is not read.

        403 also counts as 'exists'. The training app creates the subscription with its own AE-ID and there is
        no acpi, so when CIN-AE (not the creator) reads it, ACME returns 403.
        ACME looks up the resource first and checks permissions after, so a missing one is 404.

        A CSE may hide unauthorized resources as 404, so on 404 check once more with the admin
        identity. Only existence is checked; the content is not used.
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
        Before issuing the Round 1 command, confirm that every MN-AE's cnt-fl-control
        subscription has been created.

        This check is a startup readiness barrier, not FL round polling.
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

            # Subscription readiness waits for 'all nodes'.
            # Cold-start nodes also need round commands to run inference, so the criterion
            # differs from the aggregation wait count (expected_nodes = number of training nodes).
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
            # DP is off by default (see the comment on config.DP_ENABLED).
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
        Prepare the Round 0 model with the same CVAE1D structure as the MN-AE.
        FedAvg only works if every node starts from the same initial values.
        If the existing file is incompatible with the current settings, it is regenerated automatically.
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

        # Global weights are distributed inside con (the downstream direction also goes through oneM2M).
        # Site apps are on other machines and have no other way to receive them,
        # so if they cannot be embedded, nothing is distributed.
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

        # Wait briefly so the CSE sends the command NOTIFY after the CIN is committed
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
        One-shot retrieve to recover a missed NOTIFY.
        Not used for repeated polling.
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
    # Weight chunks (to cope with CSE payload limits)
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
        """Build a function that RETRIEVEs chunks directly by name based on the manifest."""
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
        """Split the global model weights, upload them to a child container, and return the manifest."""
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
            # Weights arrive inside con. If missing, the node is left out of this round's aggregation.
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

        # Record which weights were used for merging. Whether it is per-site uniform or proportional to
        # sample count changes how results are read, so it must show in the logs.
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
    # Fixed validation set evaluation
    # -----------------------
    def _evaluate_global(self, round_num: int, model_path: str):
        """
        Measure this round's global model on the fixed validation set.

        Only measuring every round with the same data and the same ruler lets you claim "it keeps getting
        better". The previous val_auroc was computed on the windows received that round, so
        rounds could not be compared, and with no labels in the live stream it was always 0.5.

        Results are recorded as a CIN in cnt-fl-inference. The dashboard reads and shows them.
        FL continues even if evaluation fails.
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

        # ACP verification
        if getattr(config, "VERIFY_ACP", True):
            self._verify_acp()
        else:
            print("\n=== ACP 접근 제어 검증 — 건너뜀 (FL_VERIFY_ACP=0) ===")
            print("  이 진단은 사업장 MN-CSE 에 직접 접속한다.")
            print("  IN-AE 가 중앙에서 돌면 그쪽이 방화벽 뒤라 닿지 않는다.")
            print("  연합학습 동작과는 무관하다.\n")

        # Wait until all MN-AE subscriptions are ready so the Round 1 command is not missed
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

            # Create the round command CIN including the current global model URI
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