# Robot anomaly setup for one robot: tire material and front-left roller knob.
# Run in Isaac Sim Script Editor; result in ~/script_log.
#   First run in Stop state: creates tire_active_<robot> (friction combine = min) and binds it to the
#   drive tire. sim_watcher needs this binding for tire=normal/oil.
#   TIRE_OIL      : copy tire_oil (True) or tire_normal (False) values into tire_active (works during Play)
#   ROLLER_DEFECT : collision on/off of the roller knob (created if missing)
# All changes are written to USD and stay after Stop. Set ROBOT for each robot.
# Do not use the tensor API set_material_properties for the tire: it breaks the combine mode.
import os
import omni.usd, omni.timeline
from pxr import Usd, UsdGeom, UsdPhysics, UsdShade, PhysxSchema

LOG = os.path.expanduser("~/script_log")
lines = []
def log(s): lines.append(str(s))

TIRE_OIL = False          # robot anomaly 1: oily drive tire
ROLLER_DEFECT = False     # robot anomaly 2: knob on front-left roller

ROBOT = "/World/site_c_AMR_2"
TIRE_COL = f"{ROBOT}/back_wheel/Sphere"
MAT_NORMAL = "/World/PhysicsMaterials/tire_normal"
MAT_OIL = "/World/PhysicsMaterials/tire_oil"
MAT_ACTIVE = f"/World/PhysicsMaterials/tire_active_{ROBOT.split('/')[-1]}"
ROLLER_COL = f"{ROBOT}/roller_front_left/Cylinder_03"
LOBE = f"{ROBOT}/roller_front_left/lobe"
R_ROLLER, R_KNOB, H = 0.08, 0.02, 0.001   # m; same as sim_watcher

def make_lobe(stage):
    rc = stage.GetPrimAtPath(ROLLER_COL)
    ops = UsdGeom.Xformable(rc).GetOrderedXformOps()
    s = UsdGeom.Cylinder(rc)
    k = UsdGeom.Cylinder.Define(stage, LOBE)
    k.CreateAxisAttr(s.GetAxisAttr().Get())
    k.CreateRadiusAttr(s.GetRadiusAttr().Get())
    k.CreateHeightAttr(s.GetHeightAttr().Get())
    k.CreateExtentAttr(s.GetExtentAttr().Get())
    xf = UsdGeom.Xformable(k)
    t0, sc = ops[0].Get(), ops[2].Get()
    d = R_ROLLER - R_KNOB + H
    xf.AddTranslateOp(ops[0].GetPrecision()).Set(type(t0)(t0[0], t0[1], t0[2] + d))
    xf.AddOrientOp(ops[1].GetPrecision()).Set(ops[1].Get())
    xf.AddScaleOp(ops[2].GetPrecision()).Set(type(sc)(2 * R_KNOB, 2 * R_KNOB, sc[2]))
    k.CreateVisibilityAttr("invisible")
    p = k.GetPrim()
    UsdPhysics.CollisionAPI.Apply(p)
    UsdPhysics.MassAPI.Apply(p).CreateDensityAttr(1e-3)
    c = PhysxSchema.PhysxCollisionAPI.Apply(p)
    c.CreateContactOffsetAttr().Set(0.01)
    c.CreateRestOffsetAttr().Set(0.0)
    rmat, _ = UsdShade.MaterialBindingAPI(rc).ComputeBoundMaterial("physics")
    if rmat:
        UsdShade.MaterialBindingAPI.Apply(p).Bind(rmat, UsdShade.Tokens.weakerThanDescendants, "physics")
    log(f"[knob] created {H*1000:.0f} mm")


def ensure_active(stage, playing):
    tire = stage.GetPrimAtPath(TIRE_COL)
    bound, _ = UsdShade.MaterialBindingAPI(tire).ComputeBoundMaterial("physics")
    if bound and str(bound.GetPath()) == MAT_ACTIVE:
        return
    if playing:
        raise RuntimeError("tire_active not bound yet. Press Stop, run this script once, then Play.")
    mat = UsdShade.Material.Define(stage, MAT_ACTIVE)
    UsdPhysics.MaterialAPI.Apply(mat.GetPrim())
    PhysxSchema.PhysxMaterialAPI.Apply(mat.GetPrim()).CreateFrictionCombineModeAttr().Set("min")
    UsdShade.MaterialBindingAPI.Apply(tire).Bind(mat, UsdShade.Tokens.weakerThanDescendants, "physics")
    log(f"[tire] created {MAT_ACTIVE} (combine=min) and bound to {TIRE_COL}")


def set_tire(stage, src_path):
    src = UsdPhysics.MaterialAPI(stage.GetPrimAtPath(src_path))
    dst = UsdPhysics.MaterialAPI(stage.GetPrimAtPath(MAT_ACTIVE))
    sf = float(src.GetStaticFrictionAttr().Get())
    df = float(src.GetDynamicFrictionAttr().Get())
    rs = src.GetRestitutionAttr().Get()
    dst.CreateStaticFrictionAttr().Set(sf)
    dst.CreateDynamicFrictionAttr().Set(df)
    if rs is not None:
        dst.CreateRestitutionAttr().Set(float(rs))
    cm = PhysxSchema.PhysxMaterialAPI(stage.GetPrimAtPath(MAT_ACTIVE)).GetFrictionCombineModeAttr().Get()
    log(f"[tire] {MAT_ACTIVE} <- {src_path}: static={dst.GetStaticFrictionAttr().Get()} "
        f"dynamic={dst.GetDynamicFrictionAttr().Get()} combine={cm}")


try:
    stage = omni.usd.get_context().get_stage()
    playing = omni.timeline.get_timeline_interface().is_playing()
    log(f"playing={playing} TIRE_OIL={TIRE_OIL} ROLLER_DEFECT={ROLLER_DEFECT}")

    ensure_active(stage, playing)
    set_tire(stage, MAT_OIL if TIRE_OIL else MAT_NORMAL)

    if not stage.GetPrimAtPath(LOBE).IsValid():
        make_lobe(stage)
    col = UsdPhysics.CollisionAPI(stage.GetPrimAtPath(LOBE))
    col.CreateCollisionEnabledAttr().Set(bool(ROLLER_DEFECT))
    log(f"[knob] collision={col.GetCollisionEnabledAttr().Get()}")
except Exception:
    import traceback
    log("ERROR: " + traceback.format_exc())
finally:
    with open(LOG, "w") as f:
        f.write("\n".join(lines) + "\n")
