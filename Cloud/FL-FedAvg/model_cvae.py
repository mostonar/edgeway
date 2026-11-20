"""
model_cvae.py — AMR 주행 신호용 조건부 1D CNN VAE

[왜 조건부인가]
같은 '정상'이어도 적재(load)·속도(spd)·주행형태(scn)에 따라 신호가 완전히 다르다.
조건 없이 한 분포로 학습하면 VAE가 모든 모드의 평균만 배우고, 어떤 조건에서든
재구성 오차가 커져 이상 신호가 묻힌다. 조건을 주면 p(x | 정상, 조건)을 배우므로
"이 조건에서 이 신호는 이상하다"를 판정할 수 있다.

[입력]
    x: (B, C, T)   C=센서 채널 수(기본 38), T=윈도우 길이(기본 60 = 60Hz 1초)
    c: (B, D)      조건 one-hot. load(3) + spd(4) + scn(4) = 11

조건은 시간축으로 펼쳐 입력 채널에 이어 붙이고(인코더), 잠재벡터에 이어 붙인다(디코더).

[크기] hid=48, latent=24 기준 90,806 파라미터.
fp16+gzip+base64로 약 217KB라 contentInstance 하나에 실어 보낼 수 있다.
(기존 Conv1DAE는 749,281 파라미터 / 1777KB였다)

[이상 점수]
channel_error()는 채널별 재구성 오차를 돌려준다. 전체 평균뿐 아니라
채널별로 보거나 상위 몇 개만 보는 집계가 가능하도록 분리해 두었다.
"""

from typing import Optional, Tuple

import torch
import torch.nn as nn


class CVAE1D(nn.Module):
    def __init__(
        self,
        n_channels: int = 38,
        cond_dim: int = 11,
        latent_dim: int = 24,
        hidden: int = 48,
        seq_len: int = 60,
    ):
        super().__init__()
        self.n_channels = n_channels
        self.cond_dim = cond_dim
        self.latent_dim = latent_dim
        self.hidden = hidden
        self.seq_len = seq_len

        # 60 -> 30 -> 15 -> 8
        self.encoder = nn.Sequential(
            nn.Conv1d(n_channels + cond_dim, hidden, 5, stride=2, padding=2),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.Conv1d(hidden, hidden, 5, stride=2, padding=2),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.Conv1d(hidden, hidden, 3, stride=2, padding=1),
            nn.BatchNorm1d(hidden), nn.GELU(),
        )
        self.enc_len = self._encoded_len(seq_len)
        self.flat = hidden * self.enc_len

        self.fc_mu = nn.Linear(self.flat, latent_dim)
        self.fc_logvar = nn.Linear(self.flat, latent_dim)
        self.fc_up = nn.Linear(latent_dim + cond_dim, self.flat)

        # 8 -> 15 -> 30 -> 60 (output_padding으로 정확히 되돌린다)
        self.decoder = nn.Sequential(
            nn.ConvTranspose1d(hidden, hidden, 3, stride=2, padding=1, output_padding=0),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.ConvTranspose1d(hidden, hidden, 5, stride=2, padding=2, output_padding=1),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.ConvTranspose1d(hidden, n_channels, 5, stride=2, padding=2, output_padding=1),
        )

    @staticmethod
    def _encoded_len(seq_len: int) -> int:
        """Conv 세 번(stride 2)을 통과한 뒤의 길이."""
        n = seq_len
        for k, p in ((5, 2), (5, 2), (3, 1)):
            n = (n + 2 * p - k) // 2 + 1
        return n

    def _with_cond(self, x: torch.Tensor, c: Optional[torch.Tensor]) -> torch.Tensor:
        if not self.cond_dim:
            return x
        if c is None:
            c = x.new_zeros(x.shape[0], self.cond_dim)
        return torch.cat([x, c.unsqueeze(-1).expand(-1, -1, x.shape[-1])], dim=1)

    def encode(self, x: torch.Tensor, c: Optional[torch.Tensor] = None):
        h = self.encoder(self._with_cond(x, c)).flatten(1)
        return self.fc_mu(h), self.fc_logvar(h)

    def decode(self, z: torch.Tensor, c: Optional[torch.Tensor] = None) -> torch.Tensor:
        if self.cond_dim:
            if c is None:
                c = z.new_zeros(z.shape[0], self.cond_dim)
            z = torch.cat([z, c], dim=1)
        h = self.fc_up(z).view(-1, self.hidden, self.enc_len)
        return self.decoder(h)[..., : self.seq_len]

    def forward(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
    ) -> Tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
        mu, logvar = self.encode(x, c)
        if self.training:
            z = mu + torch.randn_like(mu) * (0.5 * logvar).exp()
        else:
            # 평가 시에는 샘플링하지 않는다. 같은 입력에 같은 점수가 나와야
            # 임계값과 비교가 재현 가능하다.
            z = mu
        return self.decode(z, c), mu, logvar

    # ── 손실 ──────────────────────────────────────────────
    def loss(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
        beta: float = 0.5,
    ) -> Tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
        """ELBO. recon은 평균제곱오차, KL은 원소 수로 나눠 recon과 규모를 맞춘다."""
        recon, mu, logvar = self.forward(x, c)
        rec = ((recon - x) ** 2).mean()
        kl = (-0.5 * (1 + logvar - mu ** 2 - logvar.exp()).sum(dim=1)).mean()
        kl = kl / (x.shape[1] * x.shape[2])
        return rec + beta * kl, rec.detach(), kl.detach()

    def loss_terms(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
        beta: float = 0.5,
    ) -> Tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
        """
        샘플별 ELBO. loss() 와 같은 식이되 배치 평균을 내지 않는다.

        절사 손실(손실 큰 쪽을 그 에폭 학습에서 제외)을 쓰려면 샘플마다
        값이 있어야 한다. loss_terms(...)[0].mean() 은 loss(...)[0] 과 같다.
        """
        recon, mu, logvar = self.forward(x, c)
        rec = ((recon - x) ** 2).mean(dim=(1, 2))
        kl = -0.5 * (1 + logvar - mu ** 2 - logvar.exp()).sum(dim=1)
        kl = kl / (x.shape[1] * x.shape[2])
        return rec + beta * kl, rec.detach(), kl.detach()

    # ── 이상 점수 ─────────────────────────────────────────
    @torch.no_grad()
    def channel_error(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        """채널별 재구성 오차 (B, C). 집계 방식은 호출측이 정한다."""
        self.eval()
        recon, _, _ = self.forward(x, c)
        return ((recon - x) ** 2).mean(dim=2)

    @torch.no_grad()
    def channel_diff_error(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        """
        채널별 '차분' 재구성 오차 (B, C).

        왜 원신호가 아니라 차분인가
          고장 세 가지(바닥 요철 / 휠 돌기 / 베어링)는 전부 가속도계의
          1 샘플 단위 떨림으로 나타난다. 진폭 자체는 거의 안 변한다.
          원신호 오차는 1 초 창의 저주파 궤적을 복원하는 오차가 크고
          주행마다 들쭉날쭉해서, 그 위에 얹힌 미세한 고주파 어긋남을
          덮어 버린다. 차분을 먼저 취하면 저주파 성분이 빠진다.

        실측 (2노드 FedAvg 10 라운드, 라벨 붙은 siteC, 시드 3 개)
          원신호 오차  floor 0.950  wknob 0.683  bearing 0.550
          차분 오차    floor 0.975  wknob 0.783  bearing 0.749
        """
        self.eval()
        recon, _, _ = self.forward(x, c)
        d_x = x[..., 1:] - x[..., :-1]
        d_r = recon[..., 1:] - recon[..., :-1]
        return (d_r - d_x).abs().mean(dim=2)

    @torch.no_grad()
    def anomaly_score(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
        channel_norm: Optional[torch.Tensor] = None,
        topk: int = 0,
    ) -> torch.Tensor:
        """
        이상 점수 (B,).

        channel_norm: 채널별 기준 오차(학습 정상의 평균). 주면 채널별로 나눠
                      정규화한다. 채널마다 오차 규모가 달라서, 주지 않으면
                      규모가 큰 채널이 점수를 지배한다.
        topk:         0이면 전 채널 평균. n>0이면 상위 n개 채널만 평균낸다.
                      이상이 일부 채널에만 나타날 때 희석을 줄이기 위한 선택지.
        """
        e = self.channel_error(x, c)
        if channel_norm is not None:
            e = e / (channel_norm.to(e.device) + 1e-12)
        if topk and 0 < topk < e.shape[1]:
            e = e.topk(topk, dim=1).values
        return e.mean(dim=1)
