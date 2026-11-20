# Physics materials for all robots and floors: apply and check.
# Run in Isaac Sim Script Editor; result in ~/script_log.
#   APPLY = True  : Stop state. Sets material values, binds tire / rollers / floors. Then Ctrl+S.
#   APPLY = False : check only. During Play it also reads the values the physics engine uses.
# The engine reads bindings only when Play starts: Stop -> APPLY=True -> save -> Play -> APPLY=False.
import os
import re
import numpy as np
import omni.usd, omni.timeline
from pxr import Usd, UsdPhysics, UsdShade, PhysxSchema

LOG = os.path.expanduser("~/script_log")
lines = []
def log(s): lines.append(str(s))

APPLY = True

MATS = {   # path: (static, dynamic, combine)
    "/World/PhysicsMaterials/tire_normal":  (1.0, 1.0, "min"),
    "/World/PhysicsMaterials/roller_rubber": (0.9, 0.9, "min"),
    "/World/PhysicsMaterials/FloorA":       (0.8, 0.8, "min"),
    "/World/PhysicsMaterials/FloorB":       (0.35, 0.35, "min"),
    "/World/PhysicsMaterials/FloorC":       (0.8, 0.8, "min"),
}
TIRE_MAT = "/World/PhysicsMaterials/tire_normal"
ROLLER_MAT = "/World/PhysicsMaterials/roller_rubber"
FLOORS = {  # site root: floor material. Floor colliders = colliders under root with "floor" in path
    "/World/site_a": "/World/PhysicsMaterials/FloorA",
    "/World/site_b": "/World/PhysicsMaterials/FloorB",
    "/World/site_c": "/World/PhysicsMaterials/FloorC",
}
WORLD = "/World"
ROBOT_PATTERN = r"^site_.+_AMR_\d+$"


def colliders(stage, root):
    return [p for p in Usd.PrimRange(stage.GetPrimAtPath(root)) if p.HasAPI(UsdPhysics.CollisionAPI)]


def bound(p):
    m, _ = UsdShade.MaterialBindingAPI(p).ComputeBoundMaterial("physics")
    return str(m.GetPath()) if m else "NONE"


def bind(stage, p, mat_path):
    UsdShade.MaterialBindingAPI.Apply(p).Bind(
        UsdShade.Material(stage.GetPrimAtPath(mat_path)), UsdShade.Tokens.weakerThanDescendants, "physics")


def robot_parts(stage, robot):
    tire = stage.GetPrimAtPath(f"{robot}/back_wheel/Sphere")
    rollers = [p for c in stage.GetPrimAtPath(robot).GetChildren() if c.GetName().startswith("roller_")
               for p in colliders(stage, str(c.GetPath()))]
    return tire, rollers


def floor_colliders(stage, root):
    return [p for p in colliders(stage, root) if "floor" in str(p.GetPath()).lower()]


def engine_check(stage, robot, n_rollers):
    from isaacsim.core.simulation_manager import SimulationManager
    root = next(str(p.GetPath()) for p in Usd.PrimRange(stage.GetPrimAtPath(robot))
                if p.HasAPI(UsdPhysics.ArticulationRootAPI))
    av = SimulationManager.get_physics_simulation_view().create_articulation_view(root)
    pr = av.get_material_properties()
    pr = pr.numpy() if hasattr(pr, "numpy") else np.asarray(pr)
    groups = {}
    for j in range(pr.shape[1]):
        k = (round(float(pr[0, j, 0]), 3), round(float(pr[0, j, 1]), 3))
        groups[k] = groups.get(k, 0) + 1
    t = MATS[TIRE_MAT][:2]; r = MATS[ROLLER_MAT][:2]
    ok_t = groups.get((round(t[0], 3), round(t[1], 3)), 0) == 1
    ok_r = groups.get((round(r[0], 3), round(r[1], 3)), 0) == n_rollers
    return ok_t, ok_r, groups


try:
    stage = omni.usd.get_context().get_stage()
    playing = omni.timeline.get_timeline_interface().is_playing()
    log(f"APPLY={APPLY} playing={playing}")
    if APPLY and playing:
        raise RuntimeError("APPLY=True needs STOP state (engine reads bindings only at Play start)")

    for path, (sf, df, cm) in MATS.items():
        prim = stage.GetPrimAtPath(path)
        if APPLY:
            prim = UsdShade.Material.Define(stage, path).GetPrim()
            m = UsdPhysics.MaterialAPI.Apply(prim)
            m.CreateStaticFrictionAttr().Set(sf)
            m.CreateDynamicFrictionAttr().Set(df)
            PhysxSchema.PhysxMaterialAPI.Apply(prim).CreateFrictionCombineModeAttr().Set(cm)
        if not prim.IsValid():
            log(f"[mat] MISSING {path}")
            continue
        m = UsdPhysics.MaterialAPI(prim)
        log(f"[mat] {path.split('/')[-1]}: {m.GetStaticFrictionAttr().Get()}/{m.GetDynamicFrictionAttr().Get()} "
            f"{prim.GetAttribute('physxMaterial:frictionCombineMode').Get()}")

    robots = [str(p.GetPath()) for p in stage.GetPrimAtPath(WORLD).GetChildren() if re.match(ROBOT_PATTERN, p.GetName())]
    log(f"robots: {len(robots)}")
    for robot in robots:
        name = robot.split("/")[-1]
        tire, rollers = robot_parts(stage, robot)
        if not tire.IsValid():
            log(f"  {name}: FAIL no tire collider")
            continue
        if APPLY:
            bind(stage, tire, TIRE_MAT)
            for p in rollers:
                bind(stage, p, ROLLER_MAT)
        rb = sorted({bound(p) for p in rollers})
        msg = f"  {name}: usd tire={bound(tire).split('/')[-1]} rollers({len(rollers)})={[x.split('/')[-1] for x in rb]}"
        if playing:
            try:
                ok_t, ok_r, groups = engine_check(stage, robot, len(rollers))
                msg += f" | engine tire {'OK' if ok_t else 'NG'} rollers {'OK' if ok_r else 'NG'} {groups}"
            except Exception as e:
                msg += f" | engine check failed: {e}"
        log(msg)

    # floors: USD check only (not part of an articulation, no engine read)
    for root, mat in FLOORS.items():
        if not stage.GetPrimAtPath(root).IsValid():
            log(f"[floor] {root}: MISSING root")
            continue
        fc = floor_colliders(stage, root)
        if APPLY:
            for p in fc:
                bind(stage, p, mat)
        cnt = {}
        for p in fc:
            b = bound(p).split("/")[-1]
            cnt[b] = cnt.get(b, 0) + 1
        log(f"[floor] {root}: {len(fc)} colliders {cnt}")
    if APPLY:
        log("applied. Save (Ctrl+S), Play, then run again with APPLY=False to check the engine.")
except Exception:
    import traceback
    log("ERROR: " + traceback.format_exc())
finally:
    with open(LOG, "w") as f:
        f.write("\n".join(lines) + "\n")

