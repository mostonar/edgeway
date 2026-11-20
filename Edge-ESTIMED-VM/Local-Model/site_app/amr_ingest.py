"""
amr_ingest.py — MN-CSE의 contentInstance에서 주행 센서 데이터를 직접 가져온다

[왜 필요한가]
기존 구현은 cnt-sensor-data의 con에 로컬 pkl 경로(data_path)만 넣고, MN-AE가
그 경로의 파일을 직접 읽었다. 모델 가중치와 똑같은 문제다.

  - 로봇/시뮬레이터와 학습 앱이 다른 머신에 있으면 즉시 깨진다
  - oneM2M이 나르는 것은 '파일이 거기 있다'는 쪽지뿐이다

여기서는 CIN의 con에 들어 있는 실제 신호를 읽어 학습 입력으로 만든다.
con 형식은 Isaac Sim이 MN-CSE로 올리는 것과 같고, 내보낸 export/merged.json의
한 CIN과도 같다. 즉 오프라인 파일과 라이브 CSE가 같은 파서를 쓴다.

[con 형식]
    {"t_wall":…, "t_sim":[...], "nav":"…",
     "pose":{"x":[...],"y":[...],"yaw":[...]},
     "imu":{"front":{"ax":[...],…},"left":…,"right":…},
     "enc":{"drive":{"pos":[...],"vel":[...],"eff":[...]}, …},
     "robot":"siteA_AMR_1", "seq":12}

채널 구성과 전처리는 prepare_data_amr과 공유한다(한 곳만 고치면 되게).

[라운드 구분]
Isaac Sim이 CIN에 round_N 라벨을 붙여 올린다는 전제다.
라벨이 없으면 con 안의 round 필드를 보고, 그것도 없으면 최신 N개를 쓴다.
"""

from __future__ import annotations

import json
from typing import Dict, List, Optional, Tuple

import numpy as np

import amr_schema as sch
import config
import onem2m_utils as om2m


def parse_cin_window(payload: dict) -> Optional[Tuple[np.ndarray, dict]]:
    """
    CIN의 con(dict) 하나를 윈도우 한 개로. 채널이 모자라면 None.
    반환: (신호 (C, T), 메타)
    """
    sig = sch.window_from_con(payload)
    if sig is None:
        return None

    meta = {
        "robot": payload.get("robot", "unknown"),
        "seq": payload.get("seq"),
        "nav": payload.get("nav"),
        "t_wall": payload.get("t_wall"),
        "speed": sch.speed_of(sig),
        "pose": sch.pose_of(payload),
    }
    return sig, meta


def _labels_of(cin: dict) -> Dict[str, str]:
    lbl = (cin.get("m2m:cin") or {}).get("lbl") or []
    return {x.split(":")[0]: x.split(":", 1)[1] for x in lbl if ":" in x}


def _con_of(cin: dict) -> Optional[dict]:
    con = (cin.get("m2m:cin") or {}).get("con")
    try:
        obj = json.loads(con) if isinstance(con, str) else con
    except (TypeError, ValueError):
        return None
    return obj if isinstance(obj, dict) else None


def _empty() -> Dict[str, np.ndarray]:
    return {
        "signals": np.zeros((0, sch.N_CH, sch.T), dtype=np.float32),
        "cond": np.zeros((0, sch.COND_DIM), dtype=np.float32),
        "robot": np.asarray([]),
        "pose": np.zeros((0, 2), np.float32),
        "cond_name": np.asarray([]),
        "nav": np.asarray([]),
        "ct": np.asarray([]),
        # 로봇이 측정한 시각(epoch 초). 사업장 전체 정렬 키다.
        "t_wall": np.zeros(0, np.float64),
        # 로봇별 순번. 빠진 윈도우를 찾는 데 쓴다.
        "seq": np.zeros(0, np.int64),
        "max_ct": "",
        "n_raw": 0,
    }


def fetch_latest_windows(
    container_path: str,
    limit: int = 200,
) -> Dict[str, np.ndarray]:
    """
    라운드 라벨과 무관하게 최근 CIN limit개를 가져온다.

    실시간 학습에서는 이쪽이 자연스럽다. "로봇들이 계속 주행 중이고,
    이번 라운드에는 방금까지 들어온 구간으로 학습한다"에 해당한다.
    """
    uril = om2m.list_uril(container_path, lim=limit)
    if not uril:
        return _empty()

    cins = []
    for u in uril[-limit:]:
        res = om2m.get_resource(u.lstrip("/"))
        if res and "m2m:cin" in res:
            cins.append(res)
    return build_from_cins(cins)


def balance_across_robots(got: Dict[str, np.ndarray],
                          cap: int) -> Dict[str, np.ndarray]:
    """
    사업장 전체 윈도우를 cap개 이하로 줄이되, 로봇별로 고르게 남긴다.

    로봇 수가 사업장마다 다르면(A 4대, B 2대) 로봇당 같은 상한을 줘도
    사업장 총량이 2배 차이 난다. FedAvg가 샘플 수로 가중하므로 그대로
    두면 로봇이 많은 사업장이 더 큰 영향력을 갖는다. 사업장별 기여를
    맞추려면 사업장 단위로 잘라야 한다.

    로봇마다 cap/로봇수 만큼 배정하고, 적게 가진 로봇의 몫은 남은 로봇이
    나눠 갖는다. 한 로봇이 몰아서 채우는 일을 막는다.
    앞에서 자르지 않고 시간 축 전체에서 고르게 뽑는다.
    """
    n = len(got["signals"])
    if cap <= 0 or n <= cap:
        return got

    robots = got["robot"]
    uniq = sorted(set(robots.tolist()))
    idx_by_robot = {r: np.where(robots == r)[0] for r in uniq}

    quota = {r: 0 for r in uniq}
    remaining = cap
    pool = list(uniq)
    while remaining > 0 and pool:
        share = max(1, remaining // len(pool))
        for r in list(pool):
            avail = len(idx_by_robot[r]) - quota[r]
            take = min(share, avail, remaining)
            quota[r] += take
            remaining -= take
            if quota[r] >= len(idx_by_robot[r]):
                pool.remove(r)
            if remaining <= 0:
                break

    keep = []
    for r in uniq:
        src = idx_by_robot[r]
        k = quota[r]
        if k <= 0:
            continue
        # 앞쪽만 자르면 주행 초반만 보게 되므로 균등 간격으로 뽑는다.
        pick = np.linspace(0, len(src) - 1, k).astype(np.int64)
        keep.append(src[pick])

    keep = np.sort(np.concatenate(keep)) if keep else np.array([], dtype=np.int64)

    out = dict(got)
    for k in ("signals", "cond", "robot", "pose", "cond_name", "nav",
              "ct", "t_wall", "seq"):
        if k in out and len(out[k]) == n:
            out[k] = out[k][keep]
    return out


def fetch_new_windows(container_path: str,
                      since_ct: Optional[str] = None,
                      limit: int = 300) -> Dict[str, np.ndarray]:
    """
    since_ct 이후에 들어온 윈도우만 가져온다.

    라운드마다 "최근 N개"를 긁으면 라운드 간격이 짧을 때 거의 같은 데이터를
    다시 학습하게 된다. 그러면 라운드가 지나며 좋아진 것이 데이터가 늘어서인지
    에폭이 쌓여서인지 구분할 수 없다. 그래서 읽은 지점을 기억하고 그 뒤만 읽는다.

    서버의 cra 필터를 먼저 쓰고, 그게 듣지 않으면 받아서 ct로 거른다.
    """
    uril = om2m.discover_cins_after(container_path, since_ct, lim=limit)
    if not uril:
        return _empty()

    # 오래된 쪽부터 limit 개를 집는다.
    #
    # 전에는 uril[-limit:] 로 '최신 limit 개'를 집었다. uril 은 오래된
    # 것부터 오므로 그러면 앞쪽(오래된) 구간을 건너뛴다. 게다가 커서는
    # 집은 것들의 max_ct 로 점프하므로, 건너뛴 구간은 영영 못 받는다.
    #
    # 실측: siteA 의 telemetry 에 윈도우가 418개 있었는데 mn1 은 238개만
    # 가져갔다. 180개가 이렇게 사라졌고, 그래서 라운드 5에서 50개를 못
    # 채우고 멈췄다.
    #
    # 앞에서부터 집으면 커서가 조금씩만 전진하고 나머지는 다음 조회에
    # 그대로 남는다. 버려지는 것이 없다.
    #
    # (fetch_latest_windows 는 '최근 N개'가 목적이라 [-limit:] 이 맞다.)
    cins = []
    for u in uril[:limit]:
        res = om2m.get_resource(u.lstrip("/"))
        if res and "m2m:cin" in res:
            cins.append(res)

    got = build_from_cins(cins)

    # cra를 무시하는 CSE 대비. 받은 것 중 since_ct 이후만 남긴다.
    if since_ct and len(got["signals"]):
        keep = got["ct"] > since_ct
        if not keep.all():
            for k in ("signals", "cond", "robot", "pose", "cond_name",
                      "nav", "ct", "t_wall", "seq"):
                got[k] = got[k][keep]
            got["max_ct"] = max([c for c in got["ct"].tolist() if c],
                                default=since_ct)
    return got


ARRAY_KEYS = ("signals", "cond", "robot", "pose",
              "cond_name", "nav", "ct", "t_wall", "seq")


def _reindex(got: Dict[str, np.ndarray], idx) -> Dict[str, np.ndarray]:
    """같은 길이의 배열들을 한꺼번에 같은 순서/선택으로 다시 만든다."""
    n = len(got["signals"])
    out = dict(got)
    for k in ARRAY_KEYS:
        if k in out and len(out[k]) == n:
            out[k] = out[k][idx]
    return out


def sort_by_time(got: Dict[str, np.ndarray]) -> Dict[str, np.ndarray]:
    """
    사업장 전체를 시간순으로 줄 세운다.

    [왜 필요한가]
    fetch_site_windows_since 는 로봇별로 받아 이어 붙이므로, 버퍼는
    [AMR_1 전부][AMR_2 전부] 처럼 로봇 단위로 묶여 있다. 그대로 앞에서
    자르면 로봇 하나만 몰아서 가져가게 된다.

    [왜 t_wall 인가]
    ct 는 CSE 에 '도착한' 시각이다. 한 로봇의 발행이 밀렸다 몰아서 올라오면
    그 로봇의 오래된 데이터가 다른 로봇의 최신 데이터보다 뒤에 온다.
    t_wall 은 로봇이 '측정한' 시각이라 밀려도 순서가 안 바뀐다.
    t_wall 이 없는 배포를 위해 ct 로 되돌아간다.
    """
    n = len(got["signals"])
    if n <= 1:
        return got

    tw = got.get("t_wall")
    if tw is not None and len(tw) == n and np.isfinite(tw).all():
        order = np.argsort(tw, kind="stable")
    else:
        order = np.argsort(got["ct"], kind="stable")

    return _reindex(got, order)


def take_head(got: Dict[str, np.ndarray], n: int):
    """
    앞에서 n개를 떼고 나머지를 돌려준다.

    버리지 않는다. 남은 것은 다음 라운드가 이어서 쓴다. 그래서 라운드마다
    정확히 n개가 되고, 한 시점에 생긴 이상이 여러 라운드로 흩어지지 않는다.

    반환: (앞 n개, 나머지)
    """
    total = len(got["signals"])
    if n <= 0:
        return _empty(), got
    if total <= n:
        return got, _empty()

    idx = np.arange(total)
    head = _reindex(got, idx[:n])
    tail = _reindex(got, idx[n:])

    head["max_ct"] = max([c for c in head["ct"].tolist() if c], default="")
    tail["max_ct"] = max([c for c in tail["ct"].tolist() if c], default="")
    head["n_raw"] = n
    tail["n_raw"] = total - n
    return head, tail


def robot_counts(got: Dict[str, np.ndarray]) -> Dict[str, int]:
    """로봇별 윈도우 수. 한 로봇이 독식하는지 로그로 보기 위한 것."""
    robots = got.get("robot")
    if robots is None or not len(robots):
        return {}
    uniq, cnt = np.unique(robots, return_counts=True)
    return {str(r): int(c) for r, c in zip(uniq, cnt)}


def seq_gaps(got: Dict[str, np.ndarray]) -> Dict[str, int]:
    """
    로봇별로 빠진 윈도우 수를 센다.

    seq 는 로봇이 매기는 순번이라 로봇 안에서만 의미가 있다. 연속이어야
    하는데 뛰면 그 사이가 유실된 것이다. 지금까지는 윈도우가 조용히
    사라져도 알 방법이 없었다.
    """
    robots, seqs = got.get("robot"), got.get("seq")
    if robots is None or seqs is None or not len(seqs):
        return {}

    out = {}
    for r in sorted(set(robots.tolist())):
        v = np.sort(seqs[robots == r])
        v = v[v >= 0]
        if len(v) < 2:
            continue
        missing = int((np.diff(v) - 1).clip(min=0).sum())
        if missing:
            out[str(r)] = missing
    return out


def merge_windows(acc: Optional[Dict[str, np.ndarray]],
                  new: Dict[str, np.ndarray]) -> Dict[str, np.ndarray]:
    """
    라운드 중 여러 번에 걸쳐 받은 윈도우를 이어 붙인다.

    [왜 필요한가]
    로봇의 telemetry 컨테이너에는 mni(최대 보관 수)가 걸려 있다. Isaac Sim
    쪽이 만든 컨테이너라 우리가 정하지 않는다. 새 CIN 이 들어오면 가장
    오래된 것이 지워진다.

    그래서 "50개 모일 때까지 기다렸다가 한 번에 긁는다"가 성립하지 않는다.
    같은 커서로 다시 조회하면 그 사이 지워진 만큼 빠져서, 총량이
    로봇 수 × mni 를 절대 못 넘는다. 로봇 4대 사업장은 57개로 50을
    넘겼는데 2대 사업장이 31/33 에서 얼어붙었던 것이 이것이다.

    받을 때마다 여기에 쌓고 커서를 전진시키면, 지워지기 전에 챙긴 것이
    버퍼에 남는다.
    """
    if acc is None or not len(acc.get("signals", [])):
        return new
    if new is None or not len(new.get("signals", [])):
        return acc

    out = {
        k: np.concatenate([acc[k], new[k]])
        for k in ("signals", "cond", "robot", "pose",
                  "cond_name", "nav", "ct", "t_wall", "seq")
    }
    out["max_ct"] = max(acc.get("max_ct", ""), new.get("max_ct", ""))
    out["n_raw"] = int(acc.get("n_raw", 0)) + int(new.get("n_raw", 0))
    return out


def fetch_site_windows_since(container_paths: List[str],
                             cursors: Dict[str, str],
                             limit_per_robot: int = 300) -> tuple:
    """
    사업장 로봇 전체에서 각자의 커서 이후 윈도우만 모아 합친다.

    반환: (합쳐진 데이터, 갱신된 커서)
    커서는 컨테이너 경로별 마지막 ct다. 로봇마다 발행 속도가 다르므로
    따로 관리한다.
    """
    parts, new_cursors = [], dict(cursors)

    for path in container_paths:
        got = fetch_new_windows(path, cursors.get(path), limit_per_robot)
        if len(got["signals"]):
            parts.append(got)
            if got["max_ct"]:
                new_cursors[path] = got["max_ct"]

    if not parts:
        return _empty(), new_cursors

    merged = {
        "signals": np.concatenate([p["signals"] for p in parts]),
        "cond": np.concatenate([p["cond"] for p in parts]),
        "robot": np.concatenate([p["robot"] for p in parts]),
        "pose": np.concatenate([p["pose"] for p in parts]),
        "cond_name": np.concatenate([p["cond_name"] for p in parts]),
        "nav": np.concatenate([p["nav"] for p in parts]),
        "ct": np.concatenate([p["ct"] for p in parts]),
        "t_wall": np.concatenate([p["t_wall"] for p in parts]),
        "seq": np.concatenate([p["seq"] for p in parts]),
        "max_ct": max([p["max_ct"] for p in parts], default=""),
        "n_raw": sum(p["n_raw"] for p in parts),
    }
    return merged, new_cursors


def fetch_site_windows(
    container_paths: List[str],
    limit_per_robot: int = 150,
) -> Dict[str, np.ndarray]:
    """
    한 사업장의 로봇 여러 대에서 최근 윈도우를 모아 하나로 합친다.

    새 데이터 배치에서는 텔레메트리가 MN-AE가 아니라 로봇 AE 아래에 쌓인다.
        <MN-CSE>/siteA_AMR_1/telemetry
        <MN-CSE>/siteA_AMR_2/telemetry   …
    사업장 하나가 FL 노드 하나이므로, 로봇 대수가 몇이든 여기서 합쳐서
    "한 공장의 데이터"로 만든다. 설계 그대로다.

    로봇 한 대가 죽어 있어도 나머지로 라운드를 돈다.
    """
    parts = []
    for path in container_paths:
        got = fetch_latest_windows(path, limit=limit_per_robot)
        if len(got["signals"]):
            parts.append(got)

    if not parts:
        return _empty()

    return {
        "signals": np.concatenate([p["signals"] for p in parts]),
        "cond": np.concatenate([p["cond"] for p in parts]),
        "robot": np.concatenate([p["robot"] for p in parts]),
        "pose": np.concatenate([p["pose"] for p in parts]),
        "cond_name": np.concatenate([p["cond_name"] for p in parts]),
        "nav": np.concatenate([p["nav"] for p in parts]),
        "ct": np.concatenate([p["ct"] for p in parts]),
        "max_ct": max([p["max_ct"] for p in parts], default=""),
        "n_raw": sum(p["n_raw"] for p in parts),
    }


def fetch_round_windows(
    container_path: str,
    round_num: int,
    limit: int = 400,
) -> Dict[str, np.ndarray]:
    """
    한 라운드분 CIN을 모아 학습 입력으로 만든다.

    container_path: 보통 "{MN-CSE}/{로봇 AE}/telemetry"
    반환 dict:
        signals (N, C, T) / cond (N, D) / robot / pose / cond_name / n_raw

    찾는 순서
      1) round_N 라벨이 붙은 CIN
      2) con 안의 round 필드가 일치하는 CIN
      3) 둘 다 없으면 최근 limit개 (실시간 스트림으로 간주)
    """
    cins = om2m.retrieve_cins_by_label(
        container_path,
        f"round_{round_num}",
        lim=limit,
        fallback_scan=max(limit, 200),
        newest_first=False,          # 시간 순서를 유지한다
    )

    if not cins:
        uril = om2m.list_uril(container_path, lim=max(limit, 200))
        for u in uril:
            res = om2m.get_resource(u.lstrip("/"))
            if not res:
                continue
            con = _con_of(res)
            if con is None:
                continue
            r = con.get("round", con.get("currentRound"))
            if r is not None and int(r) == int(round_num):
                cins.append(res)

    if not cins:
        # 라운드 구분이 아예 없는 배포. 최근 구간으로 학습한다.
        print(f"    ⓘ round_{round_num} 표시가 없어 최근 {limit}개로 대체")
        return fetch_latest_windows(container_path, limit=limit)

    return build_from_cins(cins)


def build_from_cins(cins: List[Dict]) -> Dict[str, np.ndarray]:
    """CIN 목록 -> 학습 입력. 라벨에서 조건과 상태를 읽는다."""
    sigs, conds, robots, poses, names, navs = [], [], [], [], [], []
    cts = []      # CIN 생성시각. 다음 라운드의 커서로 쓴다.
    twalls = []   # 로봇이 측정한 시각. 정렬은 이걸로 한다.
    seqs = []     # 로봇별 순번. 유실 확인용.

    for cin in cins:
        con = _con_of(cin)
        if con is None:
            continue
        parsed = parse_cin_window(con)
        if parsed is None:
            continue
        sig, meta = parsed

        # 조건은 payload에서 끌어낸다. 운영 중에는 라벨이 붙지 않는다.
        # 옛 데이터처럼 scn/spd 라벨이 있으면 그걸 우선 쓴다.
        lab = _labels_of(cin)
        if "scn" in lab and "spd" in lab:
            cvec = sch.cond_from_labels(lab["scn"], lab["spd"])
        else:
            cvec = sch.cond_vector(meta["nav"], meta["speed"], sig)

        sigs.append(sig)
        conds.append(cvec)
        robots.append(meta["robot"])
        poses.append(meta["pose"])
        names.append(lab.get("cond", "floor_bump"
                             if meta["nav"] in ("over_bump",) else "normal"))
        navs.append(meta["nav"] or "idle")
        cts.append(str(cin.get("m2m:cin", {}).get("ct", "")))

        # 없으면 NaN / -1. 정렬하는 쪽에서 ct 로 되돌아간다.
        try:
            twalls.append(float(meta.get("t_wall")))
        except (TypeError, ValueError):
            twalls.append(float("nan"))
        try:
            seqs.append(int(meta.get("seq")))
        except (TypeError, ValueError):
            seqs.append(-1)

    if not sigs:
        out = _empty()
        out["n_raw"] = len(cins)
        return out

    return {
        "signals": np.stack(sigs).astype(np.float32),
        "cond": np.stack(conds).astype(np.float32),
        "robot": np.asarray(robots),
        "pose": np.asarray(poses, dtype=np.float32),
        "cond_name": np.asarray(names),
        "nav": np.asarray(navs),
        "ct": np.asarray(cts),
        "t_wall": np.asarray(twalls, dtype=np.float64),
        "seq": np.asarray(seqs, dtype=np.int64),
        "max_ct": max([c for c in cts if c], default=""),
        "n_raw": len(cins),
    }


def normalize_with(
    signals: np.ndarray,
    mean: Optional[np.ndarray],
    std: Optional[np.ndarray],
) -> np.ndarray:
    """
    노드가 보관한 정규화 통계로 표준화한다.

    통계는 '그 사업장의 학습 정상'에서 뽑은 것이어야 한다. 사업장마다 로봇
    개체차가 있어 공용 통계를 쓰면 정상도 이상으로 보인다.
    통계가 없으면(첫 수집) 들어온 데이터로 즉석 계산한다.
    """
    if signals.size == 0:
        return signals
    if mean is None or std is None:
        mean = signals.mean(axis=(0, 2), keepdims=True)
        std = signals.std(axis=(0, 2), keepdims=True) + 1e-8
    return ((signals - mean) / std).astype(np.float32)
