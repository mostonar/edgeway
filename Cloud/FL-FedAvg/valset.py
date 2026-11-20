"""
Cloud/FL-FedAvg/valset.py — measures global model performance on a fixed validation set.

[Why a fixed set]
Until now performance figures (val_loss, val_auroc) were computed on the windows received that round.
The data changed every round, so the AUROC of R3 and R9 could not be compared.
Labels were not attached in the live stream either, so it was always a single class -> AUROC 0.5.

Here a fixed labeled bundle of windows (Cloud/FL-FedAvg/valset_amr.npz) is measured every round with the same
ruler, recording numerically "does it get better as rounds go on".

[Where labels are used]
Only for scoring. The model does not see labels at inference, and the normalization statistics and threshold
are drawn from the whole validation set without labels. So the whole pipeline is unsupervised.

[Normalization]
The global model has no normalization statistics (each site keeps its own). So the
validation set is standardized with its own per-channel mean/std.

This is a different ruler from training. This used to say "AUROC is rank-based so it is unaffected",
which was wrong. What gets ranked is the score, and normalization changes the input before
the score is produced, so the ranking itself changes.

Measured (one node, 40 epochs, 4 seeds): normalizing evaluation with the same stats as training gives
AUROC 0.7146 ± 0.0197; using the validation set's own stats gives 0.6852 ± 0.0166. About 0.03
worse, and all 4 seeds went the same direction.

Fixing it requires shipping normalization statistics with the global model, but each site has
different statistics, so "whose ruler to measure with" must be decided first. For now
it is left as is and read with this gap in mind. Do not compare absolute error values directly
against site thresholds.

[File format]
  signals (N, 38, 60) float32   windows
  cond    (N, 8)      float32   condition vector
  labels  (N,)        int64     0 normal / 1 anomaly
  (optional) robot, scn, cond_name, run_id
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
    """Read the validation set once and cache it. None if absent."""
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
        print(f"  ⚠ valset load failed ({p}): {e}")
        return None

    # Pad the condition width if it is short.
    #
    # Adding the load state (empty/light/heavy) to cond widened it from 8 -> 11.
    # Validation sets built earlier are 8 wide, so feeding them in as is crashes the model on shape,
    # and evaluation is skipped entirely (per-round AUROC never shows up).
    #
    # The missing slots are filled with 'empty'. This validation set is all to_pick segments
    # with lift.eff stuck at 4561, so it really is empty driving. This is a checked value,
    # not a guess.
    want = int(getattr(config, "AE_CFG", None).cond_dim)
    if C.shape[1] < want:
        pad = np.zeros((len(C), want - C.shape[1]), dtype=np.float32)
        if pad.shape[1] >= 1:
            pad[:, 0] = 1.0            # LOAD_STATES[0] == "empty"
        C = np.concatenate([C, pad], axis=1)
        print(f"  · valset cond width padded {C.shape[1] - pad.shape[1]} -> {want} "
              f"(as empty load). Regenerate the valset to drop this line.")
    elif C.shape[1] > want:
        print(f"  ⚠ valset cond width {C.shape[1]} > model {want}. Using leading columns only.")
        C = C[:, :want]

    if len(X) == 0 or len(np.unique(Y)) < 2:
        print(f"  ⚠ valset is empty or has only one class ({p})")
        return None

    # Standardize with the validation set's own statistics. Labels are not used.
    mean = X.mean(axis=(0, 2), keepdims=True)
    std = X.std(axis=(0, 2), keepdims=True) + 1e-8
    Xn = (X - mean) / std

    # Anomaly type. Difficulty varies a lot by type (measured: floor 0.95 /
    # wheel knob 0.69 / bearing 0.55), so a single combined number hides which one is
    # failing. If absent, leave None and report only the overall number.
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
    print(f"  ✓ valset {len(Y)} windows "
          f"(normal {int((Y == 0).sum())} / anomaly {int((Y == 1).sum())}) "
          f"<- {os.path.basename(p)}")
    return _cache


# Scoring mode. Reads the same environment variable as edge_node. If the two sides measure
# with different rulers, per-round numbers and operational verdicts disagree.
SCORE_MODE = os.getenv("FL_SCORE_MODE", "diff").strip().lower()


def _cell_key(C: np.ndarray) -> np.ndarray:
    """cond one-hot (nav 5 + speed 3 + load 3) -> condition cell index."""
    C = np.asarray(C, dtype=np.float32)
    if C.ndim != 2 or C.shape[1] < 8:
        return np.zeros(len(C), dtype=np.int64)
    k = np.argmax(C[:, :5], 1) * 9 + np.argmax(C[:, 5:8], 1) * 3
    if C.shape[1] >= 11:
        k = k + np.argmax(C[:, 8:11], 1)
    return k.astype(np.int64)


def _cell_zmax(D: np.ndarray, C: np.ndarray, min_cell: int = 8) -> np.ndarray:
    """Per-channel values (N, C) -> z-score per condition cell -> max over channels (N,).

    The reference computation is shared with amr_schema. Using a different ruler from the training side
    makes per-round numbers and operational verdicts disagree.
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
        # Without sklearn, count ranks directly (Mann-Whitney U).
        order = np.argsort(scores, kind="stable")
        ranks = np.empty(len(scores), dtype=np.float64)
        ranks[order] = np.arange(1, len(scores) + 1)
        pos, neg = labels == 1, labels == 0
        n1, n0 = int(pos.sum()), int(neg.sum())
        if n1 == 0 or n0 == 0:
            return 0.5
        return float((ranks[pos].sum() - n1 * (n1 + 1) / 2) / (n1 * n0))


# "No model" baseline. The score when the model reconstructs nothing and outputs 0.
# This number comes only from signal already present in the input (anomalous windows have more
# energy or jitter), so the global model's AUROC must beat it before one can say
# federated learning learned anything.
#
# The 2026-11-19 run's R1~R10 AUROC (0.638~0.641, bearing 0.53 / floor 0.93 /
# wknob 0.63) matched this baseline for the recon mode (0.641 / 0.535 / 0.933 / 0.631)
# to three decimal places. The baseline for the diff mode is about 0.832.
_baseline_cache: dict = {}


def _scores_no_model(X: torch.Tensor, C: torch.Tensor) -> np.ndarray:
    """Score of a model that outputs 0. Uses the same ruler as the score formula in evaluate()."""
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
    """The validation set's 'no model' baseline {auroc, by_fault}. Computed only once."""
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
    Measure one global model on the validation set.

    Returns: {auroc, threshold, detected, false_alarms, miss,
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
        print(f"  ⚠ validation model load failed: {e}")
        return None

    X, C, y = ds["X"], ds["C"], ds["y"]

    with torch.no_grad():
        if SCORE_MODE == "diff":
            # Differenced error. For each channel x condition cell, z-score with 'that cell's median/MAD'
            # and then take the max over channels. The reference comes from the validation set itself. There are
            # no training nodes here, so a training-data reference cannot be used, and since anomalies are
            # a minority the median/MAD barely move. Labels are not used.
            D = model.channel_diff_error(
                X, C if cfg.cond_dim else None).cpu().numpy()
            scores = _cell_zmax(D, C.cpu().numpy())
        else:
            E = model.channel_error(X, C if cfg.cond_dim else None)
            # Channel scales differ, so a plain average lets large channels dominate the score.
            # The reference is the per-channel mean over the whole validation set. Labels are not used.
            E = E / (E.mean(dim=0) + 1e-12)
            scores = E.mean(dim=1).cpu().numpy()

    # The threshold is also drawn from the whole set without labels. But it uses median/MAD
    # rather than mean/std.
    #
    # The validation set mixes normal and anomalous windows. Mean and std get pulled up by outliers,
    # so the threshold ends up too high. That is exactly why false alarms were 0 while
    # detection was only 8%. Median and MAD barely move even with outliers mixed in,
    # so they reflect the normal distribution as is.
    #
    # 1.4826 is the constant that puts MAD on the same scale as the std under a normal distribution.
    # So the meaning of n_sigma (3σ) is preserved.
    med = float(np.median(scores))
    mad = float(np.median(np.abs(scores - med))) * 1.4826
    if mad <= 0:
        mad = float(scores.std())      # If the scores are nearly identical, MAD becomes 0
    thr = med + n_sigma * mad

    # The old criterion is kept too. Comparing the two tells whether it is the threshold or the model
    # that is responsible.
    thr_mean = float(scores.mean() + n_sigma * scores.std())

    norm, anom = scores[y == 0], scores[y == 1]

    # Per type. Each type is compared only against 'normal'. Mixing types lets the hard
    # types drag down the numbers of the easy ones.
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
    """One log line. Per-type numbers are appended if present."""
    tail = ""
    if m.get("by_fault"):
        tail = "  [" + " ".join(
            f"{k} {v['auroc']:.3f}({v['detected']}/{v['n']})"
            for k, v in sorted(m["by_fault"].items())) + "]"
    base = ""
    if m.get("baseline_auroc") is not None:
        base = (f"  | no-model {m['baseline_auroc']:.4f} "
                f"(Δ{m['delta_vs_baseline']:+.4f})")
    return (f"AUROC {m['auroc']:.4f}  "
            f"detected {m['detected']}/{m['n_anomaly']}  "
            f"false alarms {m['false_alarms']}/{m['n_normal']}  "
            f"normal {m['mean_normal']:.3f} vs anomaly {m['mean_anomaly']:.3f}  "
            f"thr {m['threshold']:.3f}"
            f"(mean+{config.THRESHOLD_N_SIGMA:g}σ {m['threshold_mean3s']:.3f} -> "
            f"{m['detected_mean3s']}/{m['false_alarms_mean3s']})" + tail + base)
