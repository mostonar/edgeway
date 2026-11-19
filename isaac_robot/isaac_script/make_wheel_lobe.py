# Adds lobes (wlobe) to the rear drive wheel (back_wheel) and toggles them: N_KNOB small spheres on the
# wheel collider rim stick out by H, so the robot rear lifts every 1/N_KNOB wheel turn.
# Run in Isaac Sim Script Editor; result in ~/script_log. Demos toggle the same lobes via sim_watcher wknob=on/off.
#   MODE = "make"        : create (Stop state only), collision off; re-running rebuilds
#   MODE = "on" / "off"  : enable / disable lobe collision (works during Play)
#   MODE = "show" / "hide" : show lobes in red / hide again (position check)
import math, os, re
import omni.usd, omni.timeline
from pxr import Gf, UsdGeom, UsdPhysics, UsdShade, PhysxSchema

LOG = os.path.expanduser("~/script_log")
MODE = "make"                      # make / on / off / show / hide
ROBOTS = ["site_c_AMR_2"]          # [] = all site_*_AMR_n under /World
R_KNOB = 0.02                      # lobe sphere radius (m)
H = 0.005                          # height above wheel surface (m)
N_KNOB = 2                         # lobes, evenly spaced; 2 keeps a hit in most 1 s windows during in-place turns
lines = []
def log(s): lines.append(str(s))


def wheel_axis(stage, robot, link):
    for p in stage.Traverse():
        if not str(p.GetPath()).startswith(robot + "/") or not p.IsA(UsdPhysics.RevoluteJoint):
            continue
        j = UsdPhysics.RevoluteJoint(p)
        b0 = [str(t) for t in j.GetBody0Rel().GetTargets()]
        b1 = [str(t) for t in j.GetBody1Rel().GetTargets()]
        if link not in b0 + b1:
            continue
        ax = {"X": Gf.Vec3d(1, 0, 0), "Y": Gf.Vec3d(0, 1, 0), "Z": Gf.Vec3d(0, 0, 1)}[j.GetAxisAttr().Get()]
        rot = (j.GetLocalRot1Attr() if link in b1 else j.GetLocalRot0Attr()).Get()
        q = Gf.Rotation(Gf.Quatd(rot.GetReal(), Gf.Vec3d(rot.GetImaginary())))
        return q.TransformDir(ax).GetNormalized(), str(p.GetPath())
    return None, None


def lobes(stage, name):
    w = stage.GetPrimAtPath(f"/World/{name}/back_wheel")
    return [c for c in w.GetChildren() if c.GetName().startswith("wlobe")] if w.IsValid() else []


def toggle(stage, name):
    ls = lobes(stage, name)
    if not ls:
        log(f"{name}: no wlobe -> Stop and run with MODE = \"make\" first")
        return
    for lobe in ls:
        if MODE in ("on", "off"):
            UsdPhysics.CollisionAPI(lobe).CreateCollisionEnabledAttr().Set(MODE == "on")
        else:
            g = UsdGeom.Sphere(lobe)
            g.CreateDisplayColorAttr().Set([Gf.Vec3f(1, 0, 0)])
            g.CreateVisibilityAttr().Set("inherited" if MODE == "show" else "invisible")
        log(f"{name}/{lobe.GetName()}: collision {UsdPhysics.CollisionAPI(lobe).GetCollisionEnabledAttr().Get()},"
            f" visibility {UsdGeom.Imageable(lobe).GetVisibilityAttr().Get()}")


def make_one(stage, lobe, col, axis, c, r, dirn, k):
    pos = c + dirn * (r - R_KNOB + H)
    s = UsdGeom.Sphere.Define(stage, lobe)
    s.CreateRadiusAttr(R_KNOB)
    s.CreateExtentAttr([Gf.Vec3f(-R_KNOB), Gf.Vec3f(R_KNOB)])
    UsdGeom.Xformable(s).AddTranslateOp().Set(pos)
    s.CreateVisibilityAttr("invisible")
    p = s.GetPrim()
    UsdPhysics.CollisionAPI.Apply(p).CreateCollisionEnabledAttr().Set(False)
    UsdPhysics.MassAPI.Apply(p).CreateDensityAttr(1e-3)
    pc = PhysxSchema.PhysxCollisionAPI.Apply(p)
    pc.CreateContactOffsetAttr().Set(0.01)
    pc.CreateRestOffsetAttr().Set(0.0)
    mat, _ = UsdShade.MaterialBindingAPI(col).ComputeBoundMaterial("physics")   # same material as the tire
    if mat:
        UsdShade.MaterialBindingAPI.Apply(p).Bind(mat, UsdShade.Tokens.weakerThanDescendants, "physics")
    d = pos - c
    radial = (d - axis * Gf.Dot(d, axis)).GetLength()
    log(f"  {lobe.split('/')[-1]} created (collision off, {360 * k / N_KNOB:.0f} deg): height {(radial + R_KNOB - r) * 1000:.1f} mm, axial offset {abs(Gf.Dot(d, axis)) * 1000:.1f} mm,"
        f" material {mat.GetPath() if mat else 'none'}")


try:
    stage = omni.usd.get_context().get_stage()
    if MODE not in ("make", "on", "off", "show", "hide"):
        raise RuntimeError(f"MODE must be make / on / off / show / hide (got {MODE})")
    playing = omni.timeline.get_timeline_interface().is_playing()
    if MODE == "make" and playing:
        raise RuntimeError("Playing. Stop first, then run make")
    names = ROBOTS or [p.GetName() for p in stage.GetPrimAtPath("/World").GetChildren()
                       if re.match(r"^site_.+_AMR_\d+$", p.GetName())]
    log(f"MODE = {MODE} ({'Play' if playing else 'Stop'})")
    for name in names:
        if MODE != "make":
            toggle(stage, name)
            continue
        robot = f"/World/{name}"
        link = f"{robot}/back_wheel"
        col = stage.GetPrimAtPath(f"{link}/Sphere")
        if not col.IsValid():
            log(f"{name}: no {link}/Sphere -> skipped")
            continue
        axis, jpath = wheel_axis(stage, robot, link)
        if axis is None:
            log(f"{name}: back_wheel revolute joint not found -> skipped")
            continue
        m = UsdGeom.Xformable(col).GetLocalTransformation()
        c = m.Transform(Gf.Vec3d(0, 0, 0))
        r = UsdGeom.Sphere(col).GetRadiusAttr().Get() * m.TransformDir(Gf.Vec3d(1, 0, 0)).GetLength()
        ref = Gf.Vec3d(0, 0, 1) if abs(axis[2]) < 0.9 else Gf.Vec3d(1, 0, 0)
        u = Gf.Cross(axis, ref).GetNormalized()
        v = Gf.Cross(axis, u).GetNormalized()
        for old in lobes(stage, name):
            stage.RemovePrim(old.GetPath())
        log(f"{name}: wheel radius {r * 1000:.0f} mm, axis {tuple(round(x, 2) for x in axis)} ({jpath}), {N_KNOB} lobes")
        for k in range(N_KNOB):
            ang = 2 * math.pi * k / N_KNOB
            make_one(stage, f"{link}/wlobe" + (f"_{k}" if k else ""), col, axis, c, r,
                     u * math.cos(ang) + v * math.sin(ang), k)
except Exception:
    import traceback
    log("ERROR: " + traceback.format_exc())
finally:
    with open(LOG, "w") as f:
        f.write("\n".join(lines) + "\n")
