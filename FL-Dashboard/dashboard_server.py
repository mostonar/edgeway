"""
FL-Dashboard/dashboard_server.py — FL 대시보드 백엔드

백그라운드 스레드:

1. fl-poll
   - oneM2M 상태 폴링
   - 새로운 글로벌 모델 감지
   - validation threshold 재계산
   - test stream reconstruction score 재계산

2. score-stream
   - 테스트 샘플의 재구성 오차를 SSE로 전송
   - score_idx는 글로벌 모델이 갱신돼도 유지
   - MN3는 설정된 라운드 전에는 정상 데이터만 표시
   - 설정된 라운드부터 이상 데이터만 표시

SSE 이벤트:

  init
    접속 직후 현재 상태

  round
    FL 상태 및 라운드 변경

  threshold
    글로벌 모델에 따른 threshold 재계산

  score
    노드별 reconstruction error

  summary
    최종 평가 결과

읽는 곳:

  중앙 IN-CSE    라운드 상태(cnt-fl-control), 사업장별 요약(드롭박스의 손실·샘플 수),
                 글로벌 모델(cnt-global-model), 라운드별 평가(cnt-fl-inference)
  사업장 MN-CSE  윈도우별 이상 점수(TRAINING-AE-i/cnt-window-scores).
                 사업장 탐지 결과라 IN-CSE 로 올라가지 않으므로 사업장 쪽에서 읽는다.

그래서 이 대시보드는 사업장 MN-CSE 에 닿는 호스트(지금 배치에서는 Edge VM)에서 띄운다.

실행:

  bash FL-Dashboard/run_dashboard.sh

주소:

  http://localhost:7000
"""

from __future__ import annotations

import json
import os
import pickle
import queue
import threading
import time
from pathlib import Path

import numpy as np
import torch
from flask import Flask, Response, send_from_directory
from sklearn.metrics import roc_auc_score

import config
import onem2m_utils as om2m
import model_codec
from model_cvae import CVAE1D


# ════════════════════════════════════════════════════════
# 기본 설정
# ════════════════════════════════════════════════════════

PORT = int(
    os.getenv(
        "FL_DASHBOARD_PORT",
        "7000",
    )
)
POLL_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_POLL_INTERVAL",
        "3.0",
    )
)
# 라운드 상태(cnt-fl-control)만 따로, 더 자주 본다. 노드 업로드(가중치가
# 실린 큰 CIN)와 모델 재채점까지 도는 폴링 한 바퀴는 수 초가 걸려서,
# 거기에 묶여 있으면 IN-AE 가 다음 라운드를 열어도 화면이 늦게 따라왔다.
CONTROL_POLL_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_CONTROL_INTERVAL",
        "1.0",
    )
)
SCORE_INTERVAL = float(
    os.getenv(
        "FL_DASHBOARD_SCORE_INTERVAL",
        "0.5",
    )
)
N_SIGMA = float(
    getattr(
        config,
        "THRESHOLD_N_SIGMA",
        3.0,
    )
)
K_CONSECUTIVE = int(
    getattr(
        config,
        "ANOMALY_K_CONSECUTIVE",
        3,
    )
)
ANOMALY_NODE = str(
    getattr(
        config,
        "ANOMALY_DEMO_NODE",
        "mn3",
    )
)
ANOMALY_START_ROUND = int(
    getattr(
        config,
        "ANOMALY_START_ROUND",
        7,
    )
)
# 화면 파일(fl_dashboard.html)과 vendor/ 는 이 파일과 같은 폴더에 둔다.
PROJECT_ROOT = Path(
    __file__
).resolve().parent
HTML_PATH = (
    PROJECT_ROOT
    / "fl_dashboard.html"
)
PKL_DIR = Path(
    getattr(
        config,
        "FL_PKL_DIR",
        "/tmp/fl_data/amr",
    )
)
NODES = [
    "mn1",
    "mn2",
    "mn3",
]

app = Flask(
    __name__,
)


# ════════════════════════════════════════════════════════
# SSE 구독자
# ════════════════════════════════════════════════════════

_subscribers: list[queue.Queue] = []
_subscribers_lock = threading.Lock()


# ════════════════════════════════════════════════════════
# 공유 상태
# ════════════════════════════════════════════════════════

_shared: dict = {
    "fl_state": "FL_READY",
    "round": 0,
    "max_rounds": config.GLOBAL_ROUNDS,

    # node → local training metrics
    "nodes": {},

    # node → threshold
    "thresholds": {},

    # node → reconstruction scores
    "scores": {},

    # node → test labels
    "labels": {},

    # 글로벌 모델이 변경돼도 유지되는 스트림 위치
    "score_idx": 0,

    # 현재 Dashboard가 반영한 global model round
    "model_round": -1,

    # CSE 드롭박스 점수를 반영한 FL round.
    # 글로벌 모델 없이도 라운드마다 패널을 갱신하기 위한 표식이다.
    "scores_round": -1,

    "summary_sent": False,

    "anomaly_start_round": ANOMALY_START_ROUND,

    # 이 라운드 전에는 화면에도 이상 표시를 안 한다.
    # MN 쪽 판정(CIN)을 막는 기준과 같은 값을 쓴다. 둘이 어긋나면
    # CSE 에는 아무것도 없는데 화면만 빨개지는 일이 생긴다.
    "verdict_start_round": int(
        getattr(config, "VERDICT_START_ROUND", 1)
    ),

    # 라운드별 고정 검증셋 평가 결과. 나중에 접속한 클라이언트에도
    # 지난 라운드를 보내기 위해 쌓아 둔다.
    "evals": [],
}

# 평가 로그를 몇 라운드까지 들고 있을지. 라운드 수보다 넉넉하게 둔다.
EVAL_HISTORY_MAX = 60

_shared_lock = threading.Lock()


# ════════════════════════════════════════════════════════
# SSE 헬퍼
# ════════════════════════════════════════════════════════

def _broadcast(event: dict) -> None:
    """
    현재 연결된 모든 SSE 클라이언트에 이벤트를 전송한다.
    """

    message = (
        "data: "
        + json.dumps(
            event,
            ensure_ascii=False,
        )
        + "\n\n"
    )

    with _subscribers_lock:
        disconnected = []

        for subscriber in _subscribers:
            try:
                subscriber.put_nowait(
                    message,
                )
            except queue.Full:
                disconnected.append(
                    subscriber,
                )
        for subscriber in disconnected:
            if subscriber in _subscribers:
                _subscribers.remove(
                    subscriber,
                )


# ════════════════════════════════════════════════════════
# HTTP Endpoint
# ════════════════════════════════════════════════════════

@app.route("/vendor/<path:name>")
def vendor(name: str):
    """
    외부 CDN 대신 저장소 안의 라이브러리를 내준다.

    Chart.js 를 cdnjs 에서 받게 두면 <head> 에서 파싱이 막혀, 캠퍼스 밖에서
    열 때 그게 다 받아질 때까지 화면이 비어 있다. 망이 막혀 있으면 타임아웃
    까지 기다린다. 시연 중에 바깥 망에 기대지 않도록 여기서 내준다.
    """
    return send_from_directory(PROJECT_ROOT / "vendor", name,
                               max_age=86400)


@app.route("/")
def index():
    if not HTML_PATH.exists():
        return (
            f"Dashboard HTML not found: {HTML_PATH}<br>"
            "프로젝트 루트에 "
            "fl_dashboard.html 파일이 있는지 확인하세요.",
            500,
        )

    return HTML_PATH.read_text(
        encoding="utf-8",
    )


@app.route("/health")
def health():
    pkl_files = {
        node: (
            PKL_DIR
            / f"{node}.pkl"
        ).exists()
        for node in NODES
    }

    with _shared_lock:
        state = {
            "ok": True,

            "oneM2M": config.BASE_URL,

            "state": _shared["fl_state"],
            "round": _shared["round"],
            "max_rounds": _shared["max_rounds"],
            "model_round": _shared["model_round"],

            "html_path": str(
                HTML_PATH,
            ),
            "html_exists": HTML_PATH.exists(),

            "pkl_dir": str(
                PKL_DIR,
            ),
            "pkl_dir_exists": PKL_DIR.exists(),
            "pkl_files": pkl_files,

            "nodes": _shared["nodes"],
            "thresholds": _shared["thresholds"],
            "score_idx": _shared["score_idx"],
            "summary_sent": _shared["summary_sent"],

            "anomaly_node": ANOMALY_NODE,
            "verdict_start_round": _shared["verdict_start_round"],
            "anomaly_start_round": (
                ANOMALY_START_ROUND
            ),
        }

    return state


@app.route("/stream")
def stream():
    subscriber: queue.Queue = queue.Queue(
        maxsize=150,
    )
    with _subscribers_lock:
        _subscribers.append(
            subscriber,
        )

    # 접속 직후 현재 상태 전송
    with _shared_lock:
        snapshot = {
            "type": "init",
            "fl_state": _shared["fl_state"],
            "round": _shared["round"],
            "max_rounds": _shared["max_rounds"],
            "nodes": _shared["nodes"],
            "thresholds": _shared["thresholds"],
            "model_round": _shared["model_round"],
            # 접속 전에 끝난 라운드의 평가도 같이 보낸다.
            "evals": list(_shared["evals"]),
        }

    try:
        subscriber.put_nowait(
            "data: "
            + json.dumps(
                snapshot,
                ensure_ascii=False,
            )
            + "\n\n"
        )
    except queue.Full:
        pass

    def generate():
        try:
            while True:
                try:
                    yield subscriber.get(
                        timeout=25,
                    )
                except queue.Empty:
                    yield ": heartbeat\n\n"

        except GeneratorExit:
            pass

        finally:
            with _subscribers_lock:
                if subscriber in _subscribers:
                    _subscribers.remove(
                        subscriber,
                    )

    return Response(
        generate(),
        mimetype="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no",
        },
    )


# ════════════════════════════════════════════════════════
# 현재 데모 실행 시작 시각
# ════════════════════════════════════════════════════════

# ════════════════════════════════════════════════════════
# 최신 글로벌 모델 탐색
# ════════════════════════════════════════════════════════

def _find_latest_model(
    max_round: int | None = None,
) -> tuple[int, Path | None]:
    """
    IN-CSE 의 cnt-global-model 에서 가장 최근 글로벌 모델을 받는다.
    현재 FL round 보다 미래 라운드인 모델은 제외한다.
    반환: (라운드, state_dict) 또는 (-1, None)
    """

    # 글로벌 모델은 IN-AE 가 con 에 실어 cnt-global-model 로 배포한다.
    # 대시보드는 그것을 받는다. IN-AE 와 다른 호스트에서 돌기 때문에
    # IN-AE 의 로컬 디스크(/tmp/fl_models/global)는 보지 않는다.
    return _latest_global_from_cse(max_round)


def _latest_global_from_cse(max_round: int | None = None):
    """
    중앙 CSE 의 cnt-global-model 에서 최신 글로벌 모델을 가져온다.

    반환: (라운드, state_dict) 또는 (-1, None)

    con 에 가중치가 fp16+gzip+base64 로 실려 있다. 조각으로 나뉘어 있으면
    model_codec 이 fetch_chunk 로 나머지를 받아 온다.
    """
    path = om2m.in_path(f"{config.IN_AE_NAME}/cnt-global-model")
    try:
        meta = _parse_content_instance(
            om2m.get_latest_content_instance(path)
        )
    except Exception:
        return -1, None

    if not meta or meta.get("type") != "global-model":
        return -1, None

    try:
        model_round = int(meta.get("global_round", -1))
    except (TypeError, ValueError):
        return -1, None

    if max_round is not None and model_round > max_round:
        return -1, None

    def fetch_chunk(name: str):
        return _parse_content_instance(
            om2m.get_latest_content_instance(
                f"{meta.get('weights_chunk_container', '')}/{name}"
            )
        )

    try:
        state = model_codec.decode_state_dict(meta, fetch_chunk=fetch_chunk)
    except Exception as e:
        print(f"  ⚠ 글로벌 모델 복원 실패 ({type(e).__name__}: {e})")
        return -1, None

    return (model_round, state) if state is not None else (-1, None)


# ════════════════════════════════════════════════════════
# 모델 로드
# ════════════════════════════════════════════════════════

def _load_model(
    model_path: str | Path,
) -> CVAE1D:
    ae_config = config.AE_CFG

    # cond_dim 을 빠뜨리면 CVAE1D 기본값(11)로 만들어진다. 실제 설정은 8이라
    # encoder.0.weight 의 입력 채널이 46 대신 49 가 되고 로드가 실패한다.
    model = CVAE1D(
        n_channels=ae_config.n_channels,
        cond_dim=ae_config.cond_dim,
        latent_dim=ae_config.latent_dim,
        seq_len=ae_config.seq_len,
    )

    # CSE 에서 받아 온 경우에는 이미 state_dict 다. 파일 경로면 읽는다.
    state_dict = (
        model_path
        if isinstance(model_path, dict)
        else torch.load(model_path, map_location="cpu")
    )

    model.load_state_dict(
        state_dict,
    )

    model.eval()

    return model


# ════════════════════════════════════════════════════════
# Reconstruction score 계산
# ════════════════════════════════════════════════════════

def _compute_scores(
    model: CVAE1D,
    signals: np.ndarray,
    batch_size: int = 64,
) -> np.ndarray:
    """
    signals shape:
      (N, sequence_length)

    return shape:
      (N,)
    """

    if len(signals) == 0:
        return np.array(
            [],
            dtype=np.float32,
        )

    score_batches: list[np.ndarray] = []

    tensor = torch.tensor(
        signals,
        dtype=torch.float32,
    )

    with torch.no_grad():
        for start in range(
            0,
            len(tensor),
            batch_size,
        ):
            batch = tensor[
                start:start + batch_size
            ].unsqueeze(1)

            reconstruction = model(
                batch,
            )

            error = (
                (
                    batch
                    - reconstruction
                )
                ** 2
            ).mean(
                dim=(1, 2),
            )
            score_batches.append(
                error.cpu().numpy(),
            )

    return np.concatenate(
        score_batches,
    )


# ════════════════════════════════════════════════════════
# Threshold 계산
# ════════════════════════════════════════════════════════

def _compute_threshold(
    model: CVAE1D,
    validation_signals: np.ndarray,
    validation_labels: np.ndarray,
) -> float:
    normal_signals = validation_signals[
        validation_labels == 0
    ]

    if len(normal_signals) == 0:
        return 1.0

    scores = _compute_scores(
        model,
        normal_signals,
    )

    return float(
        scores.mean()
        + N_SIGMA * scores.std()
    )


# ════════════════════════════════════════════════════════
# 글로벌 모델 변경 시 score 재계산
# ════════════════════════════════════════════════════════

_mn_lock = threading.Lock()


def _mn_endpoint_of(node: str):
    """
    사업장 앱이 지금 붙어 있는 MN-CSE (base, cse).

    드롭박스 업로드의 mn_base / mn_cse 를 따른다. CSE 핸드오버(시나리오 6)
    뒤에도 사업장 앱이 옮겨 간 CSE 를 읽는다. 아직 업로드가 없으면 규칙 주소.
    """
    with _shared_lock:
        data = dict(_shared["nodes"].get(node) or {})
    if data.get("mn_base") and data.get("mn_cse"):
        return data["mn_base"], data["mn_cse"]
    return config.mn_endpoint(node)


def _node_scores_from_cse(node: str):
    """
    사업장 MN-CSE 에서 노드의 윈도우별 점수를 읽는다.

    사업장 앱이 라운드마다 TRAINING-AE-i/cnt-window-scores 에 window_scores /
    window_labels / threshold 를 쓴다. 사업장 탐지 결과라 IN-CSE 로는 올라가지
    않는다. CSE 모드에는 PKL 파일이 없으므로 이쪽을 쓴다.

    반환: (scores, labels, threshold) 또는 None
    """
    base, cse = _mn_endpoint_of(node)
    path = (f"{cse}/{config.training_ae_name(node)}"
            f"/{config.WINDOW_SCORES_CNT}")

    try:
        # om2m 의 사업장 쪽 연결은 하나라, 사업장을 바꿔 가며 읽을 때 잠근다.
        # 다른 스레드는 중앙(@in/) 경로만 쓰므로 영향이 없다.
        with _mn_lock:
            om2m.set_endpoint(base, cse)
            content = _parse_content_instance(
                om2m.get_latest_content_instance(path)
            )
    except Exception as error:
        print(f"    ⚠ {node} 윈도우 점수 조회 실패 ({base}/{path}): {error}")
        return None

    if not content:
        return None

    # 컨테이너는 실행마다 비우지 않는다. 이 대시보드가 뜨기 전에 올라온 CIN 은
    # 지난 실행의 마지막 라운드 점수라, 그걸 쓰면 라운드 0 에서 그래프가 먼저
    # 그려졌다.
    if _is_stale_payload(content):
        return None

    raw = content.get("window_scores")
    if not raw:
        return None

    scores = np.asarray(raw, dtype=np.float32)

    raw_labels = content.get("window_labels")
    if raw_labels is not None and len(raw_labels) == len(scores):
        labels = np.asarray(raw_labels, dtype=np.int64)
    else:
        # 이상 주입을 끈 실험에서는 라벨이 전부 정상이다.
        labels = np.zeros(len(scores), dtype=np.int64)

    threshold = content.get("threshold")
    try:
        threshold = float(threshold) if threshold is not None else None
    except (TypeError, ValueError):
        threshold = None

    return scores, labels, threshold


def _reload_scores_if_needed() -> None:
    """
    새로운 글로벌 모델이 생성되면:

    1. validation threshold 재계산
    2. test stream reconstruction score 재계산
    3. 기존 score_idx 유지

    기존 코드처럼 score_idx를 0으로 되돌리지 않는다.
    """

    with _shared_lock:
        current_model_round = _shared[
            "model_round"
        ]
        current_fl_round = _shared[
            "round"
        ]

    model_round, model_path = _find_latest_model(
        max_round=current_fl_round,
    )

    with _shared_lock:
        current_scores_round = _shared["scores_round"]
        # 노드별로 마지막에 올린 라운드. 바뀌면 새 점수가 올라온 것이다.
        uploads = tuple(sorted(
            (node, data.get("round", 0))
            for node, data in _shared["nodes"].items()
        ))
        last_uploads = _shared.get("scores_uploads")

    has_new_model = (
        model_round > current_model_round
        and model_path is not None
    )
    # 글로벌 모델이 아직 없어도(Round 1) MN-AE 가 올린 점수는 있다.
    # 라운드 번호만 보면, 라운드 N 의 점수는 노드가 다 올린 뒤에도 N+1 이
    # 시작될 때까지 안 읽혀 그래프가 한 라운드씩 늦었다. 업로드가 바뀌어도 읽는다.
    has_new_round = (current_fl_round > current_scores_round
                     or uploads != last_uploads)

    if not has_new_model and not has_new_round:
        return

    model = None

    if has_new_model:
        print(
            "  [Scores] "
            f"새 글로벌 모델 R{model_round} 감지"
        )
        try:
            model = _load_model(
                model_path,
            )
        except Exception as error:
            print(
                "  ✗ 글로벌 모델 로드 실패: "
                f"{error}"
            )
            model = None

    if model is None and not has_new_round:
        return

    if model is None:
        print(
            "  [Scores] "
            f"R{current_fl_round} — 글로벌 모델 없이 "
            "사업장 MN-CSE 점수로 갱신"
        )

    new_thresholds: dict[str, float] = {}
    new_scores: dict[str, np.ndarray] = {}
    new_labels: dict[str, np.ndarray] = {}

    for node in NODES:
        pkl_path = (
            PKL_DIR
            / f"{node}.pkl"
        )

        # PKL 로 직접 채점하려면 글로벌 모델이 있어야 한다.
        if not pkl_path.exists() or model is None:
            # CSE 모드. 사업장 앱이 MN-CSE 에 쓴 점수를 그대로 쓴다.
            from_cse = _node_scores_from_cse(node)

            if from_cse is None:
                print(
                    f"  ⚠ {node}: PKL 도 CSE 점수도 없다 "
                    f"({pkl_path.name})"
                )
                continue

            cse_scores, cse_labels, cse_threshold = from_cse

            if cse_threshold is None:
                # 임계값을 못 받았으면 받은 점수로 임시 산출한다.
                # 모델이 수렴하기 전 초반 라운드에서 생길 수 있다.
                cse_threshold = float(
                    cse_scores.mean()
                    + config.THRESHOLD_N_SIGMA * cse_scores.std()
                )

            new_scores[node] = cse_scores
            new_labels[node] = cse_labels
            new_thresholds[node] = round(cse_threshold, 4)

            print(
                f"    {node}: n={len(cse_scores)}, "
                f"threshold={cse_threshold:.4f} (CSE 드롭박스)"
            )
            continue

        try:
            with pkl_path.open(
                "rb",
            ) as file:
                dataset = pickle.load(
                    file,
                )
        except Exception as error:
            print(
                f"  ✗ {node} PKL 로드 실패: "
                f"{error}"
            )
            continue

        sequence_length = (
            config.AE_CFG.seq_len
        )
        empty_signals = np.empty(
            (
                0,
                sequence_length,
            ),
            dtype=np.float32,
        )
        empty_labels = np.empty(
            (0,),
            dtype=np.int64,
        )
        validation_signals = dataset.get(
            "val_signals",
            empty_signals,
        )
        validation_labels = dataset.get(
            "val_labels",
            empty_labels,
        )
        test_signals = dataset.get(
            "test_stream_signals",
            empty_signals,
        )
        test_labels = dataset.get(
            "test_stream_labels",
            empty_labels,
        )
        threshold = _compute_threshold(
            model,
            validation_signals,
            validation_labels,
        )
        reconstruction_scores = _compute_scores(
            model,
            test_signals,
        )
        new_thresholds[node] = round(
            threshold,
            4,
        )
        new_scores[node] = (
            reconstruction_scores
        )
        new_labels[node] = test_labels

        print(
            f"    {node}: "
            f"n={len(reconstruction_scores)}, "
            f"threshold={threshold:.4f}"
        )

    if not new_scores:
        # 아무 노드도 점수를 못 줬다. 기존 패널을 지우지 말고 그대로 두고,
        # scores_round 도 올리지 않아 다음 tick 에 다시 시도한다.
        #
        # 다만 모델을 읽은 사실은 여기서 기록해야 한다. 안 하면
        # has_new_model 이 계속 참이라, 점수가 빈 동안 매 tick 마다
        # 213KB 글로벌 모델을 CSE 에서 내려받아 gzip+base64 를 풀고
        # torch 로 적재한다. 그 CPU 때문에 Flask 가 요청을 못 받아
        # 대시보드가 "Connecting..." 에서 멈춘다.
        if model is not None:
            with _shared_lock:
                _shared["model_round"] = model_round

        # 이번 실행 업로드가 아직 없는 동안(라운드 0, 1라운드 학습 중) 매 tick
        # 다시 읽지 않게, 본 상태를 기록해 둔다. 노드가 올리면 uploads 가
        # 바뀌어 그때 다시 읽는다.
        with _shared_lock:
            _shared["scores_round"] = current_fl_round
            _shared["scores_uploads"] = uploads

        print(
            "  ⚠ [Scores] 읽을 점수가 없다 "
            "(PKL 없음 + 드롭박스 비어 있음). 기존 값 유지."
        )
        return

    with _shared_lock:
        # 핵심 수정:
        # 글로벌 모델이 바뀌더라도 기존 스트림 진행 위치를 유지한다.
        current_score_idx = _shared[
            "score_idx"
        ]

        _shared["thresholds"] = (
            new_thresholds
        )

        _shared["scores"] = new_scores
        _shared["labels"] = new_labels

        _shared["score_idx"] = (
            current_score_idx
        )

        if model is not None:
            _shared["model_round"] = (
                model_round
            )

        _shared["scores_round"] = current_fl_round
        _shared["scores_uploads"] = uploads

        _shared["summary_sent"] = False

    _broadcast(
        {
            "type": "threshold",
            "model_round": model_round,
            "thresholds": new_thresholds,
        }
    )

    print(
        f"  ✓ R{model_round} score 재계산 완료"
    )


# ════════════════════════════════════════════════════════
# 최종 Summary 계산
# ════════════════════════════════════════════════════════

def _compute_summary() -> dict | None:
    with _shared_lock:
        scores = {
            key: value.copy()
            for key, value
            in _shared["scores"].items()
        }
        labels = {
            key: value.copy()
            for key, value
            in _shared["labels"].items()
        }
        thresholds = dict(
            _shared["thresholds"],
        )
        model_round = _shared[
            "model_round"
        ]
        max_rounds = _shared[
            "max_rounds"
        ]
        evals = list(
            _shared.get("evals", [])
        )

    if (
        model_round < max_rounds
        or not scores
    ):
        return None

    # 라이브 스트림에는 라벨이 없다. 그래서 AUROC·오경보 같은 수치는
    # 고정 검증셋(valset) 평가 결과에서 가져온다. 라운드가 가장 큰 것을
    # 쓰고, 없으면 None 으로 두고 화면에서 해당 블록을 감춘다.
    global_eval = None
    if evals:
        def _round_of(e: dict) -> int:
            try:
                return int(e.get("round", -1))
            except (TypeError, ValueError):
                return -1

        global_eval = max(evals, key=_round_of)

    result = {
        "type": "summary",
        "model_round": model_round,
        "global_eval": global_eval,
        "nodes": {},
    }

    for node in NODES:
        node_scores = scores.get(
            node,
        )
        node_labels = labels.get(
            node,
        )
        threshold = thresholds.get(
            node,
            1.0,
        )

        if (
            node_scores is None
            or node_labels is None
            or len(node_scores) == 0
        ):
            continue

        try:
            if len(
                np.unique(
                    node_labels,
                )
            ) > 1:
                auroc = float(
                    roc_auc_score(
                        node_labels,
                        node_scores,
                    )
                )
            else:
                auroc = 0.0
        except Exception:
            auroc = 0.0

        onset_index = next(
            (
                int(index)
                for index, label
                in enumerate(node_labels)
                if label == 1
            ),
            None,
        )

        detection_index = None
        consecutive = 0

        for index, score in enumerate(
            node_scores,
        ):
            if score > threshold:
                consecutive += 1

                if consecutive >= K_CONSECUTIVE:
                    detection_index = int(
                        index
                        - K_CONSECUTIVE
                        + 1
                    )
                    break
            else:
                consecutive = 0

        if (
            detection_index is not None
            and onset_index is not None
        ):
            detection_delay = int(
                detection_index
                - onset_index
            )
        else:
            detection_delay = None

        false_alarms = 0
        false_alarm_consecutive = 0

        for score, label in zip(
            node_scores,
            node_labels,
        ):
            if label != 0:
                continue

            if score > threshold:
                false_alarm_consecutive += 1
                if (
                    false_alarm_consecutive
                    >= K_CONSECUTIVE
                ):
                    false_alarms += 1
                    false_alarm_consecutive = 0
            else:
                false_alarm_consecutive = 0

        # K 회 연속 초과가 '시작된' 지점의 개수. verdict.py 와 같은 규칙이라
        # 실제로 CSE 에 올라간 판정 건수와 맞는다.
        detections = 0
        run = 0
        for score in node_scores:
            if score > threshold:
                run += 1
                if run == K_CONSECUTIVE:
                    detections += 1
            else:
                run = 0

        labeled = bool(
            len(
                np.unique(
                    node_labels,
                )
            ) > 1
        )

        entry = {
            "threshold": round(
                float(threshold),
                4,
            ),
            "labeled": labeled,

            # 라벨이 없어도 말할 수 있는 것들
            "n_windows": int(
                len(node_scores)
            ),
            "exceed": int(
                np.sum(
                    node_scores > threshold
                )
            ),
            "detections": detections,
            "mean_score": round(
                float(
                    np.mean(node_scores)
                ),
                4,
            ),
            "max_score": round(
                float(
                    np.max(node_scores)
                ),
                4,
            ),
        }

        # 라벨이 있을 때만 의미가 있는 것들. 라이브 스트림은 전부 정상
        # 라벨이라 AUROC 가 0 으로 찍혔고, 그 0 을 화면에 띄우면 거짓말이
        # 된다. 아예 싣지 않는다.
        if labeled:
            entry.update({
                "auroc": round(
                    auroc,
                    4,
                ),
                "onset_idx": onset_index,
                "detect_idx": detection_index,
                "delay": detection_delay,
                "false_alarms": false_alarms,
                "n_normal": int(
                    np.sum(
                        node_labels == 0
                    )
                ),
                "n_anomaly": int(
                    np.sum(
                        node_labels == 1
                    )
                ),
            })

        result["nodes"][node] = entry

    return result


# ════════════════════════════════════════════════════════
# 표시할 테스트 스트림 인덱스 선택
# ════════════════════════════════════════════════════════

def _select_stream_index(
    node: str,
    node_labels: np.ndarray,
    stream_index: int,
    fl_round: int,
) -> int | None:
    """
    노드와 현재 FL round에 따라 표시할 샘플 인덱스를 선택한다.
    기본 동작:
      MN1:
        정상 샘플 반복
      MN2:
        정상 샘플 반복
      MN3 Round 1~6:
        정상 샘플 반복
      MN3 Round 7 이상:
        이상 샘플 반복
    """

    if len(node_labels) == 0:
        return None

    if node == ANOMALY_NODE:
        if fl_round >= ANOMALY_START_ROUND:
            target_label = 1
        else:
            target_label = 0
    else:
        target_label = 0

    candidate_indices = np.flatnonzero(
        node_labels == target_label
    )

    # 해당 라벨이 없다면 전체 데이터 사용
    if len(candidate_indices) == 0:
        candidate_indices = np.arange(
            len(node_labels),
        )
    if len(candidate_indices) == 0:
        return None

    selected_position = (
        stream_index
        % len(candidate_indices)
    )

    return int(
        candidate_indices[
            selected_position
        ]
    )


# ════════════════════════════════════════════════════════
# Score streaming thread
# ════════════════════════════════════════════════════════

def _score_thread() -> None:
    """
    테스트 reconstruction score를 주기적으로 전송한다.
    score_idx는 글로벌 모델이 변경돼도 초기화하지 않는다.
    """

    while True:
        time.sleep(
            SCORE_INTERVAL,
        )

        with _shared_lock:
            scores = _shared["scores"]
            labels = _shared["labels"]
            thresholds = _shared["thresholds"]
            stream_index = _shared["score_idx"]
            fl_state = _shared["fl_state"]
            fl_round = _shared["round"]

        if (
            not scores
            or fl_state == "FL_COMPLETED"
        ):
            continue

        event: dict = {
            "type": "score",
            "round": fl_round,
            "anomaly_active": (
                fl_round
                >= ANOMALY_START_ROUND
            ),
        }

        for node in NODES:
            node_scores = scores.get(
                node,
            )
            node_labels = labels.get(
                node,
            )

            if (
                node_scores is None
                or node_labels is None
                or len(node_scores) == 0
                or len(node_labels) == 0
            ):
                continue

            source_index = _select_stream_index(
                node=node,
                node_labels=node_labels,
                stream_index=stream_index,
                fl_round=fl_round,
            )

            if source_index is None:
                continue

            if source_index >= len(node_scores):
                continue

            event[node] = round(
                float(
                    node_scores[source_index]
                ),
                5,
            )
            event[f"{node}_label"] = int(
                node_labels[source_index]
            )
            event[f"{node}_thr"] = (
                thresholds.get(
                    node,
                    1.0,
                )
            )

        # type, round, anomaly_active 이외에
        # 실제 노드 데이터가 하나 이상 들어간 경우만 전송
        if len(event) > 3:
            with _shared_lock:
                _shared["score_idx"] = (
                    stream_index + 1
                )
            _broadcast(
                event,
            )


# ════════════════════════════════════════════════════════
# oneM2M CIN 파싱
# ════════════════════════════════════════════════════════

def _parse_content_instance(
    content_instance: dict | None,
) -> dict | None:
    if (
        not content_instance
        or "m2m:cin" not in content_instance
    ):
        return None

    content = (
        content_instance["m2m:cin"]
        .get("con")
    )

    try:
        if isinstance(
            content,
            str,
        ):
            return json.loads(
                content,
            )
        if isinstance(
            content,
            dict,
        ):
            return content

    except (
        TypeError,
        ValueError,
        json.JSONDecodeError,
    ):
        return None

    return None


# ════════════════════════════════════════════════════════
# FL 상태 polling thread
# ════════════════════════════════════════════════════════

DASHBOARD_STARTED_AT = time.time()
IGNORE_STALE_CONTROL = os.getenv("FL_DASH_IGNORE_STALE", "1") != "0"
_stale_control_warned = False


def _is_stale_payload(content: dict) -> bool:
    """
    지난 실행이 남긴 CIN 인지 본다. _is_stale_control 과 기준은 같고
    경고를 찍지 않는다. 평가 결과처럼 조용히 걸러야 하는 곳에서 쓴다.
    """
    if not IGNORE_STALE_CONTROL:
        return False

    try:
        ts = float(content.get("timestamp", 0))
    except (TypeError, ValueError):
        return False

    return 0 < ts < DASHBOARD_STARTED_AT


def _is_stale_control(content: dict) -> bool:
    """
    지난 실행이 남긴 라운드 명령인지 본다.

    cnt-fl-control 은 라운드마다 CIN 을 덧붙이기만 하고 비우지 않는다.
    그래서 새로 띄워도 IN-AE 가 첫 명령을 올리기 전까지는 지난 판의
    마지막 상태가 그대로 보인다. "대시보드가 2라운드부터 시작"하는 것이
    이것이다. run_fl.sh 에서 대시보드는 [2/5], IN-AE 는 [5/5] 라
    그 틈이 수십 초다.

    MN-AE 가 묵은 명령을 거를 때와 같은 기준을 쓴다. payload 의
    timestamp 가 이 대시보드 기동 시각보다 이르면 지난 실행 것이다.
    FL_DASH_IGNORE_STALE=0 으로 끄면 예전처럼 그대로 보여준다.
    """
    global _stale_control_warned

    if not IGNORE_STALE_CONTROL:
        return False

    try:
        ts = float(content.get("timestamp", 0))
    except (TypeError, ValueError):
        return False

    if ts <= 0 or ts >= DASHBOARD_STARTED_AT:
        return False

    if not _stale_control_warned:
        _stale_control_warned = True
        print(
            f"  ⓘ 지난 실행의 라운드 상태를 무시한다 "
            f"(R{content.get('currentRound')} "
            f"{content.get('jobState')}). "
            f"IN-AE 가 새 명령을 올릴 때까지 대기한다."
        )

    return True


def _control_thread() -> None:
    """
    라운드 상태만 본다. 바뀌면 바로 화면에 보낸다.

    jobState / currentRound / maxRounds 는 이 스레드만 고친다. _poll_thread 가
    고치면, 그쪽이 느린 한 바퀴 앞부분에서 읽은 옛 라운드로 덮어써 화면이
    한 라운드 뒤로 갔다가 돌아올 수 있다.
    """
    fl_control_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-control"
    )
    last = None

    while True:
        try:
            content = _parse_content_instance(
                om2m.get_latest_content_instance(fl_control_path)
            )
            if content and not _is_stale_control(content):
                state = (
                    content.get("jobState", "FL_READY"),
                    int(content.get("currentRound", 0)),
                    int(content.get("maxRounds", config.GLOBAL_ROUNDS)),
                )
                if state != last:
                    last = state
                    with _shared_lock:
                        _shared.update(
                            {
                                "fl_state": state[0],
                                "round": state[1],
                                "max_rounds": state[2],
                            }
                        )
                        nodes = dict(_shared["nodes"])
                    _broadcast(
                        {
                            "type": "round",
                            "fl_state": state[0],
                            "round": state[1],
                            "max_rounds": state[2],
                            "nodes": nodes,
                        }
                    )
                    print(f"  [Round] R{state[1]}/{state[2]} {state[0]}")
        except Exception as error:
            print(f"  ⚠ 라운드 상태 조회 오류: {error}")

        time.sleep(CONTROL_POLL_INTERVAL)


def _poll_thread() -> None:
    # IN-AE 의 컨테이너는 중앙 IN-CSE 에 있다. config.CSE_NAME 은 사업장
    # MN-CSE 이름이라, 그걸로 경로를 만들면 없는 리소스를 찾게 된다.
    fl_control_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-control"
    )
    local_updates_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-local-updates"
    )
    # IN-AE 가 라운드마다 고정 검증셋으로 잰 결과를 올리는 곳.
    fl_inference_path = om2m.in_path(
        f"{config.IN_AE_NAME}/cnt-fl-inference"
    )

    previous_snapshot: dict = {}
    last_eval_round = -1

    while True:
        try:
            control_content = (
                _parse_content_instance(
                    om2m.get_latest_content_instance(
                        fl_control_path,
                    )
                )
            )

            if control_content and _is_stale_control(control_content):
                # 지난 실행의 잔상. 새 명령이 올 때까지 아무것도 안 바꾼다.
                control_content = None

            if control_content:
                fl_state = control_content.get(
                    "jobState",
                    "FL_READY",
                )
                round_number = int(
                    control_content.get(
                        "currentRound",
                        0,
                    )
                )
                max_rounds = int(
                    control_content.get(
                        "maxRounds",
                        config.GLOBAL_ROUNDS,
                    )
                )

                nodes: dict = {}

                for node in NODES:
                    node_content = (
                        _parse_content_instance(
                            om2m.get_latest_content_instance(
                                (
                                    f"{local_updates_path}/"
                                    f"cnt-{node}"
                                )
                            )
                        )
                    )

                    if not node_content:
                        continue

                    # 지난 실행이 남긴 업로드(이 대시보드 기동 전)는 이번 판 것이 아니다.
                    if _is_stale_payload(node_content):
                        continue

                    nodes[node] = {
                        "train_loss": round(
                            float(
                                node_content.get(
                                    "train_loss",
                                    0,
                                )
                            ),
                            5,
                        ),
                        "val_loss": round(
                            float(
                                node_content.get(
                                    "val_loss",
                                    0,
                                )
                            ),
                            5,
                        ),
                        "val_auroc": round(
                            float(
                                node_content.get(
                                    "val_auroc",
                                    0,
                                )
                            ),
                            4,
                        ),
                        "num_samples": int(
                            node_content.get(
                                "num_samples",
                                0,
                            )
                        ),
                        "round": int(
                            node_content.get(
                                "round",
                                0,
                            )
                        ),
                        # 사업장 앱이 지금 붙어 있는 MN-CSE 와 핸드오버 횟수.
                        # 0 보다 크면 화면에 spare 표시를 띄운다.
                        "mn_cse": node_content.get("mn_cse"),
                        "mn_base": node_content.get("mn_base"),
                        "cse_handovers": int(
                            node_content.get(
                                "cse_handovers",
                                0,
                            ) or 0
                        ),
                    }

                # 라운드 상태는 _control_thread 가 맡는다. 여기서는 노드
                # 업로드가 바뀌었을 때만 보낸다.
                snapshot = {
                    "node_keys": sorted(
                        nodes.keys(),
                    ),
                    "rounds": {
                        node: data["round"]
                        for node, data
                        in nodes.items()
                    },

                    "losses": {
                        node: data[
                            "train_loss"
                        ]
                        for node, data
                        in nodes.items()
                    },
                }

                if snapshot != previous_snapshot:
                    previous_snapshot = snapshot

                    with _shared_lock:
                        _shared["nodes"] = nodes
                        cur_state = _shared["fl_state"]
                        cur_round = _shared["round"]
                        cur_max = _shared["max_rounds"]

                    _broadcast(
                        {
                            "type": "round",
                            "fl_state": cur_state,
                            "round": cur_round,
                            "max_rounds": cur_max,
                            "nodes": nodes,
                        }
                    )
                    print(
                        "  [Poll] "
                        f"R{cur_round}/{cur_max} "
                        f"{cur_state} "
                        f"nodes={ {n: d['round'] for n, d in nodes.items()} }"
                    )

            # 라운드별 평가 결과. 새 라운드 것만 한 번씩 내보낸다.
            ev = _parse_content_instance(
                om2m.get_latest_content_instance(fl_inference_path)
            )
            if (ev and ev.get("type") == "global-eval"
                    and int(ev.get("round", -1)) > last_eval_round
                    and not _is_stale_payload(ev)):
                last_eval_round = int(ev["round"])
                ev = dict(ev)
                ev["type"] = "eval"
                # 접속 중인 쪽에 보내고, 나중에 접속한 쪽도 지난 라운드를
                # 볼 수 있게 쌓아 둔다. 새로고침하면 그동안의 라운드가
                # 통째로 사라지던 것이 이것이다.
                with _shared_lock:
                    _shared["evals"].append(ev)
                    del _shared["evals"][:-EVAL_HISTORY_MAX]
                _broadcast(ev)
                print(f"  [Eval] R{last_eval_round} "
                      f"AUROC={ev.get('auroc')} "
                      f"탐지={ev.get('detected')}/{ev.get('n_anomaly')} "
                      f"오경보={ev.get('false_alarms')}/{ev.get('n_normal')}")

        except Exception as error:
            print(
                "  ⚠ oneM2M poll 오류: "
                f"{error}"
            )

        # 새로운 글로벌 모델이 있는지 확인
        _reload_scores_if_needed()

        # FL 완료 후 최종 모델까지 준비되면
        # summary를 한 번만 전송한다.
        with _shared_lock:
            fl_completed = (
                _shared["fl_state"]
                == "FL_COMPLETED"
            )
            summary_already_sent = (
                _shared["summary_sent"]
            )
            final_model_ready = (
                _shared["model_round"]
                >= _shared["max_rounds"]
            )

        if (
            fl_completed
            and final_model_ready
            and not summary_already_sent
        ):
            summary = _compute_summary()

            if summary:
                with _shared_lock:
                    _shared["summary_sent"] = True

                _broadcast(
                    summary,
                )
                print(
                    "  ✓ Summary 전송 완료 "
                    f"(R{summary['model_round']})"
                )
        time.sleep(
            POLL_INTERVAL,
        )


# ════════════════════════════════════════════════════════
# Main
# ════════════════════════════════════════════════════════

def main() -> None:
    # 대시보드는 양쪽을 다 본다.
    #   사업장 MN-CSE  윈도우별 이상 점수 (사업장마다 바꿔 가며 읽는다)
    #   중앙 IN-CSE    라운드 상태 / 사업장별 요약 / 공통 모델 / 라운드별 평가
    config.require_sandbox()
    om2m.set_endpoint(*config.mn_endpoint("mn1"))
    om2m.set_in_endpoint(*config.in_endpoint())

    print()
    print("=== FL Dashboard Server ===")
    print("  사업장 CSE: " + ", ".join(
        f"{n} {config.mn_endpoint(n)[1]}" for n in NODES))
    print(f"  중앙 CSE  : {config.in_endpoint()[0]}/{om2m.in_cse_name()}")
    print(f"  PKL: {PKL_DIR}")
    print(
        "  이상 표시: "
        f"{ANOMALY_NODE}, "
        f"Round {ANOMALY_START_ROUND}부터"
    )
    print(f"  Port: {PORT}")
    print(f"  http://localhost:{PORT}")
    print()

    polling_thread = threading.Thread(
        target=_poll_thread,
        daemon=True,
        name="fl-poll",
    )

    control_thread = threading.Thread(
        target=_control_thread,
        daemon=True,
        name="fl-control",
    )

    score_thread = threading.Thread(
        target=_score_thread,
        daemon=True,
        name="score-stream",
    )

    polling_thread.start()
    control_thread.start()
    score_thread.start()

    app.run(
        host="0.0.0.0",
        port=PORT,
        threaded=True,
        use_reloader=False,
    )


if __name__ == "__main__":
    main()