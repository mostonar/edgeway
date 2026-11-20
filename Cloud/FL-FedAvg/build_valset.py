"""
Cloud/FL-FedAvg/build_valset.py — builds a fixed validation set from Isaac Sim jsonl captures.

Why build it separately
  prepare_data_amr.py builds per-site 'training' data. The validation set has a different
  purpose and different requirements. It must resemble the condition distribution seen in operation,
  and anomaly labels must come from 'the fact of injection', not from the signal.

Three things to get right
  (a) Condition distribution   Put a floor/cap on each cond cell (nav x speed x load)
                   so no single cell dominates. The current validation set
                   fills only 7 of 45 cells and is 98.7% straight.
  (b) Labels        Use the fault field in the CIN if present. Otherwise a person
                   gives 'which robot / which interval' directly via --anomaly.
                   Deriving labels from the signal (e.g. lift.eff > 1e4) becomes a tautology
                   where looking at that channel alone gives AUROC 1.0. Forbidden.
  (c) cond width     Use amr_schema.COND_DIM as is. Do not hard-code numbers.

Usage
  python3 Cloud/FL-FedAvg/build_valset.py CAPTURE_DIR -o Cloud/FL-FedAvg/valset_amr.npz \
      --anomaly siteC_AMR_2:240-   # anomalous from 240 s to the end
"""
from __future__ import annotations
import argparse, collections, glob, json, os, sys
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import amr_schema as sch


def parse_anomaly(specs):
    """'siteC_AMR_2:240-380' -> ('siteC_AMR_2', 240.0, 380.0). An empty end means infinity."""
    out = []
    for s in specs or []:
        robot, _, rng = s.partition(":")
        lo, _, hi = rng.partition("-")
        out.append((robot.strip(),
                    float(lo) if lo.strip() else 0.0,
                    float(hi) if hi.strip() else float("inf")))
    return out


def load_capture(d):
    """Bundle of jsonl -> list of windows. Whatever window_from_con filters out is skipped as is."""
    rows, skipped = [], 0
    for f in sorted(glob.glob(os.path.join(d, "*.jsonl"))):
        for line in open(f, encoding="utf-8"):
            if not line.strip():
                continue
            obj = json.loads(line)
            con = (obj.get("con") or {})
            sig = sch.window_from_con(con)
            if sig is None:
                skipped += 1
                continue
            # Read the ground truth from the CIN's lbl. Same rules as amr_ingest._labels_of.
            #   lbl: ["cond:bearing", "run:...", "load:250", ...]
            kv = {x.split(":", 1)[0]: x.split(":", 1)[1]
                  for x in (obj.get("lbl") or []) if ":" in x}
            ts = [x for x in (con.get("t_sim") or []) if x is not None]
            rows.append(dict(
                robot=str(con.get("robot") or "?"),
                nav=str(con.get("nav") or ""),
                seq=int(con.get("seq") or 0),
                t=float(np.mean(ts)) if ts else float("nan"),
                # Ground truth can come from two places. The cond in lbl takes priority.
                cond=kv.get("cond"),
                load_lbl=kv.get("load"),
                fault=con.get("fault"),
                sig=sig,
            ))
    return rows, skipped


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("capture", help="folder containing jsonl files")
    ap.add_argument("-o", "--out", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "valset_amr.npz"))
    ap.add_argument("--anomaly", action="append",
                    help="ROBOT:START-END (seconds). Can be repeated")
    ap.add_argument("--min-cell", type=int, default=10,
                    help="condition cell minimum. Smaller cells are dropped")
    ap.add_argument("--max-cell", type=int, default=40,
                    help="condition cell maximum. Larger cells are randomly downsampled")
    ap.add_argument("--only", default="",
                    help="use only robots with this prefix (e.g. siteC). "
                         "Including sites used to train the global model "
                         "inflates scores, so normally use only local-only "
                         "sites")
    ap.add_argument("--seed", type=int, default=0)
    a = ap.parse_args()

    rows, skipped = load_capture(a.capture)
    if not rows:
        print(f"✗ {a.capture} : no usable windows")
        return 1
    print(f"input {a.capture}: {len(rows)} windows (skipped {skipped})")

    X = np.stack([r["sig"] for r in rows]).astype(np.float32)
    robot = np.array([r["robot"] for r in rows])
    nav = np.array([r["nav"] for r in rows])
    t = np.array([r["t"] for r in rows])
    seq = np.array([r["seq"] for r in rows])

    # ── Labels ─────────────────────────────────────────────────
    # Priority: cond in lbl -> fault in con -> --anomaly
    fault_name = np.array([r["cond"] or "" for r in rows])
    has_cond = any(fault_name != "")
    has_field = any(r["fault"] is not None for r in rows)
    if has_cond:
        y = (fault_name != "normal").astype(np.int64)
        # exclude marks intervals Isaac Sim flagged as 'do not use'. Dropped entirely.
        drop_ex = fault_name == "exclude"
        print(f"  labels: cond from lbl. "
              + ", ".join(f"{k} {v}" for k, v in
                          collections.Counter(fault_name.tolist()).most_common()))
    elif has_field:
        y = np.array([1 if r["fault"] else 0 for r in rows], dtype=np.int64)
        print(f"  labels: CIN fault field (anomaly {int(y.sum())} windows)")
    else:
        spec = parse_anomaly(a.anomaly)
        if not spec:
            print("✗ No labels. Without a CIN fault field, use --anomaly "
                  "to mark which robot and interval are anomalous.")
            print("  Labels are not derived from signals: a model seeing that "
                  "signal would always be right, making the measure meaningless.")
            return 1
        y = np.zeros(len(rows), dtype=np.int64)
        for rb, lo, hi in spec:
            m = (robot == rb) & (t >= lo) & (t <= hi)
            y[m] = 1
            print(f"  labels: {rb} {lo:.0f}~{hi if hi < 1e9 else float('inf'):.0f}s"
                  f" -> anomaly {int(m.sum())} windows")

    if not has_cond:
        fault_name = np.where(y == 1, "anomaly", "normal")
        drop_ex = np.zeros(len(rows), dtype=bool)

    # ── Filter robots ─────────────────────────────────────────
    # Putting a training site into the validation set means scoring on 'data already seen',
    # which inflates the score. mn3 (site C) is local-only and does not upload weights to the
    # global model, so scoring on its data avoids leakage.
    inc = np.ones(len(rows), dtype=bool)
    if a.only:
        inc = np.array([r.startswith(a.only) for r in robot])
        print(f"  robot filter '{a.only}': kept {int(inc.sum())}/{len(rows)} windows")

    # ── Driving only. Stopped windows are easy to reconstruct and make evaluation easy ──
    mov = sch.moving_mask(X)
    keep = mov & inc & ~drop_ex
    print(f"  moving {int(mov.sum())} / stopped {int((~mov).sum())} -> moving only")

    # ── Balance condition cells ───────────────────────────────
    cond = np.stack([sch.cond_vector(n, sch.speed_of(x), x)
                     for n, x in zip(nav, X)]).astype(np.float32)
    g = np.array([sch.nav_group(n) for n in nav])
    sb = np.array([sch.speed_bucket(sch.speed_of(x)) for x in X])
    ld = np.array([sch.load_state(n, x) for n, x in zip(nav, X)])
    cell = g * 9 + sb * 3 + ld

    rng = np.random.default_rng(a.seed)
    sel = np.zeros(len(rows), dtype=bool)
    dropped = kept = 0
    # Balance normal and anomalous separately. Anomalies are few, so they are not capped.
    for c in np.unique(cell[keep]):
        idx = np.flatnonzero(keep & (cell == c))
        nrm = idx[y[idx] == 0]
        ano = idx[y[idx] == 1]
        if len(nrm) < a.min_cell:
            dropped += len(idx)
            continue
        if len(nrm) > a.max_cell:
            nrm = rng.choice(nrm, a.max_cell, replace=False)
        sel[nrm] = True
        sel[ano] = True
        kept += 1
    print(f"  condition cells: {kept} kept, {dropped} windows dropped "
          f"(cell min {a.min_cell} / max {a.max_cell})")

    if len(np.unique(y[sel])) < 2:
        print("✗ Normal or anomaly class is empty. valset.load() will reject it.")
        return 1

    # ── Save ──────────────────────────────────────────────────
    os.makedirs(os.path.dirname(a.out) or ".", exist_ok=True)
    np.savez_compressed(
        a.out,
        signals=X[sel], cond=cond[sel], labels=y[sel],
        robot=robot[sel], nav=nav[sel], seq=seq[sel],
        # Anomaly type. Difficulty varies completely by type (measured AUROC
        # floor 0.95 / wknob 0.69 / bearing 0.55), so looking only at a single combined number
        # hides which one is failing.
        fault_type=fault_name[sel],
        speed=np.array([sch.speed_of(x) for x in X[sel]], dtype=np.float32),
    )

    # ── Report results ────────────────────────────────────────
    yy = y[sel]
    print(f"\nsaved: {a.out}")
    print(f"  {int(sel.sum())} windows   normal {int((yy==0).sum())} / "
          f"anomaly {int((yy==1).sum())} ({yy.mean():.1%})")
    print(f"  cond width {cond.shape[1]}   robots {sorted(set(robot[sel].tolist()))}")
    for name, arr, labels in (("nav group", g[sel], sch.NAV_GROUPS),
                              ("speed", sb[sel], ["slow", "medium", "fast"]),
                              ("load", ld[sel], sch.LOAD_STATES)):
        c = collections.Counter(arr.tolist())
        print(f"  {name}: " + " / ".join(
            f"{labels[k]} {v} ({v/sel.sum():.0%})" for k, v in sorted(c.items())))
    ft = collections.Counter(fault_name[sel].tolist())
    print(f"  anomaly types: " + " / ".join(f"{k} {v}" for k, v in ft.most_common()))
    cc = collections.Counter(cell[sel].tolist())
    print(f"  condition cells {len(cc)}/45, {sum(1 for v in cc.values() if v>=10)} with >=10 windows")

    # Runs of 3 consecutive. The verdict is K=3 consecutive, so this count is the real detection opportunity.
    runs = []
    for rb in sorted(set(robot[sel].tolist())):
        m = robot[sel] == rb
        o = np.argsort(seq[sel][m])
        v = yy[m][o]
        cur = 0
        for b in v:
            if b:
                cur += 1
            else:
                if cur:
                    runs.append(cur)
                cur = 0
        if cur:
            runs.append(cur)
    runs = np.array(runs) if runs else np.array([0])
    print(f"  anomaly runs {len(runs)}, longest {runs.max()}, "
          f"{int((runs>=3).sum())} runs of 3+  <- actual detection chances")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
