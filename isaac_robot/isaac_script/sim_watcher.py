# Sim watcher: applies demo commands to the running simulation.
# Run once in Isaac Sim Script Editor (Play or Stop); running it again replaces the old one.
# Every 0.5 s it reads ~/sim_cmd (one line, written by the demo scripts), e.g.
#     seq=1730000000 robot=site_c_AMR_1 cargo=250 knob=on
# On a new seq it applies the settings to that robot and writes ~/sim_ack:
#     seq=1730000000 ok robot=site_c_AMR_1 ...   or   seq=1730000000 error <reason>
# Keys (Play state only):
#   robot   : site_a_AMR_1 ... site_c_AMR_2 or /World/... (default DEFAULT_ROBOT)
#   cargo   : kg, lift mass = BASE_KG + cargo
#   knob    : on / off, front-left roller lobe collision (created if missing)
#   wknob   : on / off, rear drive wheel lobes back_wheel/wlobe* (create first: make_wheel_lobe.py, MODE = "make")
#   bearing : FL roller joint viscous friction (N*m*s/rad); 0 = original value
#   tire    : normal / oil (needs robot_anomaly_apply.py run once in Stop for that robot)
# Stop it: import builtins; builtins._sim_watch_sub.unsubscribe()
# Log: ~/script_log (appended)
import builtins, os, re, time, traceback
import omni.kit.app, omni.usd, omni.timeline
from pxr import Usd, UsdGeom, UsdPhysics, UsdShade, PhysxSchema

DEFAULT_ROBOT = "site_a_AMR_1"
ROBOT_PATTERN = r"^site_.+_AMR_\d+$"
CMD = os.path.expanduser("~/sim_cmd")
ACK = os.path.expanduser("~/sim_ack")
LOG = os.path.expanduser("~/script_log")
BASE_KG = 465.0
MAT_NORMAL = "/World/PhysicsMaterials/tire_normal"
MAT_OIL = "/World/PhysicsMaterials/tire_oil"
R_ROLLER, R_KNOB, H = 0.08, 0.02, 0.001   # m; knob lobe sticks out 1 mm
JOINT_FL = "front_left_roller"            # same as joint_roller_fl in ipe_common.yaml
BEARING_ORIG = builtins.__dict__.setdefault("_bearing_orig", {})   # per robot, survives re-running the watcher

old = getattr(builtins, "_sim_watch_sub", None)
if old is not None:
    old.unsubscribe()
state = {"t": 0.0, "seq": None}
try:                                   # ignore a command left over from before start
    state["seq"] = dict(kv.split("=", 1) for kv in open(CMD).read().split() if "=" in kv).get("seq")
except Exception:
    pass

def log(s):
    with open(LOG, "a") as f:
        f.write(time.strftime("%H:%M:%S ") + s + "\n")


def paths(robot):
    r = robot if robot.startswith("/") else f"/World/{robot}"
    name = r.split("/")[-1]
    if not re.match(ROBOT_PATTERN, name):
        raise RuntimeError(f"bad robot name {name} (expected site_x_AMR_n)")
    return {
        "robot": r,
        "name": name,
        "tire_col": f"{r}/back_wheel/Sphere",
        "mat_active": f"/World/PhysicsMaterials/tire_active_{name}",
        "roller_col": f"{r}/roller_front_left/Cylinder_03",
        "lobe": f"{r}/roller_front_left/lobe",
        "wlobe": f"{r}/back_wheel/wlobe",
    }


def set_mass(stage, P, cargo):
    from isaacsim.core.experimental.prims import Articulation
    total = BASE_KG + cargo
    UsdPhysics.MassAPI.Apply(stage.GetPrimAtPath(P["robot"] + "/lift")).CreateMassAttr().Set(total)
    vis = stage.GetPrimAtPath(P["robot"] + "/lift/load")
    if vis.IsValid():
        img = UsdGeom.Imageable(vis)
        img.MakeVisible() if cargo > 0 else img.MakeInvisible()
    art = Articulation(P["robot"])
    k = art.link_names.index("lift")
    m = art.get_link_masses().numpy().copy()
    m[..., k] = total
    art.set_link_masses(m)
    return float(art.get_link_masses().numpy()[..., k].ravel()[0])


def set_tire(stage, P, oil):
    tire = stage.GetPrimAtPath(P["tire_col"])
    bound, _ = UsdShade.MaterialBindingAPI(tire).ComputeBoundMaterial("physics")
    if not bound or str(bound.GetPath()) != P["mat_active"]:
        raise RuntimeError(f"{P['mat_active']} not bound: Stop, run robot_anomaly_apply.py for this robot, Play")
    src = UsdPhysics.MaterialAPI(stage.GetPrimAtPath(MAT_OIL if oil else MAT_NORMAL))
    dst = UsdPhysics.MaterialAPI(stage.GetPrimAtPath(P["mat_active"]))
    dst.CreateStaticFrictionAttr().Set(float(src.GetStaticFrictionAttr().Get()))
    dst.CreateDynamicFrictionAttr().Set(float(src.GetDynamicFrictionAttr().Get()))
    rs = src.GetRestitutionAttr().Get()
    if rs is not None:
        dst.CreateRestitutionAttr().Set(float(rs))
    return float(dst.GetStaticFrictionAttr().Get())


def make_lobe(stage, P):
    rc = stage.GetPrimAtPath(P["roller_col"])
    ops = UsdGeom.Xformable(rc).GetOrderedXformOps()
    s = UsdGeom.Cylinder(rc)
    k = UsdGeom.Cylinder.Define(stage, P["lobe"])
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


def set_knob(stage, P, on):
    if not stage.GetPrimAtPath(P["lobe"]).IsValid():
        make_lobe(stage, P)
    col = UsdPhysics.CollisionAPI(stage.GetPrimAtPath(P["lobe"]))
    col.CreateCollisionEnabledAttr().Set(bool(on))
    return col.GetCollisionEnabledAttr().Get()


def set_wknob(stage, P, on):
    wheel = stage.GetPrimAtPath(P["wlobe"].rsplit("/", 1)[0])
    lobes = [c for c in wheel.GetChildren() if c.GetName().startswith("wlobe")] if wheel.IsValid() else []
    if not lobes:
        raise RuntimeError(f"{P['wlobe']}* not found: Stop, run make_wheel_lobe.py (MODE = 'make'), Play")
    for lb in lobes:
        UsdPhysics.CollisionAPI(lb).CreateCollisionEnabledAttr().Set(bool(on))
    n_on = sum(bool(UsdPhysics.CollisionAPI(lb).GetCollisionEnabledAttr().Get()) for lb in lobes)
    return f"{n_on}/{len(lobes)}"


def set_bearing(stage, P, v):
    from isaacsim.core.experimental.prims import Articulation
    art = Articulation(P["robot"])
    names = list(art.dof_names)
    if JOINT_FL not in names:
        raise RuntimeError(f"{JOINT_FL} not in dof_names {names}")
    k = names.index(JOINT_FL)
    st, dy, vi = [x.numpy().copy() for x in art.get_dof_friction_properties()]
    orig = BEARING_ORIG.setdefault(P["name"], float(vi[..., k].ravel()[0]))
    vi[..., k] = orig if v <= 0 else v
    art.set_dof_friction_properties(static_frictions=st, dynamic_frictions=dy, viscous_frictions=vi)
    got = float(art.get_dof_friction_properties()[2].numpy()[..., k].ravel()[0])
    return f"{got:.3f}(orig {orig:.3f})"


def apply(c):
    if not omni.timeline.get_timeline_interface().is_playing():
        raise RuntimeError("simulation not playing")
    stage = omni.usd.get_context().get_stage()
    P = paths(c.pop("robot", DEFAULT_ROBOT))
    if not stage.GetPrimAtPath(P["robot"]).IsValid():
        raise RuntimeError(f"{P['robot']} not found")
    unknown = set(c) - {"cargo", "tire", "knob", "wknob", "bearing"}
    if unknown:                                    # never ignore a key silently
        raise RuntimeError(f"unknown key {sorted(unknown)}")
    out = [f"robot={P['name']}"]
    if "cargo" in c:
        out.append(f"mass={set_mass(stage, P, float(c['cargo'])):.0f}")
    if "tire" in c:
        if c["tire"] not in ("normal", "oil"):
            raise RuntimeError("tire must be normal / oil")
        out.append(f"tire={set_tire(stage, P, c['tire'] == 'oil'):.3f}")
    if "knob" in c:
        if c["knob"] not in ("on", "off"):
            raise RuntimeError("knob must be on / off")
        out.append(f"knob={set_knob(stage, P, c['knob'] == 'on')}")
    if "wknob" in c:
        if c["wknob"] not in ("on", "off"):
            raise RuntimeError("wknob must be on / off")
        out.append(f"wknob={set_wknob(stage, P, c['wknob'] == 'on')}")
    if "bearing" in c:
        out.append(f"bearing={set_bearing(stage, P, float(c['bearing']))}")
    return " ".join(out)


def on_update(e):
    now = time.time()
    if now - state["t"] < 0.5:
        return
    state["t"] = now
    try:
        line = open(CMD).read().strip()
    except Exception:
        return
    c = dict(kv.split("=", 1) for kv in line.split() if "=" in kv)
    seq = c.pop("seq", None)
    if seq is None or seq == state["seq"]:
        return
    state["seq"] = seq
    try:
        res = f"seq={seq} ok {apply(c)}"
    except Exception as ex:
        res = f"seq={seq} error {ex}"
        log(traceback.format_exc())
    with open(ACK, "w") as f:
        f.write(res + "\n")
    log(f"{line} -> {res}")


builtins._sim_watch_sub = omni.kit.app.get_app().get_update_event_stream() \
    .create_subscription_to_pop(on_update, name="sim_watch")
log("sim watcher started, cmd " + CMD)
