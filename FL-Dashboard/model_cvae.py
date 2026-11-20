"""
model_cvae.py — conditional 1D CNN VAE for AMR driving signals

[Why conditional]
Even when 'normal', the signal is completely different depending on load, speed (spd) and driving pattern (scn).
Training on a single distribution without conditions makes the VAE learn only the average of all modes, so
reconstruction error grows under every condition and anomalies get buried. With conditions it learns p(x | normal, condition),
so it can judge "this signal is anomalous under this condition".

[Input]
    x: (B, C, T)   C=number of sensor channels (default 38), T=window length (default 60 = 1 s at 60 Hz)
    c: (B, D)      condition one-hot. load(3) + spd(4) + scn(4) = 11

The condition is broadcast along the time axis and concatenated to the input channels (encoder), and concatenated to the latent vector (decoder).

[Size] 90,806 parameters with hid=48, latent=24.
About 217 KB with fp16+gzip+base64, so it fits in a single contentInstance.
(The previous Conv1DAE was 749,281 parameters / 1777 KB)

[Anomaly score]
channel_error() returns per-channel reconstruction error. It is kept separate so that besides the overall mean,
aggregations per channel or over only the top few channels are possible.
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

        # 8 -> 15 -> 30 -> 60 (output_padding restores the exact length)
        self.decoder = nn.Sequential(
            nn.ConvTranspose1d(hidden, hidden, 3, stride=2, padding=1, output_padding=0),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.ConvTranspose1d(hidden, hidden, 5, stride=2, padding=2, output_padding=1),
            nn.BatchNorm1d(hidden), nn.GELU(),
            nn.ConvTranspose1d(hidden, n_channels, 5, stride=2, padding=2, output_padding=1),
        )

    @staticmethod
    def _encoded_len(seq_len: int) -> int:
        """Length after passing through three convs (stride 2)."""
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
            # No sampling at evaluation time. The same input must give the same score
            # for comparisons against the threshold to be reproducible.
            z = mu
        return self.decode(z, c), mu, logvar

    # ── Loss ──────────────────────────────────────────────
    def loss(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
        beta: float = 0.5,
    ) -> Tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
        """ELBO. recon is mean squared error; KL is divided by the element count to match recon's scale."""
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
        Per-sample ELBO. Same formula as loss() but without averaging over the batch.

        Trimmed loss (excluding high-loss samples from that epoch's training) needs a value
        per sample. loss_terms(...)[0].mean() equals loss(...)[0].
        """
        recon, mu, logvar = self.forward(x, c)
        rec = ((recon - x) ** 2).mean(dim=(1, 2))
        kl = -0.5 * (1 + logvar - mu ** 2 - logvar.exp()).sum(dim=1)
        kl = kl / (x.shape[1] * x.shape[2])
        return rec + beta * kl, rec.detach(), kl.detach()

    # ── Anomaly score ─────────────────────────────────────
    @torch.no_grad()
    def channel_error(
        self,
        x: torch.Tensor,
        c: Optional[torch.Tensor] = None,
    ) -> torch.Tensor:
        """Per-channel reconstruction error (B, C). The caller decides how to aggregate."""
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
        Per-channel 'differenced' reconstruction error (B, C).

        Why differences rather than the raw signal
          All three faults (floor bumps / wheel knob / bearing) show up as sample-to-sample
          jitter in the accelerometers. The amplitude itself barely changes.
          Raw-signal error is dominated by the error in reconstructing the low-frequency trajectory of the 1 s window,
          which varies from run to run, and it masks the subtle high-frequency deviations riding on top.
          Taking the difference first removes the low-frequency component.

        Measured (2-node FedAvg, 10 rounds, labeled siteC, 3 seeds)
          Raw error         floor 0.950  wknob 0.683  bearing 0.550
          Differenced error floor 0.975  wknob 0.783  bearing 0.749
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
        Anomaly score (B,).

        channel_norm: per-channel reference error (mean on normal training data). If given, each channel
                      is normalized by it. Error scales differ per channel, so without it
                      large-scale channels dominate the score.
        topk:         0 = mean over all channels. n>0 = mean over the top n channels only.
                      An option to reduce dilution when an anomaly appears in only some channels.
        """
        e = self.channel_error(x, c)
        if channel_norm is not None:
            e = e / (channel_norm.to(e.device) + 1e-12)
        if topk and 0 < topk < e.shape[1]:
            e = e.topk(topk, dim=1).values
        return e.mean(dim=1)
