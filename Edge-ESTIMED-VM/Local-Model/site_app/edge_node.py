"""
Edge-ESTIMED-VM/Local-Model/site_app/edge_node.py — edge node for AMR driving data

AEEdgeNode: learns to reconstruct normal driving with a conditional 1D CNN VAE (anomaly detection)
  - Input: multichannel time series (N, 38, 60) + condition one-hot (N, 11)
  - Loss: ELBO (reconstruction MSE + beta * KL), normal data only
  - Score: per-channel reconstruction error normalized by the reference error, then aggregated
  - FedAvg aggregation: get/set_state_dict()

Legacy:
  EdgeNode: MLP classifier on statistical features (previous version, unused)
"""
from __future__ import annotations

import math
import os
from dataclasses import dataclass
from typing import List, Optional

import numpy as np
import torch
import torch.nn as nn
from torch.utils.data import DataLoader, TensorDataset

from config import FEDBN

# Whether to divide scores by a per-driving-condition reference. The default reads the same environment
# variable as config (edge_node must run in standalone experiments too, so it does not import config).
#
# Measured (2-node FedAvg, 10 rounds, 8 seeds): AUROC +0.0016, win rate 74%.
# Small, but the direction was consistent with 6 wins out of 8 seeds.
#
# Caution: turning this on means n_sigma for the threshold must be raised too. The reason and measurements
# are in the comment on config.THRESHOLD_N_SIGMA.
COND_NORM = os.getenv("FL_COND_NORM", "1") not in ("0", "false", "False")

# Where the score is measured.
#   diff  : z-score |diff(x) - diff(x_hat)| per channel against the training reference -> max (default)
#   recon : mean over channels of |x - x_hat| (old method)
# All three faults show up as one-sample jitter in the accelerometers, but raw-signal error is masked by the
# low-frequency trajectory reconstruction error. For measurements see the comment on model_cvae.channel_diff_error.
SCORE_MODE = os.getenv("FL_SCORE_MODE", "diff").strip().lower()

# Condition-cell z-scoring. edge_node must run in standalone experiments too, so if missing it
# falls back to recon (without z-scoring the diff score is meaningless).
try:
    from amr_schema import robust_ref as _robust_ref, apply_ref as _apply_ref
except Exception:                                   # pragma: no cover
    _robust_ref = _apply_ref = None
    if SCORE_MODE == "diff":
        print("  ⚠ amr_schema 를 못 들여왔다 -> FL_SCORE_MODE=recon 으로 진행")
        SCORE_MODE = "recon"
from config import TrainConfig, AEConfig
from model_cvae import CVAE1D

StateDict = dict[str, torch.Tensor]


# ════════════════════════════════════════════════════════
# AE Edge Node
# ════════════════════════════════════════════════════════

class AEEdgeNode:
    """
    Edge node based on a conditional 1D CNN VAE

    Training: normal driving data only, minimizing ELBO (reconstruction + KL)
    Evaluation: AUROC / threshold from the reconstruction-error-based anomaly score

    Input shapes
        train_signals (N, C, T)  C=sensor channels (38), T=window length (60)
        train_cond    (N, D)     condition one-hot. Filled with zero vectors if absent.
        If it comes in as 2D (N, T), it is treated as a single channel and lifted to (N, 1, T).
    """

    def __init__(
        self,
        node_id:    int,
        train_signals: np.ndarray,
        val_signals:   np.ndarray,
        val_labels:    np.ndarray,
        ae_cfg:     AEConfig,
        train_cfg:  TrainConfig,
        device:     str = "cpu",
        train_cond: Optional[np.ndarray] = None,
        val_cond:   Optional[np.ndarray] = None,
        train_run_id: Optional[np.ndarray] = None,
        val_run_id:   Optional[np.ndarray] = None,
        calib_signals: Optional[np.ndarray] = None,
        calib_cond:    Optional[np.ndarray] = None,
    ):
        self.node_id   = node_id
        self.device    = device
        self.ae_cfg    = ae_cfg
        self.train_cfg = train_cfg

        self.cond_dim = int(getattr(ae_cfg, "cond_dim", 0) or 0)
        self.beta     = float(getattr(ae_cfg, "beta", 0.5))
        self.topk     = int(getattr(ae_cfg, "score_topk", 0) or 0)

        X_train = self._as_3d(train_signals)
        X_val   = self._as_3d(val_signals)

        C_train = self._as_cond(train_cond, len(X_train))
        C_val   = self._as_cond(val_cond,   len(X_val))

        y_val = torch.tensor(np.asarray(val_labels).reshape(-1), dtype=torch.long)

        self.train_ds = TensorDataset(X_train, C_train)
        self.val_ds   = TensorDataset(X_val, C_val, y_val)

        # 'Normal reference windows' for setting the reference and threshold. If not given, this round's training windows are used.
        #
        # Using this round's training windows lets any anomalies mixed in pull the reference up.
        # In the 2026-11-19 10:25 run, siteC_AMR_2's anomalous windows (200~400 points against a fixed
        # reference) got into R8/R9 training, and the reference re-taken from the same windows after training
        # brought them down to 9~11 points (threshold 15~17). Fixing the windows collected during warm-up
        # and using those keeps the reference from moving even when anomalies come in.
        if calib_signals is not None and len(calib_signals):
            self.calib_ds = TensorDataset(
                self._as_3d(calib_signals),
                self._as_cond(calib_cond, len(calib_signals)))
        else:
            self.calib_ds = None

        self.train_run_id = (np.asarray(train_run_id)
                             if train_run_id is not None else None)
        self.val_run_id = (np.asarray(val_run_id)
                           if val_run_id is not None else None)

        self.num_train_samples = len(X_train)
        n_ch = X_train.shape[1] if len(X_train) else int(ae_cfg.n_channels)
        seq  = X_train.shape[2] if len(X_train) else int(ae_cfg.seq_len)

        self.model = CVAE1D(
            n_channels = n_ch,
            cond_dim   = self.cond_dim,
            latent_dim = ae_cfg.latent_dim,
            hidden     = int(getattr(ae_cfg, "hidden", 48)),
            seq_len    = seq,
        ).to(device)

        self.optimizer = torch.optim.Adam(
            self.model.parameters(),
            lr           = train_cfg.lr,
            weight_decay = train_cfg.weight_decay,
        )

        # Per-channel reference error. Filled from normal data after training.
        # Error scales differ per channel, so averaging without normalization lets
        # large-scale channels dominate the score.
        self.channel_norm: Optional[torch.Tensor] = None

        # Per-driving-condition reference score. {condition key: median}
        #
        # Scores are 10~20x higher in turning / low-speed segments, not because of faults but
        # because those segments are rare and less learned. Taking a separate reference per condition and
        # dividing removes "high because it is slow" and leaves only "high for that condition".
        # The same idea as dividing each channel by channel_norm, applied along the condition axis.
        self.cond_norm: Optional[dict] = None
        self.cond_norm_fallback: float = 1.0

        # Reference for SCORE_MODE="diff". {condition cell: (per-channel median, per-channel MAD)}
        # The normal level of jitter differs per channel and per condition, so measuring with one reference lets
        # turning segments or channels with inherently large jitter dominate the score.
        self.diff_ref: Optional[dict] = None
        self.diff_ref_global: Optional[tuple] = None
        self.diff_alive: Optional[np.ndarray] = None
        # Threshold set from normal data
        self.threshold: Optional[float] = None

        print(f"    [AEEdgeNode] node={node_id}  train={self.num_train_samples}"
              f"  val={len(X_val)}  ch={n_ch}  seq={seq}"
              f"  cond={self.cond_dim}  latent={ae_cfg.latent_dim}")

    # ── Input cleanup ─────────────────────────────────────

    @staticmethod
    def _as_3d(a: np.ndarray) -> torch.Tensor:
        arr = np.asarray(a, dtype=np.float32)
        if arr.ndim == 2:          # (N, T) → single channel
            arr = arr[:, None, :]
        elif arr.ndim == 1:
            arr = arr.reshape(0, 1, 0)
        return torch.tensor(arr, dtype=torch.float32)

    def _as_cond(self, c: Optional[np.ndarray], n: int) -> torch.Tensor:
        if self.cond_dim <= 0:
            return torch.zeros(n, 0, dtype=torch.float32)
        if c is None or len(c) != n:
            return torch.zeros(n, self.cond_dim, dtype=torch.float32)
        return torch.tensor(np.asarray(c, dtype=np.float32), dtype=torch.float32)

    # ── Weight sharing ────────────────────────────────────

    def get_state_dict(self) -> StateDict:
        return {k: v.detach().cpu().clone()
                for k, v in self.model.state_dict().items()}

    def set_state_dict(self, state: StateDict) -> None:
        """
        Load the global weights.

        With FedBN on, only the BatchNorm layers keep their own values. Excluding them on the
        aggregation side is not enough. The global model holds some values for them anyway,
        and loading it as is overwrites this site's statistics. Both sides must block it for
        per-site BatchNorm to be preserved.
        """
        if FEDBN:
            own = self.model.state_dict()
            bn = tuple(k[: -len("running_mean")]
                       for k in own if k.endswith("running_mean"))
            if bn:
                state = {k: (own[k] if k.startswith(bn) else v)
                         for k, v in state.items()}
        self.model.load_state_dict(state)
        self.model.to(self.device)

    # ── Local training ────────────────────────────────────

    def train_local(
        self,
        dp_epsilon:       Optional[float] = None,
        dp_delta:         float = 1e-5,
        dp_max_grad_norm: float = 1.5,
    ) -> tuple[float, float, float]:
        """
        Local training (normal data only). Returns: (train_loss, val_loss, val_auroc)
        """
        if self.num_train_samples == 0:
            print("    ⚠ 학습 데이터가 없다")
            v_loss, v_auroc = self._evaluate_val()
            return 0.0, float(v_loss), float(v_auroc)

        use_dp = dp_epsilon is not None
        if use_dp:
            sigma = math.sqrt(2 * math.log(1.25 / dp_delta)) / dp_epsilon

        train_loader = DataLoader(
            self.train_ds,
            batch_size = self.train_cfg.batch_size,
            shuffle    = True,
            # BatchNorm fails with batch size 1. If the last chunk is 1, drop it.
            drop_last  = (self.num_train_samples % self.train_cfg.batch_size == 1),
        )

        # Trimmed loss. The high-loss samples are excluded from that epoch's training.
        #
        # [Why]
        # A reconstruction-based detector must learn only normal data for anomaly errors to grow. But
        # the training set is unsupervised and has anomalies mixed in (FL_LABEL_FOR_TRAIN=0 by default),
        # and left alone the model learns to reconstruct anomalies too. As rounds go on
        # the anomalies' loss drops and the distinction from normal disappears.
        #
        # Anomalies by definition have high loss, so dropping the top few % of losses cleans the
        # training set toward normal without labels. Not a single label is looked at.
        # The same idea as a trimmed mean in statistics or LTS in regression.
        #
        # Trimming is per batch. It approximates trimming over the whole set but takes one
        # forward pass, and batches are shuffled so it is not biased.
        #
        # [Why the default is 0]
        # It improves ranking but ruins verdicts. Trained on the second capture and measured on the
        # validation set (5 seeds): trim 0 -> AUROC 0.7796, false alarms 138/318.
        # trim 0.25 -> AUROC 0.7976, false alarms 244/318. Both are monotonic in the ratio.
        #
        # The mechanism was measured too. With trimming, scores within the training set stay the same (median
        # 1.27 -> 1.30) but scores on unseen normal driving triple (4.12 ->
        # 11.79). Generalization narrowed. The threshold is drawn from the training set, so
        # it cannot follow that change (5.47 -> 5.98), and the gap turns into false alarms.
        #
        # Looking only at AUROC, turning it on seems better, but what operation uses is whether the threshold
        # is crossed. So it is off. To compare, turn it on with FL_TRIM_RATIO.
        trim_ratio = float(os.getenv("FL_TRIM_RATIO", "0"))
        trim_warmup = int(os.getenv("FL_TRIM_WARMUP_EPOCHS", "2"))

        # Raise the epoch count to meet the minimum step count (config.TrainConfig.min_steps).
        # There are about 50 windows per round, so local_epochs alone gives only ~20 steps,
        # and with that the model never got away from 'output 0'.
        steps_per_epoch = max(1, len(train_loader))
        min_steps = int(getattr(self.train_cfg, "min_steps", 0) or 0)
        n_epochs = max(int(self.train_cfg.local_epochs),
                       math.ceil(min_steps / steps_per_epoch))
        print(f"    ◦ 로컬 학습 {n_epochs}에폭 x {steps_per_epoch}배치 "
              f"= {n_epochs * steps_per_epoch}스텝"
              f"{' (DP)' if use_dp else ''}")

        self.model.train()
        loss_sum, total, trimmed = 0.0, 0, 0

        for epoch in range(n_epochs):
            # Early on the model is random, so a high loss does not mean an anomaly.
            # No trimming during warm-up.
            trim = trim_ratio if epoch >= trim_warmup else 0.0
            # The reported train_loss is the mean of the last epoch. With many epochs,
            # the overall mean is dragged by the large early losses and rounds cannot be compared.
            loss_sum, total = 0.0, 0

            for xb, cb in train_loader:
                xb = xb.to(self.device)
                cb = cb.to(self.device) if self.cond_dim else None

                self.optimizer.zero_grad()
                per, rec, kl = self.model.loss_terms(xb, cb, beta=self.beta)

                bs = int(xb.size(0))
                # What is recorded is the overall mean before trimming. For rounds to be comparable
                # the same set must be measured every time.
                loss_sum += float(per.mean().item()) * bs
                total    += bs

                # If a batch is too small, trimming fluctuates more.
                if trim > 0 and bs > 4:
                    keep_n = max(1, int(round(bs * (1.0 - trim))))
                    kept = per.topk(keep_n, largest=False).values
                    trimmed += bs - keep_n
                else:
                    kept = per

                loss = kept.mean()
                loss.backward()

                if use_dp:
                    torch.nn.utils.clip_grad_norm_(
                        self.model.parameters(), max_norm=dp_max_grad_norm
                    )
                    for param in self.model.parameters():
                        if param.grad is not None:
                            param.grad += (torch.randn_like(param.grad)
                                           * sigma * dp_max_grad_norm / bs)

                self.optimizer.step()

        train_loss = loss_sum / max(total, 1)

        if trimmed:
            print(f"    ◦ 절사 손실: {trimmed}/{total * n_epochs} 샘플을 학습에서 제외 "
                  f"(비율 {trim_ratio:.0%}, 워밍업 {trim_warmup}에폭)")

        # Re-take the per-channel reference error and threshold against the trained model.
        self.calibrate()

        val_loss, val_auroc = self._evaluate_val()
        return float(train_loss), float(val_loss), float(val_auroc)

    # ── Calibration ───────────────────────────────────────

    def calibrate(self, n_sigma: Optional[float] = None) -> Optional[float]:
        """
        Set per-channel reference errors and the threshold from training data.

        Each node must set its threshold from its own data. What normal looks like differs per
        site and robot, so a shared threshold makes normal look anomalous too.

        [Why median rather than mean]
        The training set is unsupervised and has anomalies mixed in (FL_LABEL_FOR_TRAIN=0 by default).
        Mean and std get pulled up by outliers, so the threshold ends up too high,
        and then nothing is caught. In fact there were 0 verdicts (cnt-verdict).

        Median and MAD barely move as long as anomalies are under half, so they reflect the normal distribution
        as is. 1.4826 is the constant that puts MAD on the same scale as the std under a normal distribution,
        so the meaning of n_sigma (3σ) is preserved.

        Difference measured on the fixed validation set:
          mean+3σ      thr 8.730  detected  6/80  false alarms 0/377
          median+3MAD  thr 2.320  detected 26/80  false alarms 0/377

        Per-channel reference error also uses the median for the same reason. With the mean, the more a channel's
        anomalies spike, the higher its reference floats, which suppresses that very channel's anomaly signal.
        """
        if n_sigma is None:
            # edge_node does not import config (it must run in standalone experiments too).
            # Use it if present, otherwise 3.0.
            try:
                import config as _cfg
                n_sigma = float(getattr(_cfg, "THRESHOLD_N_SIGMA", 3.0))
            except Exception:
                n_sigma = float(os.getenv("FL_THRESHOLD_N_SIGMA", "3.0"))

        ref_ds = self.calib_ds if self.calib_ds is not None else self.train_ds
        X_ref, C_ref = ref_ds.tensors[0], ref_ds.tensors[1]
        n_ref = len(X_ref)
        if n_ref == 0:
            return None

        self.model.eval()
        errs, derrs = [], []
        with torch.no_grad():
            for i in range(0, n_ref, self.train_cfg.batch_size):
                xb = X_ref[i:i + self.train_cfg.batch_size].to(self.device)
                cb = (C_ref[i:i + self.train_cfg.batch_size].to(self.device)
                      if self.cond_dim else None)
                errs.append(self.model.channel_error(xb, cb).cpu())
                if SCORE_MODE == "diff":
                    derrs.append(self.model.channel_diff_error(xb, cb).cpu())

        E = torch.cat(errs)                       # (N, C)
        self.channel_norm = E.median(dim=0).values    # Per-channel reference error

        if SCORE_MODE == "diff":
            # Set the 'normal level' of differenced error per channel x condition cell.
            # If a condition cell has few samples (under 8), fall back to the overall reference.
            D = torch.cat(derrs).numpy()              # (N, C)
            cond_np = (C_ref[:len(D)].cpu().numpy()
                       if self.cond_dim else None)
            keys = (self.cell_key(cond_np) if cond_np is not None
                    else np.zeros(len(D), dtype=np.int64))
            (self.diff_ref, self.diff_ref_global,
             self.diff_alive) = _robust_ref(D, keys)
            scores = torch.from_numpy(self._diff_scores(D, cond_np))
            dead = int((~self.diff_alive).sum())
            print(f"    ◦ 점수 방식 diff (차분 오차 채널 z 최대) · "
                  f"조건칸 기준 {len(self.diff_ref)}개 · 기준 창 {n_ref}개"
                  f"{' (고정 기준 집합)' if self.calib_ds is not None else ''}"
                  + (f" · 상수 채널 {dead}개 제외" if dead else ""))
        else:
            scores = self._aggregate(E)

        # Take the per-condition reference from training scores. After that the threshold must be set on the
        # 'divided' score so its units match inference.
        if COND_NORM and self.cond_dim and SCORE_MODE != "diff":
            raw = scores.cpu().numpy()
            keys = self.cond_key(C_ref[:len(raw)].cpu().numpy())
            self.cond_norm_fallback = float(np.median(raw))
            self.cond_norm = {
                int(k): float(np.median(raw[keys == k]))
                for k in np.unique(keys)
            }
            scores = torch.from_numpy(
                self._apply_cond_norm(raw, C_ref[:len(raw)].cpu().numpy()))
            print(f"    ◦ 조건별 기준 {len(self.cond_norm)}개 "
                  f"(중앙값 범위 {min(self.cond_norm.values()):.2f}"
                  f"~{max(self.cond_norm.values()):.2f})")

        med = scores.median()
        mad = (scores - med).abs().median() * 1.4826
        if not torch.isfinite(mad) or mad <= 0:
            # If scores are nearly identical, MAD becomes 0. Fall back to the std, and if that is not
            # usable either (1 sample), to 10% of the median.
            s = scores.std()
            mad = (s if torch.isfinite(s) and s > 0
                   else med.abs() * 0.1 + 1e-6)

        self.threshold = float(med + n_sigma * mad)

        # Print the distribution too, to see by eye how close the threshold sits to the normal cluster.
        # If only exceedances are logged, you cannot judge "too low".
        q = torch.quantile(
            scores.float(),
            torch.tensor([0.50, 0.90, 0.99], device=scores.device),
        ).tolist()
        over = int((scores > self.threshold).sum())
        print(f"    ◦ 임계 {self.threshold:.4f}  (중앙값 {q[0]:.4f} "
              f"+ {n_sigma:g} x MAD {float(mad):.4f})\n"
              f"      학습점수 p50 {q[0]:.3f}  p90 {q[1]:.3f}  "
              f"p99 {q[2]:.3f}  max {float(scores.max()):.3f}  "
              f"| 임계 초과 {over}/{len(scores)} "
              f"({over / max(1, len(scores)):.1%})")

        return self.threshold

    @staticmethod
    def cond_key(cond: np.ndarray) -> np.ndarray:
        """
        Fold the cond one-hot (nav 5 + speed 3) into a single condition key.

        Uses exactly the vector the model receives. Fetching nav/speed separately could make
        the reference differ between training and inference.
        """
        C = np.asarray(cond, dtype=np.float32)
        if C.ndim != 2 or C.shape[1] < 8:
            return np.zeros(len(C), dtype=np.int64)
        return (np.argmax(C[:, :5], 1) * 10
                + np.argmax(C[:, 5:8], 1)).astype(np.int64)

    @staticmethod
    def cell_key(cond: np.ndarray) -> np.ndarray:
        """Fold the cond one-hot (nav 5 + speed 3 + load 3) into a single condition cell index.

        Unlike cond_key, this includes load. The normal level of jitter depends on the weight being carried,
        so leaving load out lets 'large because it is heavy' leak through as anomalies.
        """
        C = np.asarray(cond, dtype=np.float32)
        if C.ndim != 2 or C.shape[1] < 8:
            return np.zeros(len(C), dtype=np.int64)
        k = np.argmax(C[:, :5], 1) * 9 + np.argmax(C[:, 5:8], 1) * 3
        if C.shape[1] >= 11:
            k = k + np.argmax(C[:, 8:11], 1)
        return k.astype(np.int64)

    def _diff_scores(self, E: np.ndarray, cond: Optional[np.ndarray]) -> np.ndarray:
        """Per-channel differenced error (N, C) -> score (N,). z-score against the training reference, then max."""
        if self.diff_ref_global is None:
            return E.max(axis=1)
        keys = (self.cell_key(cond) if cond is not None
                else np.zeros(len(E), dtype=np.int64))
        Z = _apply_ref(E, keys, self.diff_ref or {},
                       self.diff_ref_global, self.diff_alive)
        return Z.max(axis=1)

    def _apply_cond_norm(self, scores: np.ndarray,
                         cond: Optional[np.ndarray]) -> np.ndarray:
        """Divide the score by that condition's reference. Leave it as is if there is no reference."""
        if not COND_NORM or not self.cond_norm or cond is None:
            return scores
        keys = self.cond_key(cond)
        if len(keys) != len(scores):
            return scores
        div = np.array(
            [max(self.cond_norm.get(int(k), self.cond_norm_fallback), 1e-6)
             for k in keys],
            dtype=np.float32,
        )
        return scores / div

    def _aggregate(self, E: torch.Tensor) -> torch.Tensor:
        """Per-channel error (N, C) -> anomaly score (N,)"""
        if self.channel_norm is not None:
            E = E / (self.channel_norm.to(E.device) + 1e-12)
        if self.topk and 0 < self.topk < E.shape[1]:
            E = E.topk(self.topk, dim=1).values
        return E.mean(dim=1)

    # ── Evaluation ────────────────────────────────────────

    def _evaluate_val(self) -> tuple[float, float]:
        """Evaluate on the val set: (mean normal score, AUROC)"""
        if len(self.val_ds) == 0:
            return 0.0, 0.5

        val_loader = DataLoader(
            self.val_ds,
            batch_size = self.train_cfg.batch_size,
            shuffle    = False,
        )

        self.model.eval()
        all_scores, all_labels, all_cond = [], [], []

        with torch.no_grad():
            for xb, cb, yb in val_loader:
                xb = xb.to(self.device)
                cb_ = cb.to(self.device) if self.cond_dim else None
                if SCORE_MODE == "diff":
                    # Measured with the same ruler as the operational score. If they differ, the val AUROC
                    # on the local screen disagrees with the actual verdicts.
                    all_scores.append(
                        self.model.channel_diff_error(xb, cb_).cpu())
                    all_cond.append(cb.cpu())
                else:
                    all_scores.append(self._aggregate(
                        self.model.channel_error(xb, cb_)).cpu())
                all_labels.append(yb)

        if SCORE_MODE == "diff":
            scores = self._diff_scores(
                torch.cat(all_scores).numpy(),
                torch.cat(all_cond).numpy() if all_cond else None)
        else:
            scores = torch.cat(all_scores).numpy()
        labels = torch.cat(all_labels).numpy()

        normal = scores[labels == 0]
        val_loss = float(normal.mean()) if len(normal) else 0.0

        return val_loss, _compute_auroc(scores, labels)

    def compute_anomaly_scores(
        self,
        signals: np.ndarray,
        cond: Optional[np.ndarray] = None,
    ) -> np.ndarray:
        """(N, C, T) signal -> anomaly score (N,)"""
        X = self._as_3d(signals).to(self.device)
        if len(X) == 0:
            return np.zeros((0,), dtype=np.float32)
        C = self._as_cond(cond, len(X)).to(self.device)

        self.model.eval()
        out = []
        with torch.no_grad():
            for i in range(0, len(X), self.train_cfg.batch_size):
                xb = X[i:i + self.train_cfg.batch_size]
                cb = C[i:i + self.train_cfg.batch_size] if self.cond_dim else None
                if SCORE_MODE == "diff":
                    out.append(self.model.channel_diff_error(xb, cb).cpu().numpy())
                else:
                    out.append(self._aggregate(
                        self.model.channel_error(xb, cb)).cpu().numpy())

        E = np.concatenate(out)
        if SCORE_MODE == "diff":
            # Per-condition-cell z-scoring already does the condition normalization, so no separate division.
            return self._diff_scores(
                E, cond if cond is not None else C.cpu().numpy())
        return self._apply_cond_norm(E, cond)

    @staticmethod
    def aggregate_by_run(
        scores: np.ndarray,
        run_ids: np.ndarray,
    ) -> tuple[np.ndarray, np.ndarray]:
        """
        Average window scores per driving run.

        The real decision unit is not 1 second but "while this robot passes through this segment".
        In validation experiments, a window-level AUROC of 0.65 rose to 0.73 at the run level.
        Returns: (per-run scores, run ids)
        """
        runs = np.asarray(run_ids)
        uniq = np.unique(runs)
        return (np.array([scores[runs == r].mean() for r in uniq]), uniq)


# ════════════════════════════════════════════════════════
# AUROC helper
# ════════════════════════════════════════════════════════

def _compute_auroc(scores: np.ndarray, labels: np.ndarray) -> float:
    """
    Assumes higher scores are closer to anomaly
    labels: 0=normal, 1=anomaly
    """
    if len(np.unique(labels)) < 2:
        return 0.5   # Meaningless with a single class

    try:
        from sklearn.metrics import roc_auc_score
        return float(roc_auc_score(labels, scores))
    except Exception:
        return 0.5


# ════════════════════════════════════════════════════════
# Legacy EdgeNode (MLP on statistical features, unused)
# ════════════════════════════════════════════════════════

class EdgeNode:
    """Legacy: statistical feature vector → classification (currently unused, interface kept)"""

    def __init__(self, node_id, data, sensor_cols, train_cfg, device="cpu", **kwargs):
        raise NotImplementedError(
            "EdgeNode (통계피처 MLP) 는 더 이상 사용하지 않습니다. "
            "AEEdgeNode 를 사용하세요."
        )
