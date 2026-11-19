"use strict";
const $ = (id) => document.getElementById(id);
const channelNames = {status:"Status",telemetry:"Sensor",log:"Log",custom:"Other"};
const stateNames = {active:"Active",pending:"Checking",inactive:"Removed",failed:"Registration failed",unknown:"Needs checking"};
let csrf = "", bindings = [], selected = "", exampleMode = false, sampleReceived = false;


// Switching sections retains the existing form nodes and unsaved values.
function showSettingsSection(title, {push = true, focus = false} = {}) {
  const panes = [...document.querySelectorAll('[data-settings-pane]')];
  const current = panes.find(p => p.dataset.settingsPane === title) || panes[0];
  for (const pane of panes) pane.hidden = pane !== current;
  for (const link of document.querySelectorAll('.settings-shortcuts a')) {
    if (link.hash === '#' + current.dataset.settingsPane) link.setAttribute('aria-current', 'page');
    else link.removeAttribute('aria-current');
  }
  if (push && location.hash !== '#' + current.dataset.settingsPane) history.pushState(null, '', '#' + current.dataset.settingsPane);
  if (focus) {
    const heading = document.getElementById(current.dataset.settingsPane);
    heading.tabIndex = -1;
    heading.focus({preventScroll: true});
    current.scrollIntoView({block: 'nearest'});
  }
}
for (const link of document.querySelectorAll('.settings-shortcuts a')) {
  link.addEventListener('click', event => {
    if (event.button || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault(); showSettingsSection(link.hash.slice(1), {focus: true});
  });
}
const syncSettingsSection = () => showSettingsSection(location.hash.slice(1), {push: false});
window.addEventListener('popstate', syncSettingsSection);
window.addEventListener('hashchange', syncSettingsSection);
syncSettingsSection();

function node(tag, text, className) { const n=document.createElement(tag); if(text!==undefined)n.textContent=text; if(className)n.className=className; return n; }
function feedback(message, error=false) { const n=$("feedback");n.textContent=message;n.dataset.error=String(error);n.hidden=false; }
async function api(path, method="GET", data) {
  const options={method,cache:"no-store",signal:AbortSignal.timeout(20000)};
  if(method!=="GET") {options.headers={"Content-Type":"application/json","X-EdgeWay-CSRF":csrf};options.body=JSON.stringify(data??{});}
  const res=await fetch(path,options);const value=await res.json();if(!res.ok)throw Error(value.error||"Could not complete the request.");return value;
}
async function run(button, task) { if(button)button.disabled=true;try{await task();}catch(error){feedback(error.message,true);}finally{if(button)button.disabled=false;} }
function time(value) {return value ? new Intl.DateTimeFormat("en-GB",{timeZone:"Asia/Seoul",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit"}).format(new Date(value)) : "No data received yet";}
function action(label, task, danger=false) {const b=node("button",label,"secondary"+(danger?" danger":""));b.type="button";b.addEventListener("click",()=>run(b,task));return b;}
function profile() {return bindings.find(p=>p.id===selected);}

async function refreshConnection() {
  const label=$("cse-state");
  try {
    const status=await api("/api/status");
    label.dataset.state=status.cse.state;
    label.textContent=status.cse.state==="reachable" ? "ACME Connected" : status.cse.state==="checking" ? "ACME Checking · Stored logs available" : "ACME Disconnected · Stored logs available";
    label.title=status.cse.message;
  } catch {
    label.dataset.state="unknown";
    label.textContent="Could not check dashboard status";
  }
}

async function reload(selectId) {
  const data=await api("/api/settings");csrf=data.csrf;bindings=data.bindings;
  $("cse-url").textContent=data.connection.cse_url;$("cse-id").textContent=data.connection.cse_id;$("notify-url").textContent=data.connection.notification_url;
  $("camera-name").value=data.camera.name;$("camera-url").value=data.camera.url;
  $("camera-external").hidden=!data.camera.url;$("camera-external").href=data.camera.url||"#";
  const list=$("bindings-list");list.replaceChildren();
  if(!bindings.length)$("subscription-create").open=true;
  if(!bindings.length)list.append(node("p","No subscriptions yet. Select Add robot subscription to register one.","empty-copy"));
  for(const p of bindings) {
    const row=node("article",undefined,"binding-item"),left=node("div"),heading=node("h3",p.name+" · "+channelNames[p.channel]);
    const badge=node("span",stateNames[p.state]||p.state,"status-badge");badge.dataset.state=p.state;heading.append(badge);left.append(heading,node("p",p.robot_id,"hint"));
    const middle=node("div"),received=node("p","Last received · "+time(p.last_received_at),"hint");received.dataset.receivedFor=p.id;middle.append(node("p",p.resource_path,"mono"),received);
    if(p.error)middle.append(node("p",p.error,"binding-error hint"));
    const buttons=node("div",undefined,"button-row");
    buttons.append(action("Display Settings",async()=>{selected=p.id;$("field-binding").value=selected;await loadEditor();showSettingsSection("fields-title", {focus:true});}));
    buttons.append(action("Check status",async()=>{await api(`/api/bindings/${p.id}/check`,"POST");await reload();feedback("Remote subscription checked. Also check the last reception time.");}));
    if(p.state==="active"||p.state==="unknown")buttons.append(action("Unsubscribe",async()=>{await api(`/api/bindings/${p.id}/subscription`,"DELETE");await reload();feedback("Unsubscribed. Existing records and display settings retained.");},true));
    else buttons.append(action("Reconnect",async()=>{await api(`/api/bindings/${p.id}/subscribe`,"POST");await reload();feedback("Subscription registered again.");}));
    row.append(left,middle,buttons);list.append(row);
  }
  const before=selected;selected=selectId||selected;
  if(!bindings.some(p=>p.id===selected))selected=bindings[0]?.id||"";
  $("field-binding").replaceChildren(...(bindings.length?bindings.map(p=>new Option(`${p.name} · ${channelNames[p.channel]} (${p.resource_path})`,p.id)):[new Option("Register a subscription first","")]));
  $("field-binding").value=selected;$("field-editor").hidden=!selected;$("field-empty").hidden=!!selected;
  if(selected&&(before!==selected||selectId))await loadEditor();
}

function collectFields() {
  return [...$("field-rows").children].map(row=>({path:row.querySelector('[data-key="path"]').value.trim(),label:row.querySelector('[data-key="label"]').value.trim(),unit:row.querySelector('[data-key="unit"]').value,scale:row.querySelector('[data-key="scale"]').value===""?null:Number(row.querySelector('[data-key="scale"]').value)}));
}
function dirty() {$("mapping-state").textContent="Save your changes";}
function addField(field={path:"",label:"",unit:"",scale:1}) {
  if($("field-rows").children.length>=16)throw Error("Maximum 16 display fields.");
  const row=node("div",undefined,"field-row");
  for(const [key,label] of [["path","Data path"],["label","Display name"],["unit","Unit"],["scale","Scale"]]) {
    const wrap=node("label",label),input=document.createElement("input");input.dataset.key=key;input.value=field[key]??"";
    input.type=key==="scale"?"number":"text";if(key==="scale")input.step="any";
    input.maxLength=key==="path"?200:key==="label"?60:20;input.addEventListener("input",dirty);wrap.append(input);row.append(wrap);
  }
  const remove=node("button","×","remove-field");remove.type="button";remove.setAttribute("aria-label","Delete this display field");remove.addEventListener("click",()=>{row.remove();dirty();});row.append(remove);$("field-rows").append(row);
}
function renderValues(values) {
  const target=$("field-preview");target.replaceChildren();
  if(!values.length){target.append(node("p","Add fields to display.","hint"));return;}
  for(const value of values) {
    const card=node("div",undefined,"value-card"),number=node("p",value.value===null?"—":typeof value.value==="number"?new Intl.NumberFormat("en-GB",{maximumFractionDigits:6}).format(value.value):String(value.value),"value-number");
    number.append(node("span",value.unit,"value-unit"));card.append(node("p",value.label,"value-label"),number,node("p",value.path+" × "+value.scale,"value-path"));if(value.issue)card.append(node("p",value.issue,"value-issue"));target.append(card);
  }
}
async function preview() {
  const content=JSON.parse($("sample-json").value);
  const result=await api(`/api/bindings/${selected}/preview`,"POST",{content,fields:collectFields()});renderValues(result.values);
  $("preview-source").textContent=exampleMode?"Manual sample · Not transmitted":sampleReceived?"Based on latest raw data":"Waiting for data · Using sample";
}
async function loadSample() {
  const sample=await api(`/api/bindings/${selected}/sample`);sampleReceived=sample.received;
  const receipt=document.querySelector(`[data-received-for="${selected}"]`);if(receipt)receipt.textContent="Last received · "+time(sample.received_at);
  $("sample-info").textContent=sample.received?"Recent data · "+time(sample.received_at)+" · "+sample.cin_ri:"No data received yet. Test display settings with the sample.";
  const example={robot:"forklift_b",load_kg:250,pose:{x:[1.2,1.25],y:[2.1,2.15]},imu:{ax:[0.1,0.2]},state:"moving",remaining_m:12.5};
  $("sample-json").value=JSON.stringify(sample.received?sample.content:example,null,2);exampleMode=!sample.received;
  $("available-paths").replaceChildren(...(sample.paths.length?sample.paths.map(p=>new Option(p,p)):[new Option("No received fields · Add fields manually","")]));
}
async function loadEditor() {
  const p=profile();if(!p)return;$("field-editor").hidden=false;$("field-empty").hidden=true;
  $("field-rows").replaceChildren();p.fields.forEach(addField);await loadSample();await preview();$("mapping-state").textContent="Saved settings";
}

$("binding-form").addEventListener("submit",event=>{event.preventDefault();run(event.submitter,async()=>{const p=await api("/api/bindings","POST",Object.fromEntries(new FormData(event.target)));await reload(p.id);feedback("Subscribed. Last reception time will appear when data arrives.");});});
$("check-connection").addEventListener("click",event=>run(event.target,async()=>feedback((await api("/api/connection/check","POST")).message)));
$("refresh-bindings").addEventListener("click",event=>run(event.target,async()=>{await reload();feedback("Subscription list refreshed.");}));
$("field-binding").addEventListener("change",event=>run(null,async()=>{selected=event.target.value;await loadEditor();}));
$("reload-sample").addEventListener("click",event=>run(event.target,async()=>{await loadSample();await preview();}));
$("sample-json").addEventListener("input",()=>{exampleMode=true;$("preview-source").textContent="Sample changed. Select Preview.";});
$("add-field").addEventListener("click",event=>run(event.target,async()=>{addField();dirty();}));
$("add-selected").addEventListener("click",event=>run(event.target,async()=>{const path=$("available-paths").value;if(!path)throw Error("Fetch the latest data or add fields manually.");addField({path,label:path,unit:"",scale:1});dirty();}));
$("preset-fields").addEventListener("click",event=>run(event.target,async()=>{
  const fields=profile()?.channel==="telemetry"?[{path:"load_kg",label:"Load weight",unit:"kg",scale:1},{path:"pose.x[-1]",label:"Current X",unit:"m",scale:1},{path:"imu.ax[-1]",label:"X Acceleration",unit:"m/s²",scale:1}]:[{path:"state",label:"Motion state",unit:"",scale:1},{path:"remaining_m",label:"Remaining distance",unit:"m",scale:1}];
  $("field-rows").replaceChildren();fields.forEach(addField);dirty();await preview();
}));
$("preview-fields").addEventListener("click",event=>run(event.target,preview));
$("save-fields").addEventListener("click",event=>run(event.target,async()=>{const fields=collectFields();await api(`/api/bindings/${selected}/fields`,"PUT",{fields});profile().fields=fields;await preview();$("mapping-state").textContent="Saved · Applied to dashboard";feedback("Display settings saved. Raw data unchanged.");}));
$("camera-form").addEventListener("submit",event=>{event.preventDefault();run(event.submitter,async()=>{const camera=await api("/api/camera","PUT",{name:$("camera-name").value,url:$("camera-url").value});$("camera-external").hidden=!camera.url;$("camera-external").href=camera.url||"#";$("camera-test-container").replaceChildren();$("camera-test-container").hidden=true;feedback("Camera URL saved. Use Test saved URL to check the video.");});});
$("camera-test").addEventListener("click",event=>run(event.target,async()=>{const saved=await api("/api/camera");if(!saved.url)throw Error("Save a web viewer URL first.");const frame=document.createElement("iframe");frame.src="/camera-preview?t="+Date.now();frame.title="Camera web viewer test";frame.allow="autoplay; fullscreen";$("camera-test-container").replaceChildren(frame);$("camera-test-container").hidden=false;}));
$("camera-stop").addEventListener("click",()=>{$("camera-test-container").replaceChildren();$("camera-test-container").hidden=true;});
run(null,()=>reload());
refreshConnection();
setInterval(()=>{if(!document.hidden)refreshConnection();},5000);
setInterval(async()=>{
  if(document.hidden)return;
  try{const data=await api('/api/bindings/values');for(const item of data.items){const receipt=document.querySelector(`[data-received-for="${item.binding.id}"]`);if(receipt)receipt.textContent="Last received · "+time(item.received_at);}}catch{/* Explicit refresh still reports connection errors. */}
},5000);
