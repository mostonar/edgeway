#!/bin/bash
# Copy the nav2_bringup launch files into launch/ and route behavior_server (Spin, BackUp,
# DriveOnHeading) cmd_vel through the velocity smoother (cmd_vel -> cmd_vel_nav).
# Stock Humble only routes controller_server through the smoother.
#   ./patch_nav2_launch.sh     (run_nav2.sh calls this automatically if launch/ is missing)
set -e
cd "$(dirname "$0")"
SRC="$(ros2 pkg prefix nav2_bringup)/share/nav2_bringup/launch"
[ -f "$SRC/navigation_launch.py" ] || { echo "nav2_bringup launch folder not found: $SRC"; exit 1; }
mkdir -p launch
cp "$SRC/bringup_launch.py" "$SRC/navigation_launch.py" launch/
echo "Copied: $SRC -> launch/"

python3 - <<'PY'
import re

# navigation_launch.py: add cmd_vel -> cmd_vel_nav to both behavior_server blocks (Node and Composable)
p = 'launch/navigation_launch.py'
s = open(p).read()
pat = re.compile(r"(name='behavior_server'.*?remappings=remappings)(\s*\))", re.S)
s, n = pat.subn(r"\1 + [('cmd_vel', 'cmd_vel_nav')]\2", s)
if n != 2:
    raise SystemExit(f'! Patched {n} behavior_server blocks (expected 2: Node/Composable). Check launch/navigation_launch.py')
open(p, 'w').write(s)
print(f'{p}: behavior_server cmd_vel -> cmd_vel_nav ({n} places)')

# bringup_launch.py: use the patched navigation_launch.py in this folder
p = 'launch/bringup_launch.py'
s = open(p).read()
old = "os.path.join(launch_dir, 'navigation_launch.py')"
if s.count(old) != 1:
    raise SystemExit(f'! navigation_launch path not found in bringup_launch.py. Check it manually')
s = s.replace(old, "os.path.join(os.path.dirname(os.path.abspath(__file__)), 'navigation_launch.py')")
open(p, 'w').write(s)
print(f'{p}: uses launch/navigation_launch.py')
PY

echo "Done. run_nav2.sh uses launch/bringup_launch.py"
