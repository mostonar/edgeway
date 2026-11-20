"""
Cloud/FL-FedAvg/valset.py — 고정 검증셋으로 글로벌 모델 성능을 잰다.

[왜 고정셋인가]
지금까지 성능 수치(val_loss, val_auroc)는 그 라운드에 받은 윈도우로 계산했다.
라운드마다 데이터가 바뀌니 R3 의 AUROC 와 R9 의 AUROC 를 비교할 수 없었다.
라벨도 라이브 스트림에는 안 붙어 있어 늘 단일 클래스 -> AUROC 0.5 였다.

여기서는 라벨이 붙은 고정 윈도우 묶음(Cloud/FL-FedAvg/valset_amr.npz)을 매 라운드 같은
자로 재서, "라운드가 갈수록 좋아지는가"를 수치로 남긴다.

[라벨을 어디에 쓰나]
채점에만 쓴다. 모델은 추론 때 라벨을 보지 않고, 정규화 통계와 임계값도
라벨 없이 검증셋 전체에서 뽑는다. 그래서 전 구간이 비지도다.

[정규화]
글로벌 모델에는 정규화 통계가 없다(사업장마다 따로 갖는다). 그래서 검증셋
자체의 채널별 평균/표준편차로 표준화한다.

학습 때 쓴 자와 다르다. 전에 여기 "AUROC 는 순위 기반이라 영향받지 않는다"
고 적혀 있었는데 틀렸다. 순위가 매겨지는 대상은 점수이고, 정규화는 점수를
내기 전에 입력을 바꾸므로 순위 자체가 바뀐다.

실측(한 노드, 40에폭, 시드 4개): 평가도 학습과 같은 통계로 정규화하면
AUROC 0.7146 ± 0.0197, 검증셋 자기 통계로 하면 0.6852 ± 0.0166. 약 0.03
손해이고 시드 4개 모두 같은 방향이었다.

고치려면 글로벌 모델과 함께 정규화 통계를 실어 보내야 하는데, 사업장마다
통계가 다르므로 "어느 사업장의 자로 잴 것인가"를 먼저 정해야 한다. 지금은
그대로 두고 이 차이를 알고 읽는다. 절대 오차값을 사업장 임계값과 직접
비교하지는 말 것.

[파일 형식]
  signals (N, 38, 60) float32   윈도우
  cond    (N, 8)      float32   조건 벡터
  labels  (N,)        int64     0 정상 / 1 이상
  (선택) robot, scn, cond_name, run_id
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

import numpy as np
import torch

import amr_schema as _sch
import config
from model_cvae import CVAE1D

DEFAULT_PATH = str(Path(__file__).resolve().parent / "valset_amr.npz")
VALSET_PATH = os.getenv("FL_VALSET_PATH", DEFAULT_PATH)

_cache = None


def load(path: Optional[str] = None):
    """검증셋을 한 번만 읽어 캐시한다. 없으면 None."""
    global _cache
    if _cache is not None:
        return _cache

    p = path or VALSET_PATH
    if not os.path.exists(p):
        return None

    try:
        z = np.load(p, allow_pickle=False)
        X = z["signals"].astype(np.float32)
        C = z["cond"].astype(np.float32)
        Y = z["labels"].astype(np.int64)
    except Exception as e:
        print(f"  ⚠ 검증셋 로드 실패 ({p}): {e}")
        return None

    # 조건 폭이 모자라면 채운다.
    #
    # 적재 상태(empty/light/heavy)를 cond 에 넣으면서 폭이 8 -> 11 이 됐다.
    # 예전에 뜬 검증셋은 8 폭이라 그대로 넣으면 모델이 모양 때문에 죽고,
    # 평가가 통째로 건너뛰어진다(라운드별 AUROC 가 영영 안 뜬다).
    #
    # 모자란 자리는 'empty' 로 채운다. 이 검증셋은 전부 to_pick 구간이고
    # lift.eff 가 4561 에 붙어 있어 실제로 빈 차 주행이다. 추정이 아니라
    # 확인한 값이다.
    want = int(getattr(config, "AE_CFG", None).cond_dim)
    if C.shape[1] < want:
        pad = np.zeros((len(C), want - C.shape[1]), dtype=np.float32)
        if pad.shape[1] >= 1:
            pad[:, 0] = 1.0            # LOAD_STATES[0] == "empty"
        C = np.concatenate([C, pad], axis=1)
        print(f"  · 검증셋 조건 폭을 {C.shape[1] - pad.shape[1]} -> {want} 로 "
              f"채웠다 (빈 차 기준). 새로 뜨면 이 줄은 사라진다.")
    elif C.shape[1] > want:
        print(f"  ⚠ 검증셋 조건 폭 {C.shape[1]} > 모델 {want}. 앞쪽만 쓴다.")
        C = C[:, :want]

    if len(X) == 0 or len(np.unique(Y)) < 2:
        print(f"  ⚠ 검증셋이 비었거나 한 클래스뿐이다 ({p})")
        return None

    # 검증셋 자체 통계로 표준화. 라벨은 쓰지 않는다.
    mean = X.mean(axis=(0, 2), keepdims=True)
    std = X.std(axis=(0, 2), keepdims=True) + 1e-8
    Xn = (X - mean) / std

    # 이상 종류. 종류마다 난이도가 크게 달라서(실측: 바닥 0.95 /
    # 휠 돌기 0.69 / 베어링 0.55) 합친 숫자 하나만 보면 어느 쪽이 안
    # 되는지 안 보인다. 없으면 None 으로 두고 전체 숫자만 낸다.
    try:
        ft = z["fault_type"].astype(str) if "fault_type" in z.files else None
    except Exception:
        ft = None

    _cache = {
        "X": torch.from_numpy(Xn),
        "C": torch.from_numpy(C),
        "y": Y,
        "fault_type": ft,
        "path": p,
    }
    print(f"  ✓ 검증셋 {len(Y)}개 "
          f"(정상 {int((Y == 0).sum())} / 이상 {int((Y == 1).sum())}) "
          f"<- {os.path.basename(p)}")
    return _cache


# 점수 방식. edge_node 와 같은 환경 변수를 본다. 두 쪽이 다른 자로
# 재면 라운드별 숫자와 운영 판정이 어긋난다.
SCORE_MODE = os.getenv("FL_SCORE_MODE", "diff").strip().lower()


def _cell_key(C: np.ndarray) -> np.ndarray:
    """cond 원핫(nav 5 + speed 3 + 적재 3) -> 조건칸 번호."""
    C = np.asarray(C, dtype=np.float32)
    if C.ndim != 2 or C.shape[1] < 8:
        return np.zeros(len(C), dtype=np.int64)
    k = np.argmax(C[:, :5], 1) * 9 + np.argmax(C[:, 5:8], 1) * 3
    if C.shape[1] >= 11:
        k = k + np.argmax(C[:, 8:11], 1)
    return k.astype(np.int64)


def _cell_zmax(D: np.ndarray, C: np.ndarray, min_cell: int = 8) -> np.ndarray:
    """채널별 값 (N, C) -> 조건칸별 z 화 -> 채널 최대 (N,).

    기준 계산은 amr_schema 와 공유한다. 학습 쪽과 다른 자를 쓰면
    라운드별 숫자와 운영 판정이 어긋난다.
    """
    keys = _cell_key(C)
    ref, glob, alive = _sch.robust_ref(D, keys, min_cell=min_cell)
    return _sch.apply_ref(D, keys, ref, glob, alive).max(axis=1)


def _auroc(scores: np.ndarray, labels: np.ndarray) -> float:
    if len(np.unique(labels)) < 2:
        return 0.5
    try:
        from sklearn.metrics import roc_auc_score
        return float(roc_auc_score(labels, scores))
    except Exception:
        # sklearn 이 없으면 순위로 직접 센다 (Mann-Whitney U).
        order = np.argsort(scores, kind="stable")
        ranks = np.empty(len(scores), dtype=np.float64)
        ranks[order] = np.arange(1, len(scores) + 1)
        pos, neg = labels == 1, labels == 0
        n1, n0 = int(pos.sum()), int(neg.sum())
        if n1 == 0 or n0 == 0:
            return 0.5
        return float((ranks[pos].sum() - n1 * (n1 + 1) / 2) / (n1 * n0))


# "모델 없음" 기준선. 모델이 아무것도 복원하지 못해 출력이 0 일 때의
# 점수다. 입력 자체에 이미 들어 있는 신호(이상 창의 에너지나 떨림이 더
# 크다)만으로 나오는 숫자라, 글로벌 모델의 AUROC 가 이것을 넘어야
# 연합학습이 무언가를 배웠다고 말할 수 있다.
#
# 2026-11-19 실행의 R1~R10 AUROC(0.638~0.641, bearing 0.53 / floor 0.93 /
# wknob 0.63)는 recon 방식의 이 기준선(0.641 / 0.535 / 0.933 / 0.631)과
# 소수 셋째 자리까지 같았다. diff 방식의 기준선은 약 0.832 다.
_baseline_cache: dict = {}


def _scores_no_model(X: torch.Tensor, C: torch.Tensor) -> np.ndarray:
    """출력 0 인 모델의 점수. evaluate() 의 점수 식과 같은 자를 쓴다."""
    Xn = X.cpu().numpy()
    if SCORE_MODE == "diff":
        D = np.abs(np.diff(Xn, axis=2)).mean(axis=2)
        return _cell_zmax(D, C.cpu().numpy())
    E = (Xn ** 2).mean(axis=2)
    E = E / (E.mean(axis=0) + 1e-12)
    return E.mean(axis=1)


def _auroc_block(scores: np.ndarray, y: np.ndarray, ft) -> dict:
    out = {"auroc": round(_auroc(scores, y), 4), "by_fault": {}}
    if ft is not None:
        for name in sorted(set(ft[y == 1].tolist())):
            m = (ft == name) | (y == 0)
            out["by_fault"][name] = round(
                _auroc(scores[m], (ft[m] == name).astype(int)), 4)
    return out


def baseline(ds=None) -> Optional[dict]:
    """검증셋의 '모델 없음' 기준선 {auroc, by_fault}. 한 번만 계산한다."""
    ds = ds or load()
    if ds is None:
        return None
    key = (ds.get("path"), SCORE_MODE)
    if key not in _baseline_cache:
        s = _scores_no_model(ds["X"], ds["C"])
        _baseline_cache[key] = _auroc_block(s, ds["y"], ds.get("fault_type"))
    return _baseline_cache[key]


def evaluate(model_path: str, n_sigma: float = None) -> Optional[dict]:
    """
    글로벌 모델 하나를 검증셋으로 잰다.

    반환: {auroc, threshold, detected, false_alarms, miss,
           mean_normal, mean_anomaly, separation, n_normal, n_anomaly}
    """
    ds = load()
    if ds is None:
        return None

    if not model_path or not os.path.exists(model_path):
        return None

    cfg = config.AE_CFG
    n_sigma = (config.THRESHOLD_N_SIGMA if n_sigma is None else n_sigma)

    try:
        model = CVAE1D(
            n_channels=cfg.n_channels,
            cond_dim=cfg.cond_dim,
            latent_dim=cfg.latent_dim,
            hidden=int(getattr(cfg, "hidden", 48)),
            seq_len=cfg.seq_len,
        )
        model.load_state_dict(torch.load(model_path, map_location="cpu"))
        model.eval()
    except Exception as e:
        print(f"  ⚠ 검증용 모델 로드 실패: {e}")
        return None

    X, C, y = ds["X"], ds["C"], ds["y"]

    with torch.no_grad():
        if SCORE_MODE == "diff":
            # 차분 오차. 채널 x 조건칸 마다 '그 칸의 중앙값/MAD' 로 z 화한
            # 뒤 채널 최대를 쓴다. 기준은 검증셋 자체에서 뽑는다. 여기에는
            # 학습 노드가 없어서 학습 데이터 기준을 쓸 수 없고, 이상이
            # 소수라 중앙값/MAD 는 거의 움직이지 않는다. 라벨은 안 쓴다.
            D = model.channel_diff_error(
                X, C if cfg.cond_dim else None).cpu().numpy()
            scores = _cell_zmax(D, C.cpu().numpy())
        else:
            E = model.channel_error(X, C if cfg.cond_dim else None)
            # 채널별 규모가 달라 그냥 평균내면 큰 채널이 점수를 지배한다.
            # 기준은 검증셋 전체의 채널 평균이다. 라벨을 쓰지 않는다.
            E = E / (E.mean(dim=0) + 1e-12)
            scores = E.mean(dim=1).cpu().numpy()

    # 임계값도 라벨 없이 전체에서 뽑는다. 다만 평균/표준편차가 아니라
    # 중앙값/MAD 를 쓴다.
    #
    # 검증셋에는 정상과 이상이 섞여 있다. 평균과 표준편차는 이상치에 끌려
    # 올라가므로 임계값이 과하게 높아진다. 실제로 그 때문에 오경보는 0 인데
    # 탐지가 8% 에 그쳤다. 중앙값과 MAD 는 이상치가 섞여도 거의 움직이지
    # 않아 정상 분포를 그대로 반영한다.
    #
    # 1.4826 은 정규분포에서 MAD 를 표준편차와 같은 자로 맞추는 상수다.
    # 그래서 n_sigma 의 의미(3σ)가 그대로 유지된다.
    med = float(np.median(scores))
    mad = float(np.median(np.abs(scores - med))) * 1.4826
    if mad <= 0:
        mad = float(scores.std())      # 점수가 거의 같으면 MAD 가 0 이 된다
    thr = med + n_sigma * mad

    # 예전 기준도 같이 남긴다. 둘을 비교해야 임계값 때문인지 모델 때문인지
    # 가릴 수 있다.
    thr_mean = float(scores.mean() + n_sigma * scores.std())

    norm, anom = scores[y == 0], scores[y == 1]

    # 종류별. 각 종류를 '정상' 하고만 비교한다. 종류끼리 섞으면 어려운
    # 종류가 쉬운 종류의 숫자를 깎는다.
    by_fault = {}
    ft = ds.get("fault_type")
    if ft is not None:
        for name in sorted(set(ft[y == 1].tolist())):
            m = (ft == name) | (y == 0)
            by_fault[name] = {
                "auroc": round(_auroc(scores[m], (ft[m] == name).astype(int)), 4),
                "detected": int((scores[ft == name] > thr).sum()),
                "n": int((ft == name).sum()),
            }

    base = baseline(ds) or {}
    auroc = round(_auroc(scores, y), 4)
    for name, b in (base.get("by_fault") or {}).items():
        if name in by_fault:
            by_fault[name]["baseline_auroc"] = b

    return {
        "by_fault": by_fault,
        "auroc": auroc,
        "baseline_auroc": base.get("auroc"),
        "delta_vs_baseline": (round(auroc - base["auroc"], 4)
                              if base.get("auroc") is not None else None),
        "threshold": round(thr, 4),
        "threshold_mean3s": round(thr_mean, 4),
        "detected_mean3s": int((anom > thr_mean).sum()),
        "false_alarms_mean3s": int((norm > thr_mean).sum()),
        "detected": int((anom > thr).sum()),
        "miss": int((anom <= thr).sum()),
        "false_alarms": int((norm > thr).sum()),
        "mean_normal": round(float(norm.mean()), 4),
        "mean_anomaly": round(float(anom.mean()), 4),
        "separation": round(float(anom.mean() - norm.mean()), 4),
        "n_normal": int(len(norm)),
        "n_anomaly": int(len(anom)),
    }


def summary_line(m: dict) -> str:
    """로그 한 줄. 종류별 숫자가 있으면 뒤에 붙인다."""
    tail = ""
    if m.get("by_fault"):
        tail = "  [" + " ".join(
            f"{k} {v['auroc']:.3f}({v['detected']}/{v['n']})"
            for k, v in sorted(m["by_fault"].items())) + "]"
    base = ""
    if m.get("baseline_auroc") is not None:
        base = (f"  | 모델없음 {m['baseline_auroc']:.4f} "
                f"(Δ{m['delta_vs_baseline']:+.4f})")
    return (f"AUROC {m['auroc']:.4f}  "
            f"탐지 {m['detected']}/{m['n_anomaly']}  "
            f"오경보 {m['false_alarms']}/{m['n_normal']}  "
            f"정상 {m['mean_normal']:.3f} vs 이상 {m['mean_anomaly']:.3f}  "
            f"thr {m['threshold']:.3f}"
            f"(평균+{config.THRESHOLD_N_SIGMA:g}σ {m['threshold_mean3s']:.3f} -> "
            f"{m['detected_mean3s']}/{m['false_alarms_mean3s']})" + tail + base)
