#!/usr/bin/env python3
"""
Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py — 로봇 telemetry 컨테이너가 어떻게 차고 있는지 본다.

[왜 필요한가]
사업장별 수집이 "윈도우 33/50" 에서 안 늘어날 때, 원인이 셋 중 어느 것인지
로그만으로는 가릴 수 없다.

  (1) 로봇이 발행을 멈췄다            -> cni 와 마지막 ct 가 안 변한다
  (2) 컨테이너 mni 가 작아 지워진다   -> cni 가 mni 에 붙어 멈춘다
  (3) 커서(cra) 조회가 새 걸 안 준다  -> 새 CIN 은 생기는데 조회가 0건

15초 간격으로 두 번 보고 그 셋을 가른다.

사용:
    cd ~/projects/edgeway-FL
    set -a; source .env.local; set +a
    python3 Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py              # 전체
    python3 Edge-ESTIMED-VM/Local-Model/site_app/check_telemetry.py mn2          # 한 사업장만
"""

import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import config                      # noqa: E402
import onem2m_utils as om2m        # noqa: E402

GAP_SEC = float(os.getenv("CHECK_GAP_SEC", "15"))


def snapshot(node):
    """이 사업장 로봇들의 telemetry 컨테이너 현재 상태."""
    om2m.set_endpoint(*config.mn_endpoint(node))
    cse = config.mn_endpoint(node)[1]
    out = {}

    for rb in config.robot_names(node):
        path = f"{cse}/{rb}/{config.ROBOT_TELEMETRY_CNT}"
        res = om2m.get_resource(path)
        cnt = (res or {}).get("m2m:cnt")

        if not cnt:
            out[rb] = None
            continue

        last = om2m.get_latest_content_instance(path)
        last_ct = ((last or {}).get("m2m:cin") or {}).get("ct")

        out[rb] = {
            "path": path,
            "mni": cnt.get("mni"),      # 최대 보관 수 (없으면 무제한)
            "cni": cnt.get("cni"),      # 지금 들어 있는 수
            "last_ct": last_ct,         # 마지막 CIN 의 생성 시각
        }

    return out


def count_after(path, since_ct):
    """커서 이후 CIN 이 몇 개로 조회되는지. MN-AE 가 쓰는 바로 그 경로."""
    try:
        uril = om2m.discover_cins_after(path, since_ct, lim=300)
        return len(uril or [])
    except Exception as e:
        return f"조회 실패: {e}"


def main():
    nodes = sys.argv[1:] or list(config.ALL_NODES)

    print("=" * 66)
    print("  로봇 telemetry 컨테이너 점검")
    print(f"  {GAP_SEC:.0f}초 간격으로 두 번 본다")
    print("=" * 66)

    first = {n: snapshot(n) for n in nodes}

    for node in nodes:
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n[{node} / {site}]  로봇 {len(first[node])}대")
        for rb, s in first[node].items():
            if s is None:
                print(f"  ✗ {rb}: telemetry 컨테이너를 못 읽음")
                continue
            mni = s["mni"] if s["mni"] is not None else "무제한"
            print(f"  {rb}: mni={mni}  cni={s['cni']}  last_ct={s['last_ct']}")

    print(f"\n{GAP_SEC:.0f}초 기다린다 ...")
    time.sleep(GAP_SEC)

    second = {n: snapshot(n) for n in nodes}

    print("\n" + "=" * 66)
    print("  결과")
    print("=" * 66)

    for node in nodes:
        site = config.SITE_OF_NODE.get(node, node)
        print(f"\n[{node} / {site}]")
        om2m.set_endpoint(*config.mn_endpoint(node))

        for rb in first[node]:
            a, b = first[node][rb], second[node].get(rb)
            if a is None or b is None:
                print(f"  ✗ {rb}: 컨테이너를 못 읽음")
                continue

            # 첫 스냅샷의 마지막 ct 를 커서로 두고 그 뒤로 몇 개가
            # 조회되는지 본다. MN-AE 가 라운드마다 하는 것과 같다.
            after = count_after(a["path"], a["last_ct"])
            grew = (a["last_ct"] != b["last_ct"])

            mni = a["mni"] if a["mni"] is not None else "무제한"
            print(f"  {rb}")
            print(f"    mni={mni}  cni: {a['cni']} -> {b['cni']}")
            print(f"    마지막 CIN 시각 바뀜: {'예' if grew else '아니오'}")
            print(f"    커서 이후 조회됨: {after}건")

            if not grew:
                print("    => 로봇이 발행을 멈췄다. Isaac Sim 을 보라.")
            elif isinstance(after, int) and after == 0:
                print("    => 새 CIN 은 생기는데 cra 조회가 0건이다. "
                      "CSE 의 createdAfter 필터 문제다.")
            elif (a["mni"] is not None
                  and str(a["cni"]) == str(a["mni"])):
                print("    => cni 가 mni 에 붙어 있다. "
                      "들어오는 만큼 지워진다. mni 를 올려야 한다.")
            else:
                print("    => 발행도 되고 조회도 된다. 여기는 정상이다.")


if __name__ == "__main__":
    main()
