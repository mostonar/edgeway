# Site C floor bumps for the capture runs (demo_c1_capture.py): same bumps as create_floor_bump.py,
# but on c1's lane (x = 58.88, y -0.90..5.40, 22 ridges). c2 does not use this lane.
# Run in Isaac Sim Script Editor (Stop state); result in ~/script_log.
# Re-running deletes everything under ROOT (including the create_floor_bump.py bumps) and rebuilds it.
# After capturing, run create_floor_bump.py again to restore the anomaly-scenario bumps.
#   REMOVE = True : only delete the bumps
#   ZIGZAG / RANDOM / SEED : same meaning as in create_floor_bump.py
import os
import random
import omni.usd, omni.timeline
from pxr import Gf, UsdGeom, UsdPhysics, UsdShade

LOG = os.path.expanduser("~/script_log")
ROOT = "/World/floor_bumps"
X = 58.88
Y_START, Y_END = -0.90, 5.40   # bump section (m), below c1 LOAD2 turn, above BOTTOM turn
SPACING = 0.30                 # m; wide enough for rollers (r 8 cm) to reach the floor between ridges
YS = [round(Y_START + i * SPACING, 3) for i in range(int(round((Y_END - Y_START) / SPACING)) + 1)]
AXIS = "X"                 # capsule axis (robot drives along y)
FLOOR_Z = 0.0              # floor top (m)
HEIGHT = 0.030             # m, when not RANDOM
RADIUS = 0.030             # m, when not RANDOM
LENGTH = 1.6               # m, full-width ridge when not ZIGZAG
ZIGZAG = True
HALF_LEN = 0.85            # m, half ridge length (ZIGZAG, not RANDOM)
RANDOM = True              # only with ZIGZAG
SEED = 7                   # None = different each run
H_RANGE = (0.010, 0.020)   # height (m)
R_RANGE = (0.030, 0.050)   # capsule radius (m); 2R must exceed height
OUTER_RANGE = (0.65, 1.00) # m, centerline to outer end (always under the rollers)
INNER_RANGE = (-0.15, 0.15)  # m, inner end vs centerline (+ = crosses it)
FLOOR_MAT = "/World/PhysicsMaterials/FloorB"   # should match the surrounding floor friction
REMOVE = True
lines = []
def log(s): lines.append(str(s))

try:
    stage = omni.usd.get_context().get_stage()
    if omni.timeline.get_timeline_interface().is_playing():
        raise RuntimeError("Playing. Stop first, then run")
    root = stage.GetPrimAtPath(ROOT)
    if root.IsValid():
        for c in root.GetChildren():
            log(f"removed {c.GetPath()}")
        stage.RemovePrim(ROOT)
    if REMOVE:
        raise SystemExit
    mat = stage.GetPrimAtPath(FLOOR_MAT)
    if not mat.IsValid() or not mat.HasAPI(UsdPhysics.MaterialAPI):
        for p in stage.Traverse():                       # list materials to pick from
            if p.HasAPI(UsdPhysics.MaterialAPI):
                m = UsdPhysics.MaterialAPI(p)
                log(f"  material {p.GetPath()}  static={m.GetStaticFrictionAttr().Get()} dynamic={m.GetDynamicFrictionAttr().Get()}")
        raise RuntimeError(f"{FLOOR_MAT} not found. Pick the site C floor material from the list above and set FLOOR_MAT")

    UsdGeom.Xform.Define(stage, ROOT)
    rng = random.Random(SEED)
    for i, y in enumerate(YS, 1):
        rnd_on = ZIGZAG and RANDOM
        h = rng.uniform(*H_RANGE) if rnd_on else HEIGHT
        rad = rng.uniform(*R_RANGE) if rnd_on else RADIUS
        z = FLOOR_Z + h - rad
        path = f"{ROOT}/site_c_bump_{i}"
        cap = UsdGeom.Capsule.Define(stage, path)
        cap.CreateAxisAttr(AXIS)
        cap.CreateRadiusAttr(rad)
        if ZIGZAG:                                       # odd ridges on -x half, even on +x half
            side = -1 if i % 2 else 1
            if RANDOM:
                outer, inner = rng.uniform(*OUTER_RANGE), rng.uniform(*INNER_RANGE)
            else:
                outer, inner = HALF_LEN, 0.0
            a, b = X - side * inner, X + side * outer
            length, cx = abs(b - a), (a + b) / 2
        else:
            length, cx = LENGTH, X
        cap.CreateHeightAttr(max(length - 2 * rad, 0.01))   # USD height = cylinder part only
        cap.CreateDisplayColorAttr([Gf.Vec3f(0.15, 0.15, 0.15)])
        UsdGeom.Xformable(cap).AddTranslateOp().Set(Gf.Vec3d(cx, y, z))
        prim = cap.GetPrim()
        UsdPhysics.CollisionAPI.Apply(prim)
        UsdShade.MaterialBindingAPI.Apply(prim).Bind(
            UsdShade.Material(mat), UsdShade.Tokens.weakerThanDescendants, "physics")
        log(f"bump {i} y={y}  x {cx - length / 2:.2f}~{cx + length / 2:.2f} (length {length:.2f} m)  height {h * 1000:.0f} mm  width {2 * rad * 100:.0f} cm")
    m = UsdPhysics.MaterialAPI(mat)
    log(f"material {FLOOR_MAT}  static={m.GetStaticFrictionAttr().Get()} dynamic={m.GetDynamicFrictionAttr().Get()}")
except SystemExit:
    pass
except Exception:
    import traceback
    log("ERROR: " + traceback.format_exc())
finally:
    with open(LOG, "w") as f:
        f.write("\n".join(lines) + "\n")
