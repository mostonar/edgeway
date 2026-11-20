"""
Cloud/FL-FedAvg/build_valset.py — 아이작심 jsonl 캡처에서 고정 검증셋을 만든다.

왜 따로 만드나
  prepare_data_amr.py 는 사업장별 '학습' 데이터를 만든다. 검증셋은 목적이
  달라서 요구가 다르다. 운영에서 실제로 나오는 조건 분포를 닮아야 하고,
  이상 라벨이 신호가 아니라 '주입 사실' 에서 와야 한다.

세 가지를 맞춘다
  (가) 조건 분포   cond 칸(nav x speed x 적재)마다 하한/상한을 둬서
                   한 칸이 전체를 좌우하지 않게 한다. 지금 검증셋은
                   45칸 중 7칸만 차 있고 직진이 98.7% 다.
  (나) 라벨        CIN 에 fault 필드가 있으면 그걸 쓴다. 없으면 사람이
                   --anomaly 로 '어느 로봇 / 어느 구간' 을 직접 준다.
                   신호에서 라벨을 뽑으면(예: lift.eff > 1e4) 그 채널만
                   봐도 AUROC 1.0 이 나오는 동어반복이 된다. 금지한다.
  (다) cond 폭     amr_schema.COND_DIM 을 그대로 쓴다. 숫자를 박지 않는다.

쓰는 법
  python3 Cloud/FL-FedAvg/build_valset.py CAPTURE_DIR -o Cloud/FL-FedAvg/valset_amr.npz \
      --anomaly siteC_AMR_2:240-   # 240초부터 끝까지가 이상
"""
from __future__ import annotations
import argparse, collections, glob, json, os, sys
import numpy as np

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import amr_schema as sch


def parse_anomaly(specs):
    """'siteC_AMR_2:240-380' -> ('siteC_AMR_2', 240.0, 380.0). 끝이 비면 무한."""
    out = []
    for s in specs or []:
        robot, _, rng = s.partition(":")
        lo, _, hi = rng.partition("-")
        out.append((robot.strip(),
                    float(lo) if lo.strip() else 0.0,
                    float(hi) if hi.strip() else float("inf")))
    return out


def load_capture(d):
    """jsonl 묶음 -> 창 리스트. window_from_con 이 거르는 건 그대로 건너뛴다."""
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
            # CIN 의 lbl 에서 정답을 읽는다. amr_ingest._labels_of 와 같은 규칙.
            #   lbl: ["cond:bearing", "run:...", "load:250", ...]
            kv = {x.split(":", 1)[0]: x.split(":", 1)[1]
                  for x in (obj.get("lbl") or []) if ":" in x}
            ts = [x for x in (con.get("t_sim") or []) if x is not None]
            rows.append(dict(
                robot=str(con.get("robot") or "?"),
                nav=str(con.get("nav") or ""),
                seq=int(con.get("seq") or 0),
                t=float(np.mean(ts)) if ts else float("nan"),
                # 정답은 두 곳에서 올 수 있다. lbl 의 cond 가 우선이다.
                cond=kv.get("cond"),
                load_lbl=kv.get("load"),
                fault=con.get("fault"),
                sig=sig,
            ))
    return rows, skipped


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("capture", help="jsonl 들이 있는 폴더")
    ap.add_argument("-o", "--out", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "valset_amr.npz"))
    ap.add_argument("--anomaly", action="append",
                    help="ROBOT:START-END (초). 여러 번 줄 수 있다")
    ap.add_argument("--min-cell", type=int, default=10,
                    help="조건 칸 하한. 이보다 적은 칸은 버린다")
    ap.add_argument("--max-cell", type=int, default=40,
                    help="조건 칸 상한. 넘으면 무작위로 줄인다")
    ap.add_argument("--only", default="",
                    help="이 접두사로 시작하는 로봇만 쓴다 (예: siteC). "
                         "글로벌 모델 학습에 쓰이는 사업장을 검증셋에 "
                         "넣으면 점수가 부풀려지므로 기본은 로컬 전용 "
                         "사업장만 쓰는 것이다")
    ap.add_argument("--seed", type=int, default=0)
    a = ap.parse_args()

    rows, skipped = load_capture(a.capture)
    if not rows:
        print(f"✗ {a.capture} 에 쓸 창이 없다")
        return 1
    print(f"입력 {a.capture}: 창 {len(rows)}개 (건너뜀 {skipped})")

    X = np.stack([r["sig"] for r in rows]).astype(np.float32)
    robot = np.array([r["robot"] for r in rows])
    nav = np.array([r["nav"] for r in rows])
    t = np.array([r["t"] for r in rows])
    seq = np.array([r["seq"] for r in rows])

    # ── 라벨 ───────────────────────────────────────────────
    # 우선순위: lbl 의 cond -> con 의 fault -> --anomaly
    fault_name = np.array([r["cond"] or "" for r in rows])
    has_cond = any(fault_name != "")
    has_field = any(r["fault"] is not None for r in rows)
    if has_cond:
        y = (fault_name != "normal").astype(np.int64)
        # exclude 는 아이작심이 '쓰지 말라' 고 표시한 구간이다. 통째로 뺀다.
        drop_ex = fault_name == "exclude"
        print(f"  라벨: lbl 의 cond 사용. "
              + ", ".join(f"{k} {v}" for k, v in
                          collections.Counter(fault_name.tolist()).most_common()))
    elif has_field:
        y = np.array([1 if r["fault"] else 0 for r in rows], dtype=np.int64)
        print(f"  라벨: CIN 의 fault 필드 사용 (이상 {int(y.sum())}창)")
    else:
        spec = parse_anomaly(a.anomaly)
        if not spec:
            print("✗ 라벨이 없다. CIN 에 fault 필드가 없으면 --anomaly 로 "
                  "어느 로봇의 어느 구간이 이상인지 지정하라.")
            print("  신호에서 라벨을 만들지는 않는다. 그러면 그 신호를 보는 "
                  "모델이 무조건 맞히게 되어 측정이 무의미해진다.")
            return 1
        y = np.zeros(len(rows), dtype=np.int64)
        for rb, lo, hi in spec:
            m = (robot == rb) & (t >= lo) & (t <= hi)
            y[m] = 1
            print(f"  라벨: {rb} {lo:.0f}~{hi if hi < 1e9 else float('inf'):.0f}초"
                  f" -> 이상 {int(m.sum())}창")

    if not has_cond:
        fault_name = np.where(y == 1, "anomaly", "normal")
        drop_ex = np.zeros(len(rows), dtype=bool)

    # ── 로봇 걸러내기 ──────────────────────────────────────
    # 학습에 쓰는 사업장을 검증셋에 넣으면 '이미 본 데이터' 로 채점하게
    # 되어 점수가 부풀려진다. mn3(사업장 C)는 로컬 전용이라 글로벌 모델에
    # 가중치를 안 올리므로, 거기 데이터로 채점해야 누수가 없다.
    inc = np.ones(len(rows), dtype=bool)
    if a.only:
        inc = np.array([r.startswith(a.only) for r in robot])
        print(f"  로봇 필터 '{a.only}': {int(inc.sum())}/{len(rows)}창 남김")

    # ── 주행 중만. 정지 구간은 재구성이 쉬워 평가가 쉬워진다 ──
    mov = sch.moving_mask(X)
    keep = mov & inc & ~drop_ex
    print(f"  주행 {int(mov.sum())} / 정지 {int((~mov).sum())} -> 주행만 쓴다")

    # ── 조건 칸 균형 ───────────────────────────────────────
    cond = np.stack([sch.cond_vector(n, sch.speed_of(x), x)
                     for n, x in zip(nav, X)]).astype(np.float32)
    g = np.array([sch.nav_group(n) for n in nav])
    sb = np.array([sch.speed_bucket(sch.speed_of(x)) for x in X])
    ld = np.array([sch.load_state(n, x) for n, x in zip(nav, X)])
    cell = g * 9 + sb * 3 + ld

    rng = np.random.default_rng(a.seed)
    sel = np.zeros(len(rows), dtype=bool)
    dropped = kept = 0
    # 정상과 이상을 따로 균형 잡는다. 이상은 수가 적어 상한을 안 건다.
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
    print(f"  조건 칸: {kept}개 채택, {dropped}창 버림 "
          f"(칸 하한 {a.min_cell} / 상한 {a.max_cell})")

    if len(np.unique(y[sel])) < 2:
        print("✗ 정상 또는 이상 한쪽이 비었다. valset.load() 가 거부한다.")
        return 1

    # ── 저장 ───────────────────────────────────────────────
    os.makedirs(os.path.dirname(a.out) or ".", exist_ok=True)
    np.savez_compressed(
        a.out,
        signals=X[sel], cond=cond[sel], labels=y[sel],
        robot=robot[sel], nav=nav[sel], seq=seq[sel],
        # 이상 종류. 종류마다 난이도가 완전히 달라서(실측 AUROC
        # floor 0.95 / wknob 0.69 / bearing 0.55) 하나로 합친 숫자만
        # 보면 어느 쪽이 안 되는지 안 보인다.
        fault_type=fault_name[sel],
        speed=np.array([sch.speed_of(x) for x in X[sel]], dtype=np.float32),
    )

    # ── 결과 보고 ──────────────────────────────────────────
    yy = y[sel]
    print(f"\n저장: {a.out}")
    print(f"  {int(sel.sum())}창   정상 {int((yy==0).sum())} / "
          f"이상 {int((yy==1).sum())} ({yy.mean():.1%})")
    print(f"  cond 폭 {cond.shape[1]}   로봇 {sorted(set(robot[sel].tolist()))}")
    for name, arr, labels in (("nav 그룹", g[sel], sch.NAV_GROUPS),
                              ("속도", sb[sel], ["느림", "보통", "빠름"]),
                              ("적재", ld[sel], sch.LOAD_STATES)):
        c = collections.Counter(arr.tolist())
        print(f"  {name}: " + " / ".join(
            f"{labels[k]} {v} ({v/sel.sum():.0%})" for k, v in sorted(c.items())))
    ft = collections.Counter(fault_name[sel].tolist())
    print(f"  이상 종류: " + " / ".join(f"{k} {v}" for k, v in ft.most_common()))
    cc = collections.Counter(cell[sel].tolist())
    print(f"  조건 칸 {len(cc)}/45, 10창 이상 {sum(1 for v in cc.values() if v>=10)}개")

    # 3회 연속 덩어리. 판정이 K=3 연속이라 이 수가 실제 탐지 기회다.
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
    print(f"  이상 연속 덩어리 {len(runs)}개, 최장 {runs.max()}, "
          f"3연속 이상 {int((runs>=3).sum())}개  <- 실제 판정 기회")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
