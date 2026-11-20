# Cloud/FL-FedAvg/aggregator.py
"""
FedAvg aggregation.

[Weighting scheme]
Standard FedAvg weights clients in proportion to their sample counts, because the goal
is to approximate what training on centrally pooled data would give.

This demo does not do that. The participants are 'sites', and each site has a different
number of robots (A: 4, B: 2, C: 2). More robots means more windows pile up in the
same time, so weighting by sample count lets the site with more robots pull the shared
model harder. Robot count is just that factory's circumstance, not a basis for voting power.

So every site gets the same weight. This is common when federating at the
institution level (cross-silo FL). It balances contributions without throwing away data.

To go back to the standard scheme, run with FL_AGG_WEIGHTING=samples.
"""
from __future__ import annotations

import os
from typing import Dict, List, Tuple

import torch

StateDict = Dict[str, torch.Tensor]

# "uniform" equal weight per site (default)
# "samples" proportional to sample count — standard FedAvg
WEIGHTING = os.getenv("FL_AGG_WEIGHTING", "uniform").strip().lower()

# Whether to exclude BatchNorm layers from averaging (FedBN). See config.py.
# Kept in config because the site app (edge_node.py) needs the same value.
from config import FEDBN


def bn_prefixes(state) -> tuple:
    """
    Find the prefixes of BatchNorm layers in a state_dict.

    running_mean exists only in BatchNorm, so that key identifies the layer.
    No need to know the model class, and it keeps working if layer names change.
    """
    return tuple(k[: -len("running_mean")]
                 for k in state if k.endswith("running_mean"))


class Aggregator:
    """FedAvg Aggregator (model-agnostic)"""

    def __init__(self, weighting: str | None = None):
        self.weighting = (weighting or WEIGHTING).lower()
        if self.weighting not in ("uniform", "samples"):
            raise ValueError(
                f"FL_AGG_WEIGHTING 은 uniform 또는 samples 여야 한다: "
                f"{self.weighting}"
            )

    def _weights(self, counts: List[int]) -> List[float]:
        """
        Decide the aggregation weights, normalized to sum to 1.

        uniform: split evenly over participating nodes. Sample counts are ignored.
                 Nodes with zero samples are excluded, though, since they could not train.
        samples: proportional to sample count. Standard FedAvg.
        """
        if self.weighting == "samples":
            total = sum(counts)
            if total == 0:
                raise ValueError("Total samples is zero")
            return [c / total for c in counts]

        active = [1.0 if c > 0 else 0.0 for c in counts]
        total = sum(active)
        if total == 0:
            raise ValueError("학습한 노드가 하나도 없다")
        return [a / total for a in active]

    def aggregate(
        self,
        client_states: List[Tuple[StateDict, int]],
    ) -> StateDict:
        """
        Aggregate by weighted average.

        client_states: [(state_dict, num_samples), ...]
        Returns: the aggregated global state_dict
        """
        if not client_states:
            raise ValueError("No client states to aggregate")

        states = [state for state, _ in client_states]
        counts = [int(n) for _, n in client_states]
        weights = self._weights(counts)

        global_state: StateDict = {}
        bn = bn_prefixes(states[0]) if FEDBN else ()
        for key in states[0].keys():
            ref = states[0][key]
            if bn and key.startswith(bn):
                # BatchNorm stays at each site. The values stored here are used only when
                # evaluating the global model; each site keeps using its own.
                global_state[key] = ref.clone()
                continue
            if not torch.is_floating_point(ref):
                # Averaging integer buffers (num_batches_tracked etc.) breaks their meaning.
                # Use the first node's value as is.
                global_state[key] = ref.clone()
                continue
            global_state[key] = sum(
                state[key] * w for state, w in zip(states, weights)
            )

        return global_state

    def describe(self, client_states: List[Tuple[StateDict, int]]) -> str:
        """Describe the aggregation weights in one line. Used in logs and the dashboard."""
        counts = [int(n) for _, n in client_states]
        try:
            weights = self._weights(counts)
        except ValueError as e:
            return f"집계 불가: {e}"
        parts = [f"n={c} w={w:.3f}" for c, w in zip(counts, weights)]
        label = ("사업장 균등" if self.weighting == "uniform"
                 else "샘플 수 비례")
        return f"{label}  " + "  ".join(parts)
