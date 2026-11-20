"""
Cloud/FL-FedAvg/config.py — unified FL + oneM2M settings (AMR floor/robot inspection service)

- 3 sites
  - mn1 = site A : 4 robots, participates in training
  - mn2 = site B : 2 robots, participates in training
  - mn3 = site C : 4 robots, receives the shared model and fine-tunes on its own data only.
                     Does not contribute to FedAvg (FL_LOCAL_ONLY_NODES)
- Anomaly detection based on a conditional 1D CNN VAE (model_cvae.CVAE1D)
- Trained on normal driving data only

[CSE layout]
Site CSEs (MN-CSE) and the central CSE (IN-CSE) are separate.
MN-CSEs are on on-site MEC hosts, the IN-CSE is in the cloud.
AEs are registered on the Isaac Sim side, so this code does not create AEs.
"""

import os

import amr_schema as _sch   # Single source of truth for the condition vector dimension
from dataclasses import dataclass


# ════════════════════════════════════════════════════════
# AE model settings
# ════════════════════════════════════════════════════════

@dataclass(frozen=True)
class AEConfig:
    """Conditional 1D CNN VAE spec. Must match the output of prepare_data_amr.py."""

    # 60 Hz × 1 s = one window
    seq_len: int = 60

    # 24 IMU channels + 14 encoder channels
    n_channels: int = 38

    # Condition vector: nav group (5) + speed bucket (3). Must equal amr_schema.COND_DIM.
    # Set by amr_schema. nav 5 + speed 3 + load 3 = 11.
    # Hard-coding a number here silently breaks when the schema grows.
    cond_dim: int = _sch.COND_DIM

    # Latent dim and hidden channels. This combination gives 90,806 parameters,
    # about 217 KB with fp16+gzip+base64, so it fits in one contentInstance.
    latent_dim: int = 24
    hidden: int = 48

    # KL weight in the ELBO
    beta: float = 0.5

    # Anomaly score aggregation: 0 = mean over all channels, n>0 = mean of the top n channels
    score_topk: int = 0


# ════════════════════════════════════════════════════════
# Training settings
# ════════════════════════════════════════════════════════

@dataclass(frozen=True)
class TrainConfig:
    rounds: int = 10
    local_epochs: int = 10
    # Minimum optimizer steps per round. Epochs are increased to reach this.
    #
    # With about 50 windows per round, batch 32 for 10 epochs is only ~20 steps.
    # In the 2026-11-19 run, for all 10 rounds the global model's validation score
    # matched the 'all-zero output' value to three decimal places (AUROC 0.641).
    # The model learned nothing. 0 means use local_epochs as is.
    min_steps: int = int(os.getenv("FL_MIN_STEPS", "200"))
    batch_size: int = 32
    lr: float = 1e-3
    weight_decay: float = 1e-4
    device: str = "cpu"
    seed: int = 42
    log_every: int = 1

    # Legacy fields
    train_split: float = 0.8
    hidden_size: int = 128
    num_classes: int = 2


# ════════════════════════════════════════════════════════
# Data settings
# ════════════════════════════════════════════════════════

# PKL directory produced by prepare_data_amr.py.
AMR_DATA_DIR = os.getenv(
    "FL_PKL_DIR",
    "/tmp/fl_data/amr",
)

# Backward-compatible alias (old code may reference it)


NODE_DATA_FILES = {
    "mn1": os.path.join(AMR_DATA_DIR, "mn1.pkl"),
    "mn2": os.path.join(AMR_DATA_DIR, "mn2.pkl"),
    "mn3": os.path.join(AMR_DATA_DIR, "mn3.pkl"),
}

# Site names and robot counts (must match prepare_data_amr.py)
SITE_OF_NODE = {"mn1": "siteA", "mn2": "siteB", "mn3": "siteC"}
ROBOTS_PER_NODE = {"mn1": 4, "mn2": 2, "mn3": 2}

# Container name Isaac Sim creates under each robot AE.
# Sensor data accumulates under the robot AE, not the MN-AE.
#   <MN-CSE>/siteA_AMR_1/telemetry/<cin>
ROBOT_TELEMETRY_CNT = os.getenv("FL_ROBOT_TELEMETRY_CNT", "telemetry")
ROBOT_STATUS_CNT = os.getenv("FL_ROBOT_STATUS_CNT", "status")


def robot_names(node: str):
    """List of robot AE names for one site. siteA_AMR_1 … siteA_AMR_4

    This is the fallback when discovery fails. Normally
    what telemetry_labels() finds is used.
    """
    site = SITE_OF_NODE.get(node, node)
    return [
        f"{site}_AMR_{i}"
        for i in range(1, ROBOTS_PER_NODE.get(node, 0) + 1)
    ]


# ── Resource labels ──────────────────────────────────────────
#
# Hard-coding paths means editing code every time robots are added or the naming
# rule changes. Attaching labels and finding them via discovery (?fu=1&lbl=...)
# reads whatever Isaac Sim uploaded at that moment.
#
# Labels the Isaac Sim side attaches to the telemetry container (agreed format):
#   ["site:siteC", "robot:AMR_2", "ae:siteC_AMR_2",
#    "type:telemetry", "siteC:telemetry"]
# We look up by two of these, site and type. The rest are for humans
# to read or for narrowing further.
LABEL_TYPE_TELEMETRY = os.getenv("FL_LBL_TELEMETRY", "type:telemetry")
# Set to 0 to turn discovery off and assemble paths the old way.
USE_DISCOVERY = os.getenv("FL_USE_DISCOVERY", "1") not in ("0", "false", "False")


def site_label(node: str) -> str:
    """'site:siteC'"""
    return f"site:{SITE_OF_NODE.get(node, node)}"


def telemetry_labels(node: str):
    """Label set (AND) that finds the site's telemetry container."""
    return [site_label(node), LABEL_TYPE_TELEMETRY]


# Site and type combined into one label. 'siteC:telemetry'
# Giving multiple lbl may be interpreted as OR depending on the CSE, so this is tried first.
# See the comment in onem2m_utils.discover_telemetry for details.
LABEL_SITE_TELEMETRY_FMT = os.getenv("FL_LBL_SITE_TELEMETRY", "{site}:telemetry")


def site_telemetry_label(node: str) -> str:
    """'siteC:telemetry'"""
    return LABEL_SITE_TELEMETRY_FMT.format(site=SITE_OF_NODE.get(node, node))


def resource_labels(node: str, kind: str):
    """Labels to attach to resources we create. kind is something like 'local-model'."""
    return [site_label(node), f"type:{kind}", f"node:{node}"]

CLASS_NAMES = ["normal", "anomaly"]
NUM_CLASSES = 2

# The first file where RMS exceeds this value is used as the fault onset
RMS_FAULT_THRESHOLD = 1.0


# ════════════════════════════════════════════════════════
# Anomaly detection settings
# ════════════════════════════════════════════════════════

# Fault is declared when reconstruction error exceeds the threshold K times in a row
ANOMALY_K_CONSECUTIVE = 3

# threshold = median + N x MAD x 1.4826  (not mean + Nσ)
#
# Until now edge_node.calibrate() ignored this value and used its own default 3.0.
# So changing this did not change the actual threshold. It is now wired up.
#
# The default depends on whether per-condition normalization (FL_COND_NORM) is on. With
# normalization, the median of training scores gathers around 1 and MAD shrinks with it. Keeping 3
# lowers the threshold and actually increases false alarms. Measured (1 site, eval 318 normal/133 anomalous):
#
#   No normalization  3MAD   thr 5.52   detect 99.2%   false alarm 48.7%
#   Normalized        3MAD   thr 3.58   detect 95.5%   false alarm 71.4%   <- worse
#   Normalized        8MAD   thr 7.87   detect 90.2%   false alarm 38.1%
#   Normalized       12MAD   thr 11.31  detect 82.7%   false alarm 28.0%
#
# Without normalization, raising only n_sigma to 8 gave detect 75.9% / false alarm 31.1%,
# so normalized 8 beats it on both detection and false alarms. Hence 8 is the
# default. For more detection use 5; to cut false alarms further use 12.
COND_NORM = os.getenv("FL_COND_NORM", "1") not in ("0", "false", "False")
THRESHOLD_N_SIGMA = float(
    os.getenv("FL_THRESHOLD_N_SIGMA", "8.0" if COND_NORM else "3.0"))

# Node that shows the anomaly state in the demo
ANOMALY_DEMO_NODE = os.getenv(
    "FL_ANOMALY_DEMO_NODE",
    "mn3",
)

# By default MN3 shows anomalous samples from Round 7.
ANOMALY_START_ROUND = int(
    os.getenv(
        "FL_ANOMALY_START_ROUND",
        "7",
    )
)


# ════════════════════════════════════════════════════════
# oneM2M connection settings
# ════════════════════════════════════════════════════════

# Uses the ACME CSE in the ETSI MEC Sandbox.
#
# Each site has its own MEP and its own MN-CSE.
#   mn1 = site A = mep-a
#   mn2 = site B = mep-b
#   mn3 = site C = mep-c
#
# Actual address format:
#   https://<host>/<sandbox>/<mep>/meep-acme-mn-cse/acme-cse-mn-<mep>-<sandbox>
#   └───────── base url ─────────┘ └──────── CSE name ────────┘
#
# The sandbox address is not in the code. This is a public repo, so it comes from environment variables.
#
#   export FL_SANDBOX_HOST=<host where the sandbox runs>
#   export FL_SANDBOX_NAME=<sandbox instance name>
#
# The host differs depending on where it runs.
#   Wired network       lab public IP
#   Same router Wi-Fi  private IP (192.168.x.x)
#   Tailnet            100.x.x.x  — the Isaac Sim PC is also on the tailnet, so this is usually easiest
#
# Put it in .env.local and source it from the shell. .gitignore blocks .env*.
#
# MN-CSEs register with the IN-CSE via the REMOTE_CSE_* settings (Mcc).
# The CSEs handle the site↔central link; this code attaches to each CSE over Mca.

SANDBOX_SCHEME = os.getenv("FL_SANDBOX_SCHEME", "https")
SANDBOX_HOST = os.getenv("FL_SANDBOX_HOST", "").strip()
SANDBOX_NAME = os.getenv("FL_SANDBOX_NAME", "").strip()

# Placeholder that goes into the URL when nothing is configured.
# .invalid is a domain reserved by RFC 2606, so name resolution always fails.
# Using an empty string as is produces addresses like "https:///...",
# which makes it hard to see why it failed.
_HOST_UNSET = "sandbox-host-not-set.invalid"
_NAME_UNSET = "sandbox-name-not-set"


# Used when the CSE requires HTTP basic auth.
#
# The sandbox MN-CSEs are open without auth, but the cloud IN-CSE returns 401.
# Credentials are not kept in code. This is a public repo, so they come only from environment variables.
# Same values as REMOTE_CSE_USER / REMOTE_CSE_PASS in edgeway.yaml.
IN_AUTH_USER = os.getenv("FL_IN_USER", "")
IN_AUTH_PASS = os.getenv("FL_IN_PASS", "")
MN_AUTH_USER = os.getenv("FL_MN_USER", "")
MN_AUTH_PASS = os.getenv("FL_MN_PASS", "")


def in_auth():
    """(user, pass) for the IN-CSE. None if not set."""
    return (IN_AUTH_USER, IN_AUTH_PASS) if IN_AUTH_USER else None


def mn_auth():
    """(user, pass) for the MN-CSE. None if not set."""
    return (MN_AUTH_USER, MN_AUTH_PASS) if MN_AUTH_USER else None


def sandbox_ready() -> bool:
    """Is the sandbox address configured?"""
    return bool(SANDBOX_HOST and SANDBOX_NAME)


def require_sandbox() -> None:
    """
    Called first by entry points that must connect to the sandbox.
    If not configured, say what to do and stop.

    It is not blocked at import time because file-only jobs such as
    prepare_data_amr.py must run without a sandbox.
    """
    if sandbox_ready():
        return
    raise SystemExit(
        "\nSandbox address is not set.\n"
        "  export FL_SANDBOX_HOST=<host>     # e.g. tailnet address or public IP\n"
        "  export FL_SANDBOX_NAME=<instance name>\n"
        "\n"
        "This is a public repo, so real addresses are not kept in code.\n"
        "You can also put them in .env.local and run `source .env.local`.\n"
    )

# If the sandbox uses an IP-based self-signed certificate, verification fails.
# Only then set it to 0. Turning it off leaves you open to man-in-the-middle attacks.
TLS_VERIFY = os.getenv("FL_TLS_VERIFY", "1") not in ("0", "false", "False")

MEP_OF_NODE = {"mn1": "mep-a", "mn2": "mep-b", "mn3": "mep-c"}


def mn_endpoint(node: str):
    """(base_url, cse_name) of a site node."""
    mep = MEP_OF_NODE.get(node, "mep-a")
    override_url = os.getenv(f"FL_MN_BASE_URL_{node.upper()}")
    override_cse = os.getenv(f"FL_MN_CSE_NAME_{node.upper()}")
    base = override_url or (
        f"{SANDBOX_SCHEME}://{SANDBOX_HOST or _HOST_UNSET}"
        f"/{SANDBOX_NAME or _NAME_UNSET}/{mep}/meep-acme-mn-cse"
    )
    cse = override_cse or f"acme-cse-mn-{mep}-{SANDBOX_NAME or _NAME_UNSET}"
    return base, cse


def in_endpoint():
    """(base_url, cse_name) of the central CSE."""
    base = os.getenv("FL_IN_BASE_URL") or (
        f"{SANDBOX_SCHEME}://{SANDBOX_HOST or _HOST_UNSET}"
        f"/{SANDBOX_NAME or _NAME_UNSET}/meep-acme-in-cse"
    )
    cse = os.getenv("FL_IN_CSE_NAME") or f"acme-cse-in-{SANDBOX_NAME or _NAME_UNSET}"
    return base, cse


# Default (for older code that assumes a single endpoint). Points to site A.
# On startup, each MN-AE calls onem2m_utils.set_endpoint() with its own node's endpoint.
MN_BASE_URL, MN_CSE_NAME = mn_endpoint("mn1")
MN_CSE_ID = os.getenv("FL_MN_CSE_ID", "")

IN_BASE_URL, IN_CSE_NAME = in_endpoint()
IN_CSE_ID = os.getenv("FL_IN_CSE_ID", "")

IN_IS_REMOTE = (IN_BASE_URL != MN_BASE_URL) or (IN_CSE_NAME != MN_CSE_NAME)

# Name referenced by older code. Points to the site CSE.
BASE_URL = MN_BASE_URL
CSE_NAME = MN_CSE_NAME

# AE name and AE-ID of the central coordinator app.
#
# The IN-CSE is a shared resource used by several teams. "IN-AE" is a name anyone
# might use, so it can be confused with someone else's or taken first. To change it, change both
# values together and re-run setup_resources_standard.py,
# because all the containers and ACPs hang under this name.
IN_AE_NAME = os.getenv("FL_IN_AE_NAME", "IN-AE")

# AEs are registered by the Isaac Sim side. setup_resources does not create them, only checks they exist.
CREATE_AES = os.getenv("FL_CREATE_AES", "0") in ("1", "true", "True")

# How long to wait for AEs to come up (seconds)
AE_WAIT_SEC = float(os.getenv("FL_AE_WAIT_SEC", "60"))

NOTIFY_HOST = os.getenv(
    "FL_NOTIFY_HOST",
    "127.0.0.1",
)

# Which path to receive notifications on.
#
#   "direct"  The CSE calls our HTTP server directly. The subscription nu is a URL and
#             the AE must have a poa set. Use when the CSE can reach the AE.
#   "pch"     Receive via oneM2M <pollingChannel> (m2m:pch, ty=15). The AE holds a long
#             request open to the CSE and the CSE sends notifications down in the response. This is what
#             TS-0001 defines for request-unreachable AEs.
#
# Subscriptions and notifications stay the same. Only the delivery direction is reversed.
#
# [Why two instead of one]
# Previously one flag decided every notification path. But in our deployment each path
# has different conditions.
#
#   Robot → MN-CSE → MN-AE     inside the site. The CSE can call us   → direct
#   IN-CSE → MN-AE / IN-AE    across a firewall. It cannot call us    → pch
#
# With one flag for both, turning on pch for the remote side also kills the in-site
# telemetry subscriptions that have nothing to do with the firewall (since poa is no longer set).
# Conversely, leaving it on direct for the local side makes every remote subscription time out.
#
# Different conditions call for different protocols. So it is split in two.
NOTIFY_LOCAL = os.getenv(
    "FL_NOTIFY_LOCAL", os.getenv("FL_NOTIFY_MODE", "direct")
).strip().lower()

NOTIFY_REMOTE = os.getenv(
    "FL_NOTIFY_REMOTE", os.getenv("FL_NOTIFY_MODE", "pch")
).strip().lower()


def local_notify_mode() -> str:
    """Delivery mode for notifications from the in-site MN-CSE."""
    return NOTIFY_LOCAL


def remote_notify_mode() -> str:
    """Delivery mode for notifications from the cloud IN-CSE."""
    return NOTIFY_REMOTE


def remote_uses_pch() -> bool:
    return NOTIFY_REMOTE == "pch"

NUM_CLIENTS = 3
# Number of federated learning rounds. Lower or raise it when experimenting.
GLOBAL_ROUNDS = int(os.getenv("FL_GLOBAL_ROUNDS", "10"))

ORIGINATOR = os.getenv("FL_ORIGINATOR", "CAdmin")

# Whether each AE sends requests to the CSE with its own AE-ID.
#
# When off, everything goes out as ORIGINATOR (default CAdmin). The admin identity passes every
# ACP, so even with policies in place nothing is actually blocked.
#
# When on, TRAINING-AE-1 requests as CTRAINING-AE-1 and IN-AE as CIN-AE. Only then do ACPs
# actually take effect and "site-to-site isolation" is more than words.
#
# But if a policy has a hole, operations get blocked from that moment. Turn this on
# after the robot AE-side ACPs are in place. Check first with setup_resources_standard.py --verify.
USE_AE_IDENTITY = os.getenv("FL_USE_AE_IDENTITY", "0") == "1"

# AE-ID. The value used in ACP acor and X-M2M-Origin.
# Defined in one place so ACPs and actual requests do not diverge.
IN_AE_ID = os.getenv("FL_IN_AE_ID", "CIN-AE")


def training_ae_name(node: str) -> str:
    """mn1 -> TRAINING-AE-1

    The site's training app. Previously MN-AE-i. The same app was registered on both MN-CSE
    and IN-CSE, so "MN" showed up in the IN-CSE tree and read as an app living
    there. Named by what it does, like the analysis app (ANALYSIS-AE-i).
    """
    return f"TRAINING-AE-{node[2:]}"


def training_ae_id(node: str) -> str:
    """mn1 -> CTRAINING-AE-1"""
    return f"C{training_ae_name(node)}"


# ── Analysis app (verdict) ────────────────────────────────
#
# In the design, apps inside a site split into analysis / operation / training. The training app
# is TRAINING-AE-i, and verdicts are the analysis app's job. For now the training app, which holds
# the model, also does the computation,
# but resources are left under the analysis app's identity. If the analysis app is later split into a separate
# process, the resource tree and subscriber-side code will not change.
#
# Verdicts do not leave the site, so they live on the MN-CSE.
ANALYSIS_AE_PREFIX = os.getenv("FL_ANALYSIS_AE_PREFIX", "ANALYSIS-AE")
VERDICT_CNT = os.getenv("FL_VERDICT_CNT", "cnt-verdict")
VERDICT_MNI = int(os.getenv("FL_VERDICT_MNI", "500"))

# Per-round window anomaly scores. The site app writes them under TRAINING-AE-i on its own
# MN-CSE and the dashboard reads them. These are site detection results, so they are not sent to the IN-CSE (drop box).
WINDOW_SCORES_CNT = os.getenv("FL_WINDOW_SCORES_CNT", "cnt-window-scores")
WINDOW_SCORES_MNI = int(os.getenv("FL_WINDOW_SCORES_MNI", "50"))
VERDICT_ENABLED = os.getenv("FL_VERDICT", "1") not in ("0", "false", "False")

# First round in which verdicts may be issued. From this round CINs are uploaded and alarms raised.
# With the default 6, nothing goes out in R1~R5; it starts at R6.
#
# Thresholds in early rounds are measured with a barely trained model, so verdicts
# are not meaningful. Where it becomes usable was measured with 8 seeds (2-node FedAvg):
#
#   R    AUROC    vs prev    thr dev    thr change rate
#   R1  0.6201        —         0.991        25%
#   R2  0.6631   +0.0431        0.381        12%
#   R3  0.6832   +0.0201        0.597        18%   <- the threshold actually jumps
#   R4  0.6917   +0.0085        0.483        15%
#   R5  0.6956   +0.0039        0.382        12%   <- gain < seed spread
#   R6  0.6907   -0.0049        0.295         9%
#   R10 0.7036   +0.0051        0.307         9%
#
# From R5 the round-to-round gain (+0.0039) becomes smaller than the seed-to-seed spread (±0.0066).
# So later changes cannot be told apart as training vs. seed luck. The threshold
# change rate also settles into the 9~13% band from R6. Up to R5 is treated as the
# warm-up window, and verdicts start from R6 after it.
#
# At first it was set to 3 based on AUROC alone, but R3 was the point where the threshold was the
# most unstable after R1. What decides the verdict is the threshold, not AUROC.
#
# Even after all 10 rounds the threshold change rate does not drop below 9%. 5 is
# "the start of the range where it stops getting worse", not "the point where it stops wobbling".
VERDICT_START_ROUND = int(os.getenv("FL_VERDICT_START_ROUND", "6"))


# Apps that will subscribe to verdicts. By default the per-site judge app (CJUDGE-AE-i).
#
# Robot AEs are created by Isaac and get their AE-IDs assigned by the CSE, so they are read separately,
# but these are names we choose, so they are built by rule.
JUDGE_AE_PREFIX = os.getenv("FL_JUDGE_AE_PREFIX", "JUDGE-AE")

# Whether to also grant CREATE to subscribers.
#
# Whether creating a <subscription> under a parent needs CREATE or works with RETRIEVE is
# up to the CSE implementation. Could not confirm it in the ACME docs. If it is needed and not granted, the subscription
# is blocked with 403; if granted when not needed, that app could forge verdicts.
# The judge app is our own component, so the default is 1; drop it to 0 once confirmed.
VERDICT_SUB_CREATE = os.getenv("FL_VERDICT_SUB_CREATE", "1") not in (
    "0", "false", "False")


def judge_ae_name(node: str) -> str:
    """mn3 -> JUDGE-AE-3"""
    return f"{JUDGE_AE_PREFIX}-{node[2:]}"


def judge_ae_id(node: str) -> str:
    """mn3 -> CJUDGE-AE-3"""
    return f"C{judge_ae_name(node)}"


def verdict_subscribers(node: str) -> list:
    """
    AE-IDs allowed to subscribe to this site's verdicts.
    More can be added comma-separated via FL_VERDICT_SUBSCRIBERS.
    """
    extra = [x.strip() for x in
             os.getenv("FL_VERDICT_SUBSCRIBERS", "").split(",") if x.strip()]
    return list(dict.fromkeys([judge_ae_id(node)] + extra))


def analysis_ae_name(node: str) -> str:
    """mn1 -> ANALYSIS-AE-1"""
    return f"{ANALYSIS_AE_PREFIX}-{node[2:]}"


def analysis_ae_id(node: str) -> str:
    """mn1 -> CANALYSIS-AE-1"""
    return f"C{analysis_ae_name(node)}"


# Whether to run the IN-AE ACP self-check.
#
# This check connects directly to the 3 site MN-CSEs to verify permissions. Only possible
# when the IN-AE is on the same network as the sites.
#
# By design the IN-AE runs on the IN node (central). From there the site MN-CSEs are behind a firewall
# and unreachable, so this check is turned off. Turning it off disables a diagnostic, not a feature,
# so federated learning is unaffected.
VERIFY_ACP = os.getenv("FL_VERIFY_ACP", "1") not in ("0", "false", "False")

# X-M2M-RI (Request Identifier) is mandatory on every request in the TS-0009 HTTP binding.
# It must be unique per request, so no fixed value is kept here;
# onem2m_utils._headers() generates a new one each time.
# Common headers. Content-Type is not included.
#
# In TS-0009 the ty parameter of Content-Type is attached only to CREATE. Sending ty=4 on
# body-less requests like RETRIEVE or DELETE makes the CSE reject them.
# ty=4 used to be hard-coded here as a default, and every GET failed.
# It is attached only to requests that need it, via onem2m_utils._headers(ty=...).
HEADERS = {
    "X-M2M-Origin": ORIGINATOR,
    "X-M2M-RVI": os.getenv("FL_RVI", "3"),
    "Accept": "application/json",
}

# contentInfo to declare when putting a JSON string into a contentInstance con
CIN_CONTENT_INFO = os.getenv("FL_CIN_CNF", "application/json:0")


# ════════════════════════════════════════════════════════
# Model save path
# ════════════════════════════════════════════════════════

MODEL_BASE_DIR = os.getenv(
    "FL_MODEL_BASE_DIR",
    "/tmp/fl_models",
)

LOCAL_MODEL_DIR = os.path.join(
    MODEL_BASE_DIR,
    "local",
)

GLOBAL_MODEL_DIR = os.path.join(
    MODEL_BASE_DIR,
    "global",
)

CACHE_MODEL_DIR = os.path.join(
    MODEL_BASE_DIR,
    "cache",
)

os.makedirs(
    LOCAL_MODEL_DIR,
    exist_ok=True,
)

os.makedirs(
    GLOBAL_MODEL_DIR,
    exist_ok=True,
)

os.makedirs(
    CACHE_MODEL_DIR,
    exist_ok=True,
)


# ════════════════════════════════════════════════════════
# DP-SGD settings
# ════════════════════════════════════════════════════════

# Whether to enable DP. Off by default.
#
# The current method clips gradients per batch (C=1.5) and then adds Gaussian
# noise per batch. With about 90k parameters and σ≈0.33, the noise norm is about 4.7 at batch
# 32 and about 8.3 at batch 18, i.e. 3~8x the clipped gradient (≤1.5).
# The gradient was buried in noise and nothing was learned. Also, clipping is not per-sample,
# so the ε=12 guarantee does not hold either.
#
# First check whether training beats the 'no model' baseline without noise.
# FL_DP=1 turns the old method on (for comparison).
DP_ENABLED = os.getenv("FL_DP", "0") not in ("0", "false", "False", "")
DP_EPSILON = 12.0
DP_DELTA = 5e-4
DP_MAX_GRAD_NORM = 1.5

# Whether to exclude BatchNorm layers from averaging (FedBN).
#
# The coordinator (aggregator.py) excludes them from averaging, and the site app (edge_node.py) keeps its own
# BatchNorm when loading the global model. Both need the same value, so it lives here.
#
# [Why]
# BatchNorm normalizes with the batch mean/variance and accumulates those stats in running_mean /
# running_var. Those are statistics of that site's data. Floors and robots differ
# per site, so the statistics differ too. Averaging them gives values that belong to
# no site.
#
# This codebase already normalizes inputs per site. Switching to shared normalization as a test
# dropped AUROC 0.6752 -> 0.5772 in 2-node FedAvg, and per-site won all 50
# comparisons. BatchNorm statistics are the same kind of thing, yet they alone were being averaged.
#
# Measured (2-node FedAvg, 4 seeds): averaging gives 10-round mean 0.6745 / R10 0.6850,
# excluding gives 0.6815 / 0.7080. False alarms 41 -> 36. The difference is small but consistent,
# and base flattens out at R3 while FedBN keeps rising through R10.
#
# FL_FEDBN=0 restores the old behavior (average everything).
FEDBN = os.getenv("FL_FEDBN", "1") not in ("0", "false", "False")


# ════════════════════════════════════════════════════════
# Cold-start node settings
# ════════════════════════════════════════════════════════

# There are three node roles.
#
#   Training participant : local training + contributes to FedAvg   (default)
#   Local-only           : trains locally but does not contribute    FL_LOCAL_ONLY_NODES
#   Observe-only         : no training, inference only               FL_COLD_START_NODES
#
# Site C (mn3) is 'local-only'. It receives the shared model and fine-tunes it on its own normal data,
# stabilizing over time, but does not upload weights to the center.
# Observe-only is for demoing a stricter new site that does not use its own data either.
COLD_START_NODES = [
    n.strip()
    for n in os.getenv("FL_COLD_START_NODES", "").split(",")
    if n.strip()
]

LOCAL_ONLY_NODES = [
    n.strip()
    for n in os.getenv("FL_LOCAL_ONLY_NODES", "mn3").split(",")
    if n.strip()
]

ALL_NODES = [f"mn{i}" for i in range(1, NUM_CLIENTS + 1)]

# Only nodes that contribute to FedAvg are 'training nodes'.
# Local-only nodes train but are not waited on for aggregation.
NON_CONTRIBUTING_NODES = list(
    dict.fromkeys(COLD_START_NODES + LOCAL_ONLY_NODES)
)

TRAINING_NODES = [
    n for n in ALL_NODES if n not in NON_CONTRIBUTING_NODES
]


# ════════════════════════════════════════════════════════
# Sensor data source
# ════════════════════════════════════════════════════════

# If True, read the actual signal from contentInstances on the MN-CSE.
# If False, read the local pkl path (data_path) carried in con, as before.
# The path method only works when the robot/simulator and the training app are on the same machine.
FL_SENSOR_FROM_CSE = os.getenv(
    "FL_SENSOR_FROM_CSE",
    "1",
) not in ("0", "false", "False")


# ════════════════════════════════════════════════════════
# oneM2M weight transfer settings
# ════════════════════════════════════════════════════════

# Model weights are always sent as base64 inside the contentInstance con.
# The IN-AE and site apps are on different machines, so file paths cannot be exchanged.

# Whether to shrink weights to fp16 before sending. Halves the transfer size.
# If accuracy is sensitive, set it to 0 to turn it off.
FL_WEIGHTS_FP16 = os.getenv(
    "FL_WEIGHTS_FP16",
    "1",
) not in ("0", "false", "False")

# Upper bound (bytes) of base64 weights to put in a single contentInstance.
# If exceeded, embedding is abandoned and it falls back to passing a path.
#
# Set generously for ACME. CVAE1D weights are about 217 KB in base64, so they pass.
# If this is set larger than the CSE's real payload limit, CIN creation fails and
# the whole round stalls.
FL_MAX_CIN_BYTES = int(
    os.getenv(
        "FL_MAX_CIN_BYTES",
        str(4 * 1024 * 1024),
    )
)

# Whether to split weights into chunks when they exceed the limit above.
#
# Off (0) by default. ACME has no small payload limit, and CVAE1D weights are
# about 217 KB in base64, so they fit in one contentInstance as is.
# Turn on only when moving to a CSE with a small payload limit (verified with a dedicated container per
# chunk, reading each one's la: 39 chunks, upload 0.8 s / retrieve 0.1 s).
FL_CHUNK_WEIGHTS = os.getenv(
    "FL_CHUNK_WEIGHTS",
    "0",
) not in ("0", "false", "False")


# ════════════════════════════════════════════════════════
# Settings instance
# ════════════════════════════════════════════════════════

TRAIN_CFG = TrainConfig()
AE_CFG = AEConfig()