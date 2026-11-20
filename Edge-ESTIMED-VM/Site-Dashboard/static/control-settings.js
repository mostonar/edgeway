"use strict";
(() => {
  let saved = null, profiles = [], busy = false, dirty = false;
  const fields = $("control-fields"), message = $("control-feedback");
  function say(text, error = false) { message.textContent = text; message.classList.toggle("binding-error", error); }
  function draft() {
    return {robot_id: $("control-robot").value.trim(), command_path: $("control-command").value.trim(),
      result_path: $("control-result").value.trim(), enabled: $("control-enabled").checked,
      ack_timeout_seconds: Number($("control-timeout").value), expected_revision: saved?.revision};
  }
  function addresses() {
    const d = draft(), base = saved?.cse_url || "";
    $("control-addresses").textContent = [d.command_path && `Command: ${base}/${d.command_path.replace(/^\/+|\/+$/g, "")}`,
      d.result_path && `Result: ${base}/${d.result_path.replace(/^\/+|\/+$/g, "")}`].filter(Boolean).join(" · ");
  }
  function edit(robot) {
    const target = saved.targets[robot];
    $("control-robot").value = robot;
    $("control-command").value = target?.command_path || "";
    $("control-result").value = target?.result_path || "";
    $("control-timeout").value = target?.ack_timeout_seconds ?? 30;
    $("control-enabled").checked = target?.enabled ?? false;
    $("control-result-paths").replaceChildren(...profiles.filter(p => p.robot_id === robot).map(p => new Option(p.resource_path, p.resource_path)));
    dirty = false; addresses();
    say(target ? "Saved settings. Changes take effect when saved." : "Enter paths for the new robot. Control is disabled by default.");
  }
  function render() {
    const list = $("control-list"); list.replaceChildren();
    if (!Object.keys(saved.targets).length) list.append(node("p", "No robot control paths saved.", "empty-copy"));
    for (const [robot, target] of Object.entries(saved.targets)) {
      const row = node("article", undefined, "binding-item"), info = node("div"), paths = node("div");
      const heading = node("h3", robot), badge = node("span", target.enabled ? "Control enabled" : "Disabled", "status-badge");
      badge.dataset.state = target.enabled ? "active" : "inactive"; heading.append(badge);
      info.append(heading, node("p", `Response timeout ${target.ack_timeout_seconds} s`, "hint"));
      paths.append(node("p", "Command · " + target.command_path, "mono"), node("p", "Result · " + target.result_path, "mono"));
      const button = node("button", "Edit settings", "secondary"); button.type = "button";
      button.addEventListener("click", () => { if (busy) return; if (dirty && !confirm("Discard unsaved changes and reload saved settings?")) return; edit(robot); $("control-form").scrollIntoView({block: "center", behavior: "smooth"}); });
      row.append(info, paths, button); list.append(row);
    }
    $("control-source").textContent = saved.source === "dashboard" ? "Saved in the dashboard · Persists after restart." : "Showing configuration file values. Saved dashboard settings take precedence.";
    const robots = [...new Set([...profiles.map(p => p.robot_id), ...Object.keys(saved.targets)])];
    $("control-robots").replaceChildren(...robots.map(r => new Option(r, r)));
  }
  async function task(fn) {
    if (busy) return; busy = true; fields.disabled = true; $("control-reload").disabled = true;
    try { await fn(); } catch (e) { say(e.message, true); }
    finally { busy = false; fields.disabled = !saved; $("control-reload").disabled = false; }
  }
  async function load() {
    const [control, settings] = await Promise.all([api("/api/control/settings"), api("/api/settings")]);
    csrf = control.csrf; saved = control; profiles = settings.bindings; render();
    edit($("control-robot").value || Object.keys(saved.targets)[0] || profiles[0]?.robot_id || "");
  }
  $("control-reload").addEventListener("click", () => { if (dirty && !confirm("Discard unsaved changes and reload?")) return; task(load); });
  $("control-robot").addEventListener("change", () => edit($("control-robot").value.trim()));
  fields.addEventListener("input", () => { dirty = true; addresses(); say("Save your changes. Previous path checks no longer apply."); });
  $("control-defaults").addEventListener("click", () => {
    const robot = $("control-robot").value.trim();
    if (!/^[A-Za-z0-9_-]{1,80}$/.test(robot)) { say("Robot ID is required first.", true); return; }
    if (($("control-command").value || $("control-result").value) && !confirm("Replace the entered paths with defaults for this robot?")) return;
    $("control-command").value = robot + "/commands"; $("control-result").value = robot + "/results";
    dirty = true; addresses(); say("Default path examples inserted. Confirm the paths with the IPE team before saving.");
  });
  $("control-subscription").addEventListener("click", () => task(async () => {
    const d = draft(); if (!d.robot_id || !d.result_path) throw Error("Robot ID and result path are required first.");
    const settings = await api("/api/settings"); profiles = settings.bindings;
    const existing = profiles.find(p => p.resource_path === d.result_path.replace(/^\/+|\/+$/g, ""));
    if (existing && existing.robot_id !== d.robot_id) throw Error("This path is registered to another robot.");
    const channel = existing?.channel || ["custom", "status", "telemetry", "log"].find(c => !profiles.some(p => p.robot_id === d.robot_id && p.channel === c));
    if (!channel) throw Error("All data types for this robot are in use. Select an existing result subscription.");
    $("robot-id").value = d.robot_id; $("robot-name").value = profiles.find(p => p.robot_id === d.robot_id)?.name || d.robot_id;
    $("resource-path").value = d.result_path; $("binding-channel").value = channel;
    showSettingsSection("subscriptions-title"); $("subscription-create").open = true;
    $("binding-form").scrollIntoView({block: "center", behavior: "smooth"}); $("resource-path").focus();
    feedback("Subscription form filled. Select Subscribe under Robot Connections to create and subscribe to the result CNT.");
  }));
  $("control-check").addEventListener("click", () => { if (!$("control-form").reportValidity()) return; task(async () => { const result = await api("/api/control/check", "POST", draft()); say(result.message); }); });
  $("control-form").addEventListener("submit", event => {
    event.preventDefault(); const data = draft();
    task(async () => {
      const result = await api("/api/control/settings", "PUT", data); saved = {...saved, ...result, source: "dashboard"}; render(); edit(data.robot_id);
      say(data.enabled ? "Saved · Command path and result subscription verified. Operator controls are available. No command was sent." : "Saved · Control is disabled. Settings persist after restart.");
    });
  });
  task(load);
})();
