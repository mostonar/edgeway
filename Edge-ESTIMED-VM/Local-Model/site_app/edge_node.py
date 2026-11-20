"""
Edge-ESTIMED-VM/Local-Model/site_app/edge_node.py — AMR 주행 데이터용 Edge 노드

AEEdgeNode: 조건부 1D CNN VAE 기반 정상 주행 재구성 학습 (이상 탐지)
  - 입력: 다채널 시계열 (N, 38, 60) + 조건 one-hot (N, 11)
  - 손실: ELBO (재구성 MSE + beta * KL), 정상 데이터만
  - 점수: 채널별 재구성 오차를 기준 오차로 정규화해 집계
  - FedAvg 집계: get/set_state_dict()

레거시:
  EdgeNode: 통계 피처 기반 MLP 분류기 (이전 버전, 미사용)
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

# 점수를 주행 조건별 기준으로 나눌지. 기본값은 config 와 같은 환경변수를
# 본다(edge_node 는 단독 실험에서도 돌아야 해서 config 를 안 들여온다).
#
# 측정(2노드 FedAvg, 10라운드, 8시드): AUROC +0.0016, 승률 74%.
# 크기는 작지만 8시드 중 6승으로 방향이 일관됐다.
#
# 주의: 이걸 켜면 임계값의 n_sigma 도 같이 올려야 한다. 이유와 실측은
# config.THRESHOLD_N_SIGMA 주석에 있다.
COND_NORM = os.getenv("FL_COND_NORM", "1") not in ("0", "false", "False")

# 점수를 어디서 재는가.
#   diff  : |diff(x) - diff(x_hat)| 를 채널마다 학습 기준으로 z 화 -> 최대 (기본)
#   recon : |x - x_hat| 를 채널 평균 (예전 방식)
# 세 고장 모두 가속도계의 1 샘플 떨림으로 나타나는데 원신호 오차는 저주파
# 궤적 복원 오차에 덮인다. 실측은 model_cvae.channel_diff_error 주석 참조.
SCORE_MODE = os.getenv("FL_SCORE_MODE", "diff").strip().lower()

# 조건 칸 기준 z 화. edge_node 는 단독 실험에서도 돌아야 해서 없으면
# recon 으로 물러선다(z 화 없이는 diff 점수가 의미가 없다).
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
    조건부 1D CNN VAE 기반 Edge 노드

    학습: 정상 주행 데이터만 사용, ELBO(재구성 + KL) 최소화
    평가: 재구성 오차 기반 이상 점수로 AUROC / 임계값 산출

    입력 형태
        train_signals (N, C, T)  C=센서 채널(38), T=윈도우 길이(60)
        train_cond    (N, D)     조건 one-hot. 없으면 0 벡터로 채운다.
        (N, T) 2차원으로 들어오면 단일 채널로 보고 (N, 1, T)로 올린다.
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

        # 기준·임계값을 잡을 '정상 기준 창'. 주지 않으면 이번 학습 창을 쓴다.
        #
        # 이번 학습 창으로 잡으면 그 안에 섞인 이상이 기준을 끌어올린다.
        # 2026-11-19 10:25 실행에서 siteC_AMR_2 의 이상 창(고정 기준으로
        # 200~400점)이 R8/R9 학습에 들어가, 학습 뒤 같은 창으로 다시 잡은
        # 기준 때문에 9~11점으로 내려갔다(임계 15~17). 워밍업 동안 모은
        # 창을 고정해 두고 그것으로 잡으면 이상이 들어와도 기준이 안 움직인다.
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

        # 채널별 기준 오차. 학습 후 정상 데이터에서 채운다.
        # 채널마다 오차 규모가 달라서, 정규화 없이 평균내면
        # 규모가 큰 채널이 점수를 지배한다.
        self.channel_norm: Optional[torch.Tensor] = None

        # 주행 조건별 기준 점수. {조건키: 중앙값}
        #
        # 점수가 회전·저속 구간에서 10~20배 높은데, 고장이라서가 아니라
        # 그 구간이 드물어 덜 배웠기 때문이다. 조건마다 기준을 따로 잡아
        # 나누면 "느려서 높은 것"이 빠지고 "그 조건치고 높은 것"만 남는다.
        # 채널마다 channel_norm 으로 나누는 것과 같은 생각을 조건 축에 쓴다.
        self.cond_norm: Optional[dict] = None
        self.cond_norm_fallback: float = 1.0

        # SCORE_MODE="diff" 용 기준. {조건칸: (채널별 중앙값, 채널별 MAD)}
        # 채널마다·조건마다 떨림의 정상 수준이 달라서, 한 기준으로 재면
        # 회전 구간이나 떨림이 원래 큰 채널이 점수를 지배한다.
        self.diff_ref: Optional[dict] = None
        self.diff_ref_global: Optional[tuple] = None
        self.diff_alive: Optional[np.ndarray] = None
        # 정상 데이터로 잡은 임계값
        self.threshold: Optional[float] = None

        print(f"    [AEEdgeNode] node={node_id}  train={self.num_train_samples}"
              f"  val={len(X_val)}  ch={n_ch}  seq={seq}"
              f"  cond={self.cond_dim}  latent={ae_cfg.latent_dim}")

    # ── 입력 정리 ─────────────────────────────────────────

    @staticmethod
    def _as_3d(a: np.ndarray) -> torch.Tensor:
        arr = np.asarray(a, dtype=np.float32)
        if arr.ndim == 2:          # (N, T) → 단일 채널
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

    # ── 가중치 공유 ───────────────────────────────────────

    def get_state_dict(self) -> StateDict:
        return {k: v.detach().cpu().clone()
                for k, v in self.model.state_dict().items()}

    def set_state_dict(self, state: StateDict) -> None:
        """
        글로벌 가중치를 싣는다.

        FedBN 이 켜져 있으면 BatchNorm 층만 자기 것을 지킨다. 집계 쪽에서
        빼는 것만으로는 부족하다. 글로벌 모델에는 어떤 값이든 들어 있고,
        그걸 그대로 실으면 이 사업장의 통계가 덮인다. 양쪽이 다 막아야
        사업장별 BatchNorm 이 유지된다.
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

    # ── 로컬 학습 ─────────────────────────────────────────

    def train_local(
        self,
        dp_epsilon:       Optional[float] = None,
        dp_delta:         float = 1e-5,
        dp_max_grad_norm: float = 1.5,
    ) -> tuple[float, float, float]:
        """
        로컬 학습 (정상 데이터만). 반환: (train_loss, val_loss, val_auroc)
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
            # BatchNorm은 배치 크기 1에서 실패한다. 마지막 조각이 1이면 버린다.
            drop_last  = (self.num_train_samples % self.train_cfg.batch_size == 1),
        )

        # 절사 손실. 손실이 큰 쪽을 그 에폭 학습에서 뺀다.
        #
        # [왜]
        # 재구성 기반 탐지기는 정상만 학습해야 이상의 오차가 커진다. 그런데
        # 학습 집합은 비지도라 이상이 섞여 있고(FL_LABEL_FOR_TRAIN=0 이 기본),
        # 그대로 두면 모델이 이상까지 복원하는 법을 배운다. 라운드가 갈수록
        # 이상의 손실이 내려가 정상과 구분이 사라진다.
        #
        # 이상은 정의상 손실이 크므로, 손실 상위 몇 %를 빼면 라벨 없이도
        # 학습 집합이 정상 쪽으로 정리된다. 라벨은 한 글자도 보지 않는다.
        # 통계의 trimmed mean, 회귀의 LTS 와 같은 생각이다.
        #
        # 자르는 단위는 배치다. 전체를 모아 자르는 것의 근사지만 한 번의
        # 순전파로 끝나고, 배치가 섞여 있으므로 편향되지 않는다.
        #
        # [기본값이 0 인 이유]
        # 순위 능력은 올리지만 판정을 망친다. 2차 캡처로 학습하고 검증셋으로
        # 잰 결과(시드 5개): 절사 0 -> AUROC 0.7796, 오경보 138/318.
        # 절사 0.25 -> AUROC 0.7976, 오경보 244/318. 둘 다 비율에 단조다.
        #
        # 기전도 쟀다. 절사하면 학습 집합 안의 점수는 그대로인데(중앙값
        # 1.27 -> 1.30) 처음 보는 정상 주행의 점수가 3배가 된다(4.12 ->
        # 11.79). 일반화가 좁아진 것이다. 임계값은 학습 집합에서 뽑으므로
        # 그 변화를 못 따라가고(5.47 -> 5.98), 격차만큼 오경보가 된다.
        #
        # AUROC 만 보면 켜는 게 나아 보이지만, 운영에서 쓰는 것은 임계값을
        # 넘는지 여부다. 그래서 끈다. 비교하려면 FL_TRIM_RATIO 로 켠다.
        trim_ratio = float(os.getenv("FL_TRIM_RATIO", "0"))
        trim_warmup = int(os.getenv("FL_TRIM_WARMUP_EPOCHS", "2"))

        # 에폭 수를 최소 스텝 수에 맞춰 늘린다(config.TrainConfig.min_steps).
        # 라운드당 창이 50개 안팎이라 local_epochs 만으로는 스텝이 20번
        # 남짓이고, 그 정도로는 모델이 '출력 0' 에서 벗어나지 못했다.
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
            # 초반에는 모델이 무작위라 손실이 커도 이상이 아니다.
            # 워밍업 동안은 자르지 않는다.
            trim = trim_ratio if epoch >= trim_warmup else 0.0
            # 보고하는 train_loss 는 마지막 에폭의 평균이다. 에폭이 많아지면
            # 전체 평균은 초반의 큰 손실에 끌려 라운드끼리 비교가 안 된다.
            loss_sum, total = 0.0, 0

            for xb, cb in train_loader:
                xb = xb.to(self.device)
                cb = cb.to(self.device) if self.cond_dim else None

                self.optimizer.zero_grad()
                per, rec, kl = self.model.loss_terms(xb, cb, beta=self.beta)

                bs = int(xb.size(0))
                # 기록은 자르기 전 전체 평균이다. 라운드 간 비교가 되려면
                # 매번 같은 집합을 재야 한다.
                loss_sum += float(per.mean().item()) * bs
                total    += bs

                # 배치가 너무 작으면 자르는 쪽이 더 흔들린다.
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

        # 학습이 끝난 모델 기준으로 채널 기준 오차와 임계값을 다시 잡는다.
        self.calibrate()

        val_loss, val_auroc = self._evaluate_val()
        return float(train_loss), float(val_loss), float(val_auroc)

    # ── 캘리브레이션 ──────────────────────────────────────

    def calibrate(self, n_sigma: Optional[float] = None) -> Optional[float]:
        """
        학습 데이터로 채널별 기준 오차와 임계값을 정한다.

        임계값은 노드마다 자기 데이터로 잡아야 한다. 사업장·로봇마다
        정상의 모습이 달라서 공용 임계값을 쓰면 정상도 이상으로 나온다.

        [왜 평균이 아니라 중앙값인가]
        학습 집합은 비지도라 이상이 섞여 있다(FL_LABEL_FOR_TRAIN=0 이 기본).
        평균과 표준편차는 이상치에 끌려 올라가므로 임계값이 과하게 높아지고,
        그러면 아무것도 안 잡힌다. 실제로 판정(cnt-verdict)이 0 건이었다.

        중앙값과 MAD 는 이상이 절반 미만이면 거의 움직이지 않아 정상 분포를
        그대로 반영한다. 1.4826 은 정규분포에서 MAD 를 표준편차와 같은 자로
        맞추는 상수라 n_sigma 의 의미(3σ)가 그대로 유지된다.

        고정 검증셋에서 실측한 차이:
          평균+3σ      thr 8.730  탐지  6/80  오경보 0/377
          중앙값+3MAD  thr 2.320  탐지 26/80  오경보 0/377

        채널 기준 오차도 같은 이유로 중앙값을 쓴다. 평균을 쓰면 이상이 크게
        튀는 채널일수록 기준이 위로 떠서, 정작 그 채널의 이상 신호가 눌린다.
        """
        if n_sigma is None:
            # edge_node 는 config 를 안 들여온다(단독 실험에서도 돌아야 한다).
            # 있으면 쓰고 없으면 3.0 으로 간다.
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
        self.channel_norm = E.median(dim=0).values    # 채널별 기준 오차

        if SCORE_MODE == "diff":
            # 차분 오차의 '정상 수준' 을 채널 x 조건칸 마다 잡는다.
            # 조건칸 표본이 적으면(8 개 미만) 전체 기준으로 물러선다.
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

        # 조건별 기준값을 학습 점수에서 잡는다. 그 다음 임계값은 '나눈'
        # 점수로 잡아야 추론 때와 단위가 같아진다.
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
            # 점수가 거의 같으면 MAD 가 0 이 된다. 표준편차로, 그것도
            # 못 쓰면(샘플 1개) 중앙값의 10% 로 물러선다.
            s = scores.std()
            mad = (s if torch.isfinite(s) and s > 0
                   else med.abs() * 0.1 + 1e-6)

        self.threshold = float(med + n_sigma * mad)

        # 임계값이 정상 덩어리에 얼마나 붙어 있는지 눈으로 보려고 분포를
        # 같이 찍는다. 넘은 것만 로그에 남으면 "너무 낮다"를 판단할 수 없다.
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
        cond 원핫(nav 5 + speed 3)을 조건키 하나로 접는다.

        모델이 실제로 받는 벡터를 그대로 쓴다. 따로 nav/speed 를 받아오면
        학습 때와 추론 때 기준이 어긋날 수 있다.
        """
        C = np.asarray(cond, dtype=np.float32)
        if C.ndim != 2 or C.shape[1] < 8:
            return np.zeros(len(C), dtype=np.int64)
        return (np.argmax(C[:, :5], 1) * 10
                + np.argmax(C[:, 5:8], 1)).astype(np.int64)

    @staticmethod
    def cell_key(cond: np.ndarray) -> np.ndarray:
        """cond 원핫(nav 5 + speed 3 + 적재 3)을 조건칸 번호 하나로 접는다.

        cond_key 와 달리 적재까지 넣는다. 떨림은 싣고 있는 무게에 따라
        정상 수준이 달라서, 적재를 빼면 '무거워서 큰 것' 이 이상으로 샌다.
        """
        C = np.asarray(cond, dtype=np.float32)
        if C.ndim != 2 or C.shape[1] < 8:
            return np.zeros(len(C), dtype=np.int64)
        k = np.argmax(C[:, :5], 1) * 9 + np.argmax(C[:, 5:8], 1) * 3
        if C.shape[1] >= 11:
            k = k + np.argmax(C[:, 8:11], 1)
        return k.astype(np.int64)

    def _diff_scores(self, E: np.ndarray, cond: Optional[np.ndarray]) -> np.ndarray:
        """채널별 차분 오차 (N, C) -> 점수 (N,). 학습 기준으로 z 화 후 최대."""
        if self.diff_ref_global is None:
            return E.max(axis=1)
        keys = (self.cell_key(cond) if cond is not None
                else np.zeros(len(E), dtype=np.int64))
        Z = _apply_ref(E, keys, self.diff_ref or {},
                       self.diff_ref_global, self.diff_alive)
        return Z.max(axis=1)

    def _apply_cond_norm(self, scores: np.ndarray,
                         cond: Optional[np.ndarray]) -> np.ndarray:
        """점수를 그 조건의 기준으로 나눈다. 기준이 없으면 그대로 둔다."""
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
        """채널별 오차 (N, C) -> 이상 점수 (N,)"""
        if self.channel_norm is not None:
            E = E / (self.channel_norm.to(E.device) + 1e-12)
        if self.topk and 0 < self.topk < E.shape[1]:
            E = E.topk(self.topk, dim=1).values
        return E.mean(dim=1)

    # ── 평가 ──────────────────────────────────────────────

    def _evaluate_val(self) -> tuple[float, float]:
        """val 세트 평가: (정상 평균 점수, AUROC)"""
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
                    # 운영 점수와 같은 자로 잰다. 둘이 다르면 로컬 화면의
                    # val AUROC 와 실제 판정이 어긋난다.
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
        """(N, C, T) 신호 -> 이상 점수 (N,)"""
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
            # 조건칸별 z 화가 조건 정규화 역할까지 하므로 따로 안 나눈다.
            return self._diff_scores(
                E, cond if cond is not None else C.cpu().numpy())
        return self._apply_cond_norm(E, cond)

    @staticmethod
    def aggregate_by_run(
        scores: np.ndarray,
        run_ids: np.ndarray,
    ) -> tuple[np.ndarray, np.ndarray]:
        """
        윈도우 점수를 주행 1회 단위로 평균낸다.

        실제 판단 단위는 1초가 아니라 "이 로봇이 이 구간을 지나는 동안"이다.
        검증 실험에서 윈도우 단위 AUROC 0.65가 주행 단위로는 0.73까지 올랐다.
        반환: (run별 점수, run id)
        """
        runs = np.asarray(run_ids)
        uniq = np.unique(runs)
        return (np.array([scores[runs == r].mean() for r in uniq]), uniq)


# ════════════════════════════════════════════════════════
# AUROC 헬퍼
# ════════════════════════════════════════════════════════

def _compute_auroc(scores: np.ndarray, labels: np.ndarray) -> float:
    """
    scores가 높을수록 anomaly에 가깝다고 가정
    labels: 0=정상, 1=이상
    """
    if len(np.unique(labels)) < 2:
        return 0.5   # 단일 클래스면 의미 없음

    try:
        from sklearn.metrics import roc_auc_score
        return float(roc_auc_score(labels, scores))
    except Exception:
        return 0.5


# ════════════════════════════════════════════════════════
# 레거시 EdgeNode (통계 피처 기반 MLP, 미사용)
# ════════════════════════════════════════════════════════

class EdgeNode:
    """레거시: 통계 피처 벡터 → 분류 (현재 미사용, 인터페이스 유지)"""

    def __init__(self, node_id, data, sensor_cols, train_cfg, device="cpu", **kwargs):
        raise NotImplementedError(
            "EdgeNode (통계피처 MLP) 는 더 이상 사용하지 않습니다. "
            "AEEdgeNode 를 사용하세요."
        )
