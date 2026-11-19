"use strict";

// Reset is a previewed, explicit operation across every factory, independent of filters.
document.addEventListener('DOMContentLoaded', () => {
  const dialog = document.getElementById('records-reset-dialog');
  const open = document.getElementById('reset-records-open');
  const confirm = document.getElementById('records-reset-confirm');
  const cancel = document.getElementById('records-reset-cancel');
  const status = document.getElementById('records-reset-status');
  let preview = null, busy = false, generation = 0;
  dialog.addEventListener('cancel', event => { if (busy) event.preventDefault(); });
  dialog.addEventListener('close', () => { generation++; preview = null; open.focus(); });
  cancel.onclick = () => { if (!busy) dialog.close(); };
  open.onclick = async () => {
    const current = ++generation;
    preview = null; confirm.disabled = true;
    document.getElementById('records-reset-count').textContent = '';
    status.textContent = 'Checking stored records…';
    dialog.showModal();
    try {
      const response = await fetch('/api/records/reset', {cache: 'no-store', signal: AbortSignal.timeout(8000)});
      const data = await response.json();
      if (current !== generation || !dialog.open) return;
      if (!response.ok) throw Error(data.error || 'Could not preview records.');
      if (!Number.isSafeInteger(data.count) || data.count < 0 || data.scope !== 'all_factories') throw Error('Invalid cleaning preview. Reload and try again.');
      preview = data;
      document.getElementById('records-reset-count').textContent = `${data.event_count.toLocaleString('en-GB')} records and ${data.marker_count.toLocaleString('en-GB')} floor markers in this snapshot.`;
      status.textContent = ''; confirm.disabled = false;
    } catch (error) {
      if (current === generation) status.textContent = 'Could not preview records. ' + error.message;
    }
  };
  confirm.onclick = async () => {
    if (!preview || busy) return;
    busy = true; confirm.disabled = cancel.disabled = true;
    status.textContent = 'Cleaning records…';
    try {
      const response = await fetch('/api/records/reset', {
        method: 'POST', headers: {'Content-Type': 'application/json', 'X-EdgeWay-CSRF': preview.csrf},
        body: JSON.stringify({confirm: 'reset_all_records', through_id: preview.through_id, fingerprint: preview.fingerprint, marker_snapshot: preview.marker_snapshot}),
        signal: AbortSignal.timeout(30000)
      });
      const data = await response.json();
      if (!response.ok) throw Error(data.error || 'Cleaning failed.');
      localStorage.removeItem('edgeway.floor-alerts.v1');
      location.reload(); // Clears map/overview caches, pending reads and event state together.
    } catch (error) {
      status.textContent = 'Cleaning not confirmed. Reload to check records before trying again. ' + error.message;
    } finally {
      busy = false; cancel.disabled = false; preview = null;
    }
  };
});

const $ = (id) => document.getElementById(id);
const factoryStorageKey='edgeway.selected-factory.v1';
const factoryChoices=new Set(['scenario-1','scenario-2','scenario-3']);
function initialFactory(){
  try {const saved=sessionStorage.getItem(factoryStorageKey);if(factoryChoices.has(saved))return saved;} catch {}
  return 'scenario-1';
}
const view = {
  events: new Map(), groups: new Map(), oldestId: Infinity, robot: "", factory: initialFactory(), source: null, generation: 0, latestSeen: 0,
  recordsRevision: null, total: 0, hasMore: false, pending: 0, loading: false, robots: [], selected: null
};
const knownRobots = new Map();
const mapSeeded = new Set();
const factoryOf = id => {const site=/^site([ABC])_/.exec(id||"")?.[1];return site?"scenario-"+("ABC".indexOf(site)+1):null;};
window.edgewayFactory = view.factory;
window.edgewayFactoryOf = factoryOf;
$("fp-scenario").value=view.factory;
function rememberRobot(robot) {
  if(robot?.id&&factoryOf(robot.id))knownRobots.set(robot.id,robot);
}
function syncRobotChoices() {
  const robots=[...knownRobots.values()].filter(r=>factoryOf(r.id)===view.factory).sort((a,b)=>a.id.localeCompare(b.id));
  const old=view.robot;
  if(view.robot&&!robots.some(r=>r.id===view.robot))view.robot="";
  const options=[new Option("All robots",""),...robots.map(r=>new Option(r.id,r.id))];
  $("robot-filter").replaceChildren(...options);$("robot-filter").value=view.robot;$("robot-filter").disabled=view.loading||!robots.length;
  return old!==view.robot;
}
function clearRobotView() {
  if($("log-dialog").open)$("log-dialog").close();
  selectEvent(null);
  if($("event-dialog").open)$("event-dialog").close();
  clearTimeout(logRenderTimer);logRenderTimer=null;
  view.events.clear();view.groups.clear();view.oldestId=Infinity;view.pending=0;view.total=0;view.hasMore=false;
  $("timeline").replaceChildren();$("new-banner").hidden=true;updateCount();updateEmpty();
}
async function seedFactoryMap() {
  const robots=[...knownRobots.values()].filter(r=>factoryOf(r.id)===view.factory&&!mapSeeded.has(r.id));
  for(const robot of robots){
    mapSeeded.add(robot.id);
    try{
      const data=await api("/api/events?robot="+encodeURIComponent(robot.id)+"&limit=50");
      window.dispatchEvent(new CustomEvent("edgeway:event-records",{detail:{events:data.events}}));
    }catch{mapSeeded.delete(robot.id);}
  }
}
async function loadFactoryEvents(before=null) {
  const factory=view.factory, robot=view.robot;
  const ids=robot?[robot]:[...knownRobots.keys()].filter(id=>factoryOf(id)===factory);
  const pages=await Promise.all(ids.map(id=>api('/api/events?robot='+encodeURIComponent(id)+'&limit=50'+(before?'&before='+before:''))));
  if(!pages.length)return {events:[],total:0,has_more:false,latest_id:view.latestSeen,records_revision:view.recordsRevision};
  if(new Set(pages.map(p=>p.records_revision)).size>1)throw Error('Records changed while loading.');
  const rows=pages.flatMap(p=>p.events).sort((a,b)=>b.id-a.id);
  return {events:rows.slice(0,50),total:pages.reduce((n,p)=>n+p.total,0),
    has_more:rows.length>50||pages.some(p=>p.has_more),
    latest_id:Math.min(...pages.map(p=>p.latest_id)),records_revision:pages[0].records_revision};
}
function changeFactory(factory) {
  if(!factoryChoices.has(factory)||factory===view.factory)return;
  view.factory=factory;window.edgewayFactory=factory;
  try {sessionStorage.setItem(factoryStorageKey,factory);} catch {}
  view.generation++;view.source?.close();clearRobotView();view.robot="";syncRobotChoices();
  window.dispatchEvent(new CustomEvent("edgeway:factory-change",{detail:{factory}}));
  loadLatest();seedFactoryMap();
}
$("fp-scenario").addEventListener("change",()=>changeFactory($("fp-scenario").value));
const timeFormat = new Intl.DateTimeFormat("en-GB", {
  timeZone: "Asia/Seoul", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false
});
const dayFormat = new Intl.DateTimeFormat("en-GB", {
  timeZone: "Asia/Seoul", month: "2-digit", day: "2-digit"
});
const fullFormat = new Intl.DateTimeFormat("en-GB", {
  timeZone: "Asia/Seoul", year: "numeric", month: "long", day: "numeric",
  hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false
});

async function api(url) {
  const response = await fetch(url, {cache: "no-store", signal: AbortSignal.timeout(8000)});
  if (!response.ok) throw new Error(`Request failed (${response.status})`);
  return response.json();
}

function setStream(state, label) {
  $("stream-state").dataset.state = state;
  $("stream-label").textContent = label;
}

function showError(message) {
  $("error-text").textContent = message;
  $("error-banner").hidden = false;
}

function announce(message) { $("announcer").textContent = message; }

function updateCount() {
  $("event-count").textContent = view.events.size.toLocaleString("en-GB");
  $("event-count").setAttribute("aria-label", `${view.events.size} state changes and events loaded`);
  $("load-more").hidden = true;
}

function openLog(id) {
  if(!$("log-dialog").open)$("log-dialog").showModal();
  $("close-log").focus();
  selectEvent(id);
}
$("close-log").addEventListener("click",()=> $("log-dialog").close());
$("log-dialog").addEventListener("close",()=>{if(!$("log-dialog").open)selectEvent(null);});
let selectionRequest = 0;
async function selectEvent(id) {
  const request = ++selectionRequest;
  view.selected = id;
  for (const row of $("timeline").children) {
    const selected = Number(row.dataset.eventId) === id;
    row.dataset.selected = String(selected);
    row.querySelector("button").setAttribute("aria-pressed", String(selected));
  }
  window.dispatchEvent(new CustomEvent("edgeway:selection", {detail:{loading:id !== null}}));
  if (id === null) return;
  try {
    const event = await api(`/api/events/${id}`);
    if (request !== selectionRequest || factoryOf(event.robot_id)!==view.factory) return;
    window.dispatchEvent(new CustomEvent("edgeway:selection", {detail:{event}}));
  } catch {
    if (request === selectionRequest) window.dispatchEvent(new CustomEvent("edgeway:selection", {detail:{error:"Could not load details. Select the log entry again."}}));
  }
}

function eventRow(event) {
  const item = document.createElement("tr");
  item.className = "event-item";
  item.dataset.eventId = String(event.id);
  item.dataset.summaryKey = event.summaryKey||'';
  item.dataset.summaryCount = String(event.summaryCount||1);
  item.dataset.level = event.display?.level || "info";
  item.dataset.selected = String(event.id === view.selected);
  const display = event.display || {title:"Data received",target:event.robot_name};
  const timeCell = document.createElement("td");
  const time = document.createElement("time");
  time.className = "event-time";
  time.dateTime = event.received_at;
  time.title = fullFormat.format(new Date(event.received_at));
  time.textContent = timeFormat.format(new Date(event.received_at));
  timeCell.append(time);
  const targetCell = document.createElement("td");
  targetCell.className = "robot-name";
  targetCell.textContent = display.target;
  targetCell.title = display.target;
  const eventCell = document.createElement("td");
  const button = document.createElement("button");
  button.type = "button";
  button.className = "event-button";
  button.setAttribute("aria-label", `${display.target}, ${fullFormat.format(new Date(event.received_at))}, ${display.title}, View details`);
  button.setAttribute("aria-pressed", String(event.id === view.selected));
  const preview = document.createElement("span");
  preview.className = "event-preview";
  preview.textContent = display.title;
  button.title = display.title + " - Open original event details.";
  button.append(preview);
  eventCell.append(button);
  item.append(timeCell, targetCell, eventCell);
  item.addEventListener("click", () => openLog(event.id));
  return item;
}

function updateEmpty() {
  $("empty-state").hidden = view.events.size > 0;
  if (!view.events.size) {
    $("empty-title").textContent = "No state changes or events";
    $("empty-description").textContent = view.robot
      ? "State changes and special events from the selected robot appear here."
      : "Repeated status updates are hidden. Live positions continue updating.";
  }
}

let logRenderTimer=null,olderLoadTimer=null,olderRetryAt=0;
$("timeline-scroll").style.overflowAnchor='none';
function scheduleOlderRecords(delay=250){
  clearTimeout(olderLoadTimer);
  olderLoadTimer=setTimeout(()=>{
    const scroller=$("timeline-scroll");
    if(document.hidden||view.loading||!view.hasMore||!scroller.getClientRects().length)return;
    if(Date.now()<olderRetryAt){scheduleOlderRecords(olderRetryAt-Date.now());return;}
    if(scroller.scrollHeight-scroller.scrollTop-scroller.clientHeight<120)loadMore();
  },delay);
}
function logAnchor(){
  const scroller=$("timeline-scroll"),top=scroller.getBoundingClientRect().top;
  const first=scroller.scrollTop>10?[...$("timeline").children].find(row=>row.getBoundingClientRect().bottom>top+30):null;
  const focused=$("timeline").contains(document.activeElement)?document.activeElement.closest('[data-summary-key]')?.dataset.summaryKey:null;
  return {key:first?.dataset.summaryKey,offset:first?first.getBoundingClientRect().top-top:0,scroll:scroller.scrollTop,focused};
}
function restoreLogAnchor(anchor){
  const scroller=$("timeline-scroll"),rows=[...$("timeline").children];
  const first=anchor.key?rows.find(row=>row.dataset.summaryKey===anchor.key):null;
  if(first)scroller.scrollTop+=first.getBoundingClientRect().top-scroller.getBoundingClientRect().top-anchor.offset;
  else scroller.scrollTop=anchor.scroll<=10?0:anchor.scroll;
  if(anchor.focused)rows.find(row=>row.dataset.summaryKey===anchor.focused)?.querySelector('button')?.focus({preventScroll:true});
}

function addLog(event){
  view.oldestId=Math.min(view.oldestId,event.id);return EventLogSummary.add(view.groups,event);
}
function renderLogs(){
  clearTimeout(logRenderTimer);logRenderTimer=null;
  const anchor=logAnchor();
  const rows=EventLogSummary.rows(view.groups);
  view.events=new Map(rows.map(e=>[e.id,e]));
  $("timeline").replaceChildren(...rows.map(eventRow));
  updateEmpty();updateCount();restoreLogAnchor(anchor);scheduleOlderRecords();
}
function insertLive(event,reading=false) {
  if(!addLog(event))return;
  // Keep already loaded history and update live rows even while reading older entries.
  view.pending=0;$("new-banner").hidden=true;
  if(!logRenderTimer)logRenderTimer=setTimeout(()=>{renderLogs();announce('Event log updated.');},100);
  // Telemetry has already reached the map; duplicate statuses do not redraw logs.
}

function startStream(after, generation) {
  const source = new EventSource(`/api/events/stream?after=${after}&records_revision=${view.recordsRevision ?? 0}`);
  view.source = source;
  source.onopen = () => {
    if (generation === view.generation) {
      setStream("connected", "Connected");
      $("error-banner").hidden = true;
    }
  };
  source.onerror = () => {
    if (generation === view.generation) setStream("waiting", "Reconnecting");
  };
  source.addEventListener("records-state", (message) => {
    if (generation !== view.generation) return;
    try {
      const {revision} = JSON.parse(message.data);
      if (view.recordsRevision !== null && revision !== view.recordsRevision) { localStorage.removeItem('edgeway.floor-alerts.v1'); location.reload(); }
      else view.recordsRevision = revision;
    } catch {}
  });
  source.addEventListener("log", (message) => {
    if (generation !== view.generation) return;
    let event;
    try { event = JSON.parse(message.data); } catch { return; }
    if (event.id <= view.latestSeen) return;
    view.latestSeen = event.id;
    updateLastReceived(event.received_at);
    rememberRobot({id:event.robot_id,name:event.robot_name});
    const changed=syncRobotChoices();
    window.dispatchEvent(new CustomEvent("edgeway:telemetry-received",{detail:{event}}));
    if(changed){clearRobotView();loadLatest();return;}
    if ((view.robot && event.robot_id !== view.robot) || factoryOf(event.robot_id)!==view.factory) return;
    view.total += 1;
    const reading = $("timeline-scroll").scrollTop > 10 || $("event-dialog").open || $("log-dialog").open
      || $("timeline").contains(document.activeElement) || document.querySelector(".admin-panel").contains(document.activeElement);
    insertLive(event,reading);
    updateCount();
  });
}

async function loadLatest() {
  clearTimeout(logRenderTimer);logRenderTimer=null;
  const generation = ++view.generation;
  view.source?.close();
  view.loading = true;
  $("robot-filter").disabled = true;
  $("load-more").disabled = true;
  setStream("waiting", "Connecting");
  try {
    const data = await loadFactoryEvents();
    if (generation !== view.generation) return;
    if (data.records_revision != null && view.recordsRevision !== null && view.recordsRevision !== data.records_revision) { localStorage.removeItem('edgeway.floor-alerts.v1'); location.reload(); return; }
    view.recordsRevision = data.records_revision ?? view.recordsRevision;
    view.groups.clear();view.oldestId=Infinity;
    for(const event of [...data.events].sort((a,b)=>a.id-b.id))addLog(event);
    view.total = data.total;
    view.hasMore = data.has_more;
    view.latestSeen = data.latest_id;
    view.pending = 0;
    $("new-banner").hidden = true;
    renderLogs();
    $("timeline-scroll").scrollTop = 0;
    $("error-banner").hidden = true;
    updateEmpty();
    updateCount();
    startStream(data.latest_id, generation);
    window.dispatchEvent(new CustomEvent("edgeway:events-loaded", {detail:{robotId:view.robot,total:data.total,events:data.events}}));
    selectEvent(view.events.has(view.selected) ? view.selected : null);
  } catch {
    if (generation !== view.generation) return;
    setStream("waiting", "Disconnected");
    showError("Could not load records. Check that the dashboard server is running.");
    window.dispatchEvent(new CustomEvent("edgeway:events-loaded", {detail:{robotId:view.robot,error:true}}));
    if (!view.events.size) {
      $("empty-title").textContent = "Waiting for the server";
      $("empty-description").textContent = "Start the server, then select ‘Retry’.";
    }
  } finally {
    if (generation === view.generation) {
      view.loading = false;
      $("robot-filter").disabled = ![...knownRobots.keys()].some(id=>factoryOf(id)===view.factory);
      $("load-more").disabled = false;scheduleOlderRecords();
    }
  }
}

async function loadMore() {
  if (view.loading || !view.hasMore || !Number.isFinite(view.oldestId)) return;
  const generation = view.generation;
  view.loading = true;
  $("load-more").disabled = true;
  try {
    const before = view.oldestId;
    const data = await loadFactoryEvents(before);
    if (generation !== view.generation) return;
    if(data.records_revision!=null&&view.recordsRevision!=null&&data.records_revision!==view.recordsRevision){localStorage.removeItem('edgeway.floor-alerts.v1');location.reload();return;}
    for (const event of [...data.events].sort((a,b)=>b.id-a.id))addLog(event);
    view.hasMore=data.has_more&&view.oldestId<before; // Stop if the server did not advance its cursor.
    olderRetryAt=0;renderLogs();
    updateCount();
    window.dispatchEvent(new CustomEvent('edgeway:event-records',{detail:{events:data.events}}));
    announce(`Loaded ${data.events.length} older records.`);
  } catch { if(generation===view.generation){olderRetryAt=Date.now()+3000;showError("Could not load older records. Retrying automatically.");} }
  finally {
    if (generation === view.generation) {
      view.loading = false;
      $("load-more").disabled = false;scheduleOlderRecords();
    }
  }
}

function updateLastReceived(value) {
  $("summary-received").textContent = value ? timeFormat.format(new Date(value)) : "No data received";
  $("summary-received-date").textContent = value ? dayFormat.format(new Date(value)) + " · KST" : "No records received yet";
  $("last-received").textContent = value ? `Last received ${dayFormat.format(new Date(value))} ${timeFormat.format(new Date(value))}` : "No records received";
}

async function refreshStatus() {
  try {
    const status = await api("/api/status");
    view.robots=status.robots||[];
    window.dispatchEvent(new CustomEvent("edgeway:catalogue",{detail:{robots:view.robots}}));
    const previousIds=[...knownRobots.keys()].filter(id=>factoryOf(id)===view.factory).sort().join('|');
    for(const robot of view.robots)rememberRobot(robot);
    const changed=syncRobotChoices();
    if(changed||(!view.robot&&previousIds!==[...knownRobots.keys()].filter(id=>factoryOf(id)===view.factory).sort().join('|'))){clearRobotView();loadLatest();}
    seedFactoryMap();
    $("summary-subscriptions").textContent = String(status.subscriptions?.registered ?? 0);
    $("summary-events").textContent = Number(status.total ?? 0).toLocaleString("en-GB");
    $("summary-cse").dataset.state = status.cse.state;
    $("summary-cse").textContent = status.cse.state === "reachable" ? "Connected" : status.cse.state === "checking" ? "Checking" : "Disconnected";
    const node = $("acme-state");
    node.dataset.state = status.cse.state;
    node.textContent = status.cse.state === "reachable"
      ? (status.subscriptions?.registered ? "ACME Query successful" : "ACME Query successful · No subscriptions")
      : status.cse.state === "checking" ? "ACME Checking · Stored logs available" : "ACME Disconnected · Stored logs available";
    node.title = status.cse.state === "reachable"
      ? "CSE query status; this does not indicate the robot operating state."
      : status.cse.message;
    updateLastReceived(status.last_received_at);
  } catch {
    $("acme-state").dataset.state = "unreachable";
    $("acme-state").textContent = "ACME Status unavailable";
    $("summary-cse").dataset.state = "unknown";
    $("summary-cse").textContent = "Unavailable";
    $("summary-subscriptions").textContent = "—";
    $("summary-events").textContent = "—";
  }
}

let detailRequest = 0;
async function openDetail(id, trigger) {
  const requestId = ++detailRequest;
  $("detail-title").textContent = "Loading record";
  $("detail-time").textContent = "";
  $("detail-content").textContent = "Please wait.";
  $("detail-metadata").replaceChildren();
  $("detail-raw").textContent = "";
  $("technical-details").open = false;
  $("event-dialog").showModal();
  $("close-detail").focus();
  try {
    const event = await api(`/api/events/${id}`);
    if (requestId !== detailRequest || !$("event-dialog").open) return;
    $("detail-title").textContent = `${event.display?.target || event.robot_name} · Received record`;
    $("detail-time").textContent = `${fullFormat.format(new Date(event.received_at))} KST · Received at`;
    $("detail-content").textContent = typeof event.content === "string" ? (event.content || "(Empty string)") : JSON.stringify(event.content, null, 2);
    const pairs = [["Robot ID", event.robot_id], ["Resource path", event.resource_path], ["CIN ID", event.cin_ri],
      ["Parent CNT ID", event.parent_ri], ["CSE Created at", event.cse_created_at || "Not provided"],
      ["Subscription reference", event.subscription_ref], ["CSE URL", event.source]];
    for (const [key, value] of pairs) {
      const term = document.createElement("dt"), description = document.createElement("dd");
      term.textContent = key;
      description.textContent = value;
      $("detail-metadata").append(term, description);
    }
    $("detail-raw").textContent = event.notification_raw;
  } catch {
    $("detail-title").textContent = "Could not load record";
    $("detail-content").textContent = "Check the server connection and reopen this record.";
  }
}

$("close-detail").addEventListener("click", () => $("event-dialog").close());
$("show-raw").addEventListener("click", () => { if (view.selected !== null) openDetail(view.selected, $("show-raw")); });
$("event-dialog").addEventListener("close", () => { detailRequest += 1; });
$("robot-filter").addEventListener("change", () => { view.robot = $("robot-filter").value; clearRobotView(); loadLatest(); });
// A map selection changes only the existing log filter, never commands or layout.
window.addEventListener("edgeway:robot-request", ({detail}) => {
  const robot = detail?.robotId;
  if (typeof robot !== "string" || !/^[A-Za-z0-9_-]{1,180}$/.test(robot)) return;
  if(factoryOf(robot)!==view.factory)return;
  rememberRobot({id:robot,name:robot});
  view.robot=robot;syncRobotChoices();
  clearRobotView();
  loadLatest();
});
$("load-more").hidden=true;$("new-banner").hidden=true;
$("timeline-scroll").addEventListener('scroll',()=>scheduleOlderRecords(),{passive:true});
new ResizeObserver(()=>scheduleOlderRecords()).observe($("timeline-scroll"));
document.addEventListener('visibilitychange',()=>{if(!document.hidden)scheduleOlderRecords();});
$("retry").addEventListener("click", () => { loadLatest(); refreshStatus(); });
window.addEventListener("online", () => { if (!view.loading) loadLatest(); refreshStatus(); });
window.addEventListener("pagehide", () => view.source?.close());
window.addEventListener("pageshow", (event) => { if (event.persisted) loadLatest(); });
function today() { $("today").textContent = new Intl.DateTimeFormat("en-GB", {timeZone:"Asia/Seoul",year:"numeric",month:"2-digit",day:"2-digit",weekday:"short"}).format(new Date()); }
today();
setInterval(today, 60000);
setInterval(refreshStatus, 10000);
refreshStatus();
loadLatest();

// Focus views reuse the same panels so camera frames, drafts and selection survive.
const monitorViews = {
  events: {hash: '#events-title', selector: '.events-panel', label: 'Event Log'},
  camera: {hash: '#camera-title', selector: '.camera-panel', label: 'Site Camera'},
  map: {hash: '#map-title', selector: '.map-panel', label: 'Floor Plan'},
  admin: {hash: '#admin-title', selector: '.admin-panel', label: 'Factory Overview'}
};
const monitorScroll = new Map();
const logFilter = document.querySelector('.events-heading .filter-group');
const logFilterHome = logFilter.parentElement;
const logFilterNext = logFilter.nextElementSibling;
let monitorMode = null, monitorRevision = 0, monitorReturnFocus = null;
function syncMonitorNavigation(moveFocus = false) {
  const mode = Object.keys(monitorViews).find(key => monitorViews[key].hash === location.hash) || 'overview';
  const main = $('monitor-main');
  const activeHash = monitorViews[mode]?.hash || '#monitor-main';
  for (const link of document.querySelectorAll('.monitor-rail a[href^="#"]')) {
    if (link.hash === activeHash) link.setAttribute('aria-current', 'location');
    else link.removeAttribute('aria-current');
  }
  if (monitorMode === mode) return;
  if (monitorMode !== null) monitorScroll.set(monitorMode, {
    windowY: window.scrollY, events: $('timeline-scroll').scrollTop,
    admin: document.querySelector('.admin-content').scrollTop
  });
  monitorMode = mode;
  const revision = ++monitorRevision;
  if (mode === 'overview') delete main.dataset.focus;
  else main.dataset.focus = mode;
  const paired = mode === 'events' || mode === 'admin';
  main.classList.toggle('paired-monitor', paired);
  if (paired) $('focus-title').parentElement.append(logFilter);
  else logFilterHome.insertBefore(logFilter, logFilterNext);
  for (const [key, definition] of Object.entries(monitorViews)) {
    const panel = main.querySelector(definition.selector);
    const active = paired ? key === 'events' || key === 'admin' : key === mode;
    panel.hidden = mode !== 'overview' && !active;
    panel.classList.toggle('is-focused', active);
  }
  main.querySelector('.status-strip').hidden = true;
  $('focus-toolbar').hidden = mode === 'overview';
  $('focus-title').textContent = paired ? 'Event Log' : monitorViews[mode]?.label || '';
  main.setAttribute('aria-label', mode === 'overview' ? 'EdgeWay Operations dashboard' : monitorViews[mode].label + ' Expanded view');
  requestAnimationFrame(() => {
    if (revision !== monitorRevision) return;
    const saved = monitorScroll.get(mode);
    $('timeline-scroll').scrollTop = saved?.events || 0;
    document.querySelector('.admin-content').scrollTop = saved?.admin || 0;
    window.scrollTo(0, mode === 'overview' ? saved?.windowY || 0 : 0);
    if (moveFocus) {
      const target = mode !== 'overview' ? $('focus-title') :
        monitorReturnFocus?.isConnected && monitorReturnFocus.getClientRects().length ? monitorReturnFocus : main;
      target.focus({preventScroll: true});
    }
  });
  if (moveFocus) announce(mode === 'overview' ? 'Returned to the dashboard.' : monitorViews[mode].label + ' Expanded view');
}
function navigateMonitor(mode, trigger) {
  if (mode !== 'overview' && !monitorViews[mode]) return;
  if (monitorMode === 'overview' && mode !== 'overview') monitorReturnFocus = trigger;
  const hash = monitorViews[mode]?.hash || '#monitor-main';
  if (location.hash !== hash) history.pushState(null, '', hash);
  syncMonitorNavigation(true);
}
for (const link of document.querySelectorAll('.monitor-rail a[href^="#"]')) {
  link.addEventListener('click', event => {
    if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    const mode = Object.keys(monitorViews).find(key => monitorViews[key].hash === link.hash) || 'overview';
    navigateMonitor(mode, link);
  });
}
for (const button of document.querySelectorAll('[data-focus-target]')) {
  button.addEventListener('click', () => navigateMonitor(button.dataset.focusTarget, button));
}
$('exit-focus').addEventListener('click', () => navigateMonitor('overview'));
document.addEventListener('keydown', event => {
  if (event.key !== 'Escape' || event.isComposing || event.defaultPrevented || monitorMode === 'overview') return;
  // Let dialogs and native selects consume Escape before any view navigation.
  if (document.querySelector('dialog[open]') || event.target.closest?.('select')) return;
  event.preventDefault();
  navigateMonitor('overview');
}, true);
window.addEventListener('hashchange', () => syncMonitorNavigation(true));
window.addEventListener('popstate', () => syncMonitorNavigation(true));
syncMonitorNavigation();

window.addEventListener('edgeway:event-request',({detail})=>{if(Number.isSafeInteger(detail?.id)&&detail.id>0)openLog(detail.id);});

// Replay the already loaded list when the map script starts after the log request.
window.addEventListener('edgeway:map-ready',()=>{window.dispatchEvent(new CustomEvent('edgeway:event-records',{detail:{events:[...view.events.values()]}}));mapSeeded.clear();seedFactoryMap();});
