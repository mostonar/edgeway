"use strict";
(()=>{
  const primaryState=RobotState.primary;
  const target=document.getElementById("data-values"),camera=document.getElementById("dashboard-camera");
  const make=(tag,text,cls)=>{const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n;};
  const get=id=>document.getElementById(id);
  const formatTime=new Intl.DateTimeFormat('en-GB',{timeZone:'Asia/Seoul',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false});
  function renderPayload(event){
    let payload=event.content;
    if(typeof payload==='string'){try{payload=JSON.parse(payload);}catch{}}
    const rows=get('selected-payload');rows.replaceChildren();
    const extra=get('selected-extra');extra.hidden=true;extra.open=false;get('selected-extra-body').textContent='';
    const value=v=>v===null?'—':typeof v==='object'?JSON.stringify(v):String(v);
    const add=(label,v)=>{rows.append(make('dt',label),make('dd',value(v)));};
    add('Robot ID',event.robot_id);
    if(payload&&typeof payload==='object'&&!Array.isArray(payload)){
      const labels={state:'Received state',nav:'Navigation state',pose:'Position',goal:'Goal',remaining_m:'Remaining distance (m)',cmd_id:'Command ID',recoveries:'Recoveries',seq:'Sequence'};
      for(const [key,label] of Object.entries(labels)){
        if(!Object.hasOwn(payload,key))continue;
        const v=payload[key];
        if(key==='pose'&&v&&typeof v==='object'&&!Array.isArray(v)){
          add(label,['x','y','yaw'].filter(k=>Object.hasOwn(v,k)).map(k=>k.toUpperCase()+' '+value(v[k])).join(' · ')||value(v));
        }else add(label,v);
      }
      const other=Object.fromEntries(Object.entries(payload).filter(([k])=>k!=='robot'&&!Object.hasOwn(labels,k)));
      if(Object.keys(other).length){extra.hidden=false;get('selected-extra-body').textContent=JSON.stringify(other,null,2);}
    }else add('Content',payload===undefined?'—':payload);
  }
  window.addEventListener('edgeway:selection',({detail})=>{
    const event=detail.event,empty=get('selection-empty'),content=get('selection-content'),badge=get('selection-badge');
    empty.hidden=!!event;content.hidden=!event;badge.hidden=!event;
    if(!event){empty.textContent=detail.loading?'Loading selected record.':detail.error||'Select an event log entry to view its details.';return;}
    const display=event.display;
    get('selected-log').textContent=display.title;
    get('selected-target').textContent=display.target;
    get('selected-time').textContent=formatTime.format(new Date(event.received_at))+' Received';
    get('selected-time').dateTime=event.received_at;
    badge.textContent=display.restriction||display.kind;badge.dataset.level=display.restriction?'warning':display.level;
    const rows=get('context-fields');rows.replaceChildren();
    const hasContext=Object.keys(display.context).length>0;rows.hidden=!hasContext;
    if(hasContext)for(const [key,label] of [['inspection_target','Inspection target'],['reason','Assessment basis'],['command','Operating instruction'],['result','Applied result']]){if(display.context[key])rows.append(make('dt',label),make('dd',display.context[key]));}
    get('selected-description').textContent=(display.test?'Test data · ':'')+(display.description||'');
    get('selected-description').hidden=!display.test&&!display.description;
    renderPayload(event);
    const values=get('selected-values');values.replaceChildren();
    for(const field of event.display_values||[]){const card=make('div',undefined,'value-card'),number=make('p',field.value===null?'—':typeof field.value==='number'?new Intl.NumberFormat('en-GB',{maximumFractionDigits:6}).format(field.value):String(field.value),'value-number');number.append(make('span',field.unit,'value-unit'));card.append(make('p',field.label,'value-label'),number);if(field.issue)card.append(make('p',field.issue,'value-issue'));values.append(card);}
    values.hidden=!values.children.length;
    get('context-note').hidden=!display.note;get('context-note').textContent=display.note;
  });

  // Whole-factory overview is independent from the log dropdown and detail dialog.
  const catalogue=new Map(),records=new Map(),hydrating=new Set();
  // Keep the latest actionable incident per robot/type outside the 50-record telemetry window.
  const robotIssues=new Map(),issueOperations=new Map();
  let operationsBusy=false;
  function issueType(event){
    const type=payloadOf(event)?.event?.type;
    if(['robot_fault','traffic_restriction','judgment_deferred'].includes(type))return type;
    return {'Drive fault detected':'robot_fault','Traffic restriction detected':'traffic_restriction','Assessment deferred':'judgment_deferred'}[event.display?.title]||null;
  }
  function issueDone(event){
    const item=issueOperations.get(event.id);
    return item?.type===issueType(event)&&['Repair complete','Restriction released','Passage approved'].includes(item.status);
  }
  function currentIssue(robotId,candidate){
    // Only a currently reported inspection_wait is an actionable robot fault.
    return statusOf(candidate||{}).state==='inspection_wait'?candidate:null;
  }
  const stateNames={idle:'Idle',moving:'Moving',navigating:'Moving',carrying:'Carrying',picking:'Pickup',pickup:'Pickup',dropping:'Drop-off',dropoff:'Drop-off',stopped:'Stopped',paused:'Paused',error:'Error',failed:'Failed',arrived:'Arrived',completed:'Completed'};
  Object.assign(stateNames,{waiting:'Idle',blocked:'Path blocked',loaded:'Loaded',fault:'Robot fault',collision:'Collision'});
  Object.assign(stateNames,{anomaly_detected:'Suspected robot fault',inspection_wait:'Robot fault',cross_check:'Cross-checking'});
  Object.assign(stateNames,{reset:'Resetting',to_pick:'Moving to pickup',aborted:'Task aborted'});
  Object.assign(stateNames,{
    to_aisle:'Moving to aisle',to_drop:'Moving to drop-off',
    turn_start:'Turning to start',turn_in:'Turning into pickup',turn_out:'Turning out',
    turn_drop:'Turning at drop-off',turn_unload:'Turning to unload',
    loading:'Loading',unloading:'Unloading',done:'Task complete'
  });
  function robotCardTone(state){
    if(state==='inspection_wait')return 'fault';
    if(state==='anomaly_detected')return 'attention';
    if(['done','completed','arrived'].includes(state))return 'complete';
    if(['idle','waiting','stopped','paused'].includes(state))return 'idle';
    if(state&&Object.hasOwn(stateNames,state))return 'working';
    return 'unknown';
  }
  let overviewTimer,overviewVersion=0,robotPage=0,alertPage=0;
  function payloadOf(event){
    let v=event.content??event.map_content??event.preview;
    if(typeof v==='string'){try{v=JSON.parse(v);}catch{return null;}}
    return v&&typeof v==='object'&&!Array.isArray(v)?v:null;
  }
  function statusOf(event){
    const p=payloadOf(event),d=event.display||{};
    const state=typeof p?.state==='string'?p.state:typeof p?.nav==='string'?p.nav:d.reported_state;
    const time=p?.t_wall??d.state_time;
    return {state:primaryState(state),reportedState:typeof state==='string'?state:null,time:typeof time==='number'&&Number.isFinite(time)?time:null,
      priority:typeof p?.state==='string'?2:typeof p?.nav==='string'?1:d.state_priority||0};
  }
  function rememberOverview(events){
    for(const event of events||[]){
      if(!window.edgewayFactoryOf?.(event.robot_id)||!Number.isSafeInteger(event.id))continue;
      catalogue.set(event.robot_id,{id:event.robot_id,name:event.robot_name});
      let list=records.get(event.robot_id);if(!list){list=new Map();records.set(event.robot_id,list);}
      list.set(event.id,event);
      const kind=issueType(event),key=event.robot_id+':'+kind;
      if(kind&&(!robotIssues.has(key)||robotIssues.get(key).id<=event.id)){
        const previous=robotIssues.get(key);
        if(previous&&previous.id!==event.id)issueOperations.delete(previous.id);
        robotIssues.set(key,event);
      }
      if(list.size>50)list.delete(Math.min(...list.keys()));
    }
    clearTimeout(overviewTimer);overviewTimer=setTimeout(renderOverview,0);
  }
  function renderOverview(){
    const factory=window.edgewayFactory||'scenario-1';
    get('factory-overview-title').textContent='Factory '+({'scenario-1':'A','scenario-2':'B','scenario-3':'C'}[factory]||'')+' Overview';
    const letter={'scenario-1':'A','scenario-2':'B','scenario-3':'C'}[factory]||'A';
    const robots=Array.from({length:letter==='A'?4:2},(_,i)=>{
      const id=`site${letter}_AMR_${i+1}`,mock=id+'_mock';
      return catalogue.get(id)||(records.has(mock)?catalogue.get(mock):null)||{id,name:id};
    });
    const robotList=get('factory-robots');robotList.replaceChildren();
    const alerts=[];
    for(const robot of robots){
      const events=[...(records.get(robot.id)?.values()||[])].sort((a,b)=>b.id-a.id);
      // Only hydrate the latest status candidate when its list preview was truncated.
      const candidate=events.filter(e=>statusOf(e).state!==null||['Status','상태'].includes(e.display?.kind)).sort((a,b)=>{
        const x=statusOf(a),y=statusOf(b);
        return x.time!==null&&y.time!==null?(y.time-x.time||y.priority-x.priority||b.id-a.id):b.id-a.id;
      })[0];
      if(candidate?.truncated&&candidate.content===undefined&&!hydrating.has(candidate.id)){
        hydrating.add(candidate.id);
        fetch('/api/events/'+candidate.id,{cache:'no-store',signal:AbortSignal.timeout(8000)}).then(r=>{if(!r.ok)throw Error();return r.json();}).then(e=>rememberOverview([e])).catch(()=>hydrating.delete(candidate.id));
      }
      const statusInfo=statusOf(candidate||{}),state=statusInfo.state;
      const known=typeof state==='string'?state:null;
      const issue=currentIssue(robot.id,candidate);
      const row=make('article',undefined,'factory-robot robot-status-card');
      row.dataset.robot=robot.id;
      row.dataset.state=known||'';
      row.dataset.tone=issue?(issueType(issue)==='robot_fault'||robotCardTone(statusOf(issue).state)==='fault'?'fault':'attention'):candidate?robotCardTone(known):'empty';
      if(issue)row.dataset.issueId=issue.id;
      const name=robot.id.replace(/^site([ABC])_AMR_(\d+).*$/,'$1-$2');
      row.setAttribute('aria-label',name+' robot status');
      const head=make('div',undefined,'robot-card-heading');
      head.append(make('strong',name));
      head.append(make('span',!candidate?'No data':row.dataset.tone==='fault'?'Fault':row.dataset.tone==='attention'?'Attention':'Status','robot-card-tag'));
      row.append(head);
      const label=!candidate?'Waiting for data':known?(Object.hasOwn(stateNames,known)?stateNames[known]:'Unrecognized state'):'State not reported';
      const status=make('span',label,'robot-card-state');
      if(known)status.title='Reported state: '+statusInfo.reportedState;
      row.append(status);
      if(known&&!Object.hasOwn(stateNames,known))row.append(make('small',known,'robot-card-raw'));
      const stamp=Date.parse(candidate?.received_at||'');
      const time=make('small',!candidate?'No records received':Number.isFinite(stamp)?'Last received '+formatTime.format(new Date(stamp)):'Receive time unavailable','robot-card-time');
      const age=make('small',!candidate?'':!Number.isFinite(stamp)?'':Date.now()-stamp>5000?'Earlier data':'Recent data','robot-card-age');
      row.append(time,age);
      if(issue){
        const action=make('button',undefined,'robot-issue-open');action.type='button';
        action.dataset.eventId=issue.id;
        action.setAttribute('aria-label',name+' review issue EV-'+issue.id);
        action.append(make('span','! Review issue'),make('small',issueOperations.get(issue.id)?.status||({'robot_fault':'Robot fault reported','traffic_restriction':'Path restriction','judgment_deferred':'Review required'}[issueType(issue)]||'Check reported state')));
        action.addEventListener('click',()=>window.dispatchEvent(new CustomEvent('edgeway:event-request',{detail:{id:issue.id}})));
        row.append(action);
      }
      robotList.append(row);
      if(issue)alerts.push(issue); // Current inspection only; never resurrect historical alerts.
    }
    get('factory-counts').replaceChildren();
    const alertList=get('factory-alerts');alertList.replaceChildren();
    for(const e of alerts.sort((a,b)=>b.id-a.id)){
      const button=make('button',undefined,'factory-alert');button.type='button';button.dataset.eventId=e.id;
      button.append(make('span',e.display?.title||'Alerts'),make('small',e.robot_id.replace(/^site([ABC])_AMR_(\d+).*$/,'$1-$2')));
      const status=make('small','','factory-alert-status');button.append(status);
      button.addEventListener('click',()=>window.dispatchEvent(new CustomEvent('edgeway:event-request',{detail:{id:e.id}})));
      alertList.append(button);
    }
    alertList.parentElement.hidden=!alerts.length;
    fitOverview();
    overviewVersion++;
  }
  function fitOverview(){
    const robotList=get('factory-robots');
    for(const row of robotList.children)row.hidden=false;
    get('robots-page').parentElement.hidden=true;
    get('robots-page').textContent='';
    for(const [prefix,id,fallbackHeight] of [['alerts','factory-alerts',58]]){
      const list=get(id),items=[...list.children].filter(e=>e.classList.contains(prefix==='robots'?'factory-robot':'factory-alert'));
      const pager=get(prefix+'-page').parentElement;
      const style=items.length?getComputedStyle(items[0]):null;
      const rowHeight=style?(parseFloat(style.height)||fallbackHeight)+(parseFloat(style.marginBottom)||0):fallbackHeight;
      const gap=parseFloat(getComputedStyle(list.parentElement).rowGap)||0;
      const fullHeight=list.clientHeight+(pager.hidden?0:pager.offsetHeight+gap);
      pager.hidden=items.length<=Math.max(1,Math.floor(fullHeight/rowHeight));
      const capacity=Math.max(1,Math.floor(list.clientHeight/rowHeight)),pages=Math.max(1,Math.ceil(items.length/capacity));
      let page=prefix==='robots'?robotPage:alertPage;page=Math.min(page,pages-1);
      if(prefix==='robots')robotPage=page;else alertPage=page;
      items.forEach((e,i)=>e.hidden=i<page*capacity||i>=(page+1)*capacity);
      get(prefix+'-page').textContent=items.length?(page+1)+' / '+pages:'0 / 0';
      get(prefix+'-prev').disabled=page===0;get(prefix+'-next').disabled=page>=pages-1;
    }
  }
  for(const prefix of ['robots','alerts'])for(const [suffix,delta] of [['prev',-1],['next',1]]){
    get(prefix+'-'+suffix).addEventListener('click',()=>{if(prefix==='robots')robotPage+=delta;else alertPage+=delta;fitOverview();refreshOverviewOperations();});
  }
  new ResizeObserver(fitOverview).observe(document.querySelector('.overview-lists'));
  get('factory-values-open').onclick=()=>{get('factory-values-title').textContent=get('factory-overview-title').textContent+' · Data';get('latest-data').open=true;get('factory-values-dialog').showModal();};
  get('factory-values-close').onclick=()=>get('factory-values-dialog').close();
  async function refreshOverviewOperations(){
    if(operationsBusy||document.hidden)return;
    const factory=window.edgewayFactory;
    const rows=[...get('factory-alerts').querySelectorAll('[data-event-id]:not([hidden])')];
    const issues=[...robotIssues.values()].filter(e=>window.edgewayFactoryOf?.(e.robot_id)===factory);
    const ids=[...new Set([...rows.map(e=>Number(e.dataset.eventId)),...issues.map(e=>e.id)])];
    if(!ids.length)return;
    operationsBusy=true;
    try{
      const r=await fetch('/api/operations?ids='+ids.join(','),{cache:'no-store',signal:AbortSignal.timeout(8000)});
      if(!r.ok)return;const data=await r.json();let changed=false;
      for(const item of data.items||[]){
        if(!ids.includes(item.event_id))continue;
        const previous=issueOperations.get(item.event_id);
        if(previous?.status!==item.status||previous?.type!==item.type)changed=true;
        issueOperations.set(item.event_id,item);
      }
      if(changed)renderOverview();
      for(const row of get('factory-alerts').querySelectorAll('[data-event-id]')){
        const item=issueOperations.get(Number(row.dataset.eventId));
        if(item?.type)row.querySelector('.factory-alert-status').textContent=item.status;
      }
    }catch{}finally{operationsBusy=false;}
  }
  window.addEventListener('edgeway:catalogue',({detail})=>{for(const r of detail.robots||[])catalogue.set(r.id,r);renderOverview();});
  for(const type of ['edgeway:event-records','edgeway:events-loaded'])window.addEventListener(type,({detail})=>rememberOverview(detail.events));
  window.addEventListener('edgeway:telemetry-received',({detail})=>rememberOverview([detail.event]));
  window.addEventListener('edgeway:factory-change',()=>{robotPage=0;alertPage=0;get('factory-values-dialog').close();renderOverview();});
  setInterval(()=>{renderOverview();refreshOverviewOperations();},2000);

  let valuesGeneration=0;
  async function refresh(){
    const generation=++valuesGeneration;
    if(document.hidden)return;
    try {
      const response=await fetch("/api/bindings/values",{cache:"no-store",signal:AbortSignal.timeout(8000)});if(!response.ok)throw Error("Could not load display values.");
      const data=await response.json();if(generation!==valuesGeneration)return;target.replaceChildren();
      const visible=data.items.filter(item=>item.binding.fields.length&&window.edgewayFactoryOf?.(item.binding.robot_id)===window.edgewayFactory);
      if(!visible.length){const p=make("p","Choose received fields in Connection Settings.","hint");target.append(p);return;}
      for(const item of visible){
        const block=make("section",undefined,"binding-values");block.append(make("h3",item.binding.name+" · "+item.binding.channel));
        const state=item.binding.state==="inactive"?"Unsubscribed · Last record":item.binding.state==="active"?"Latest data":"Check connection · Last record";
        block.append(make("p",item.received?`${state} · ${new Date(item.received_at).toLocaleTimeString("en-GB",{timeZone:"Asia/Seoul"})}`:"No data received yet.","hint"));
        const grid=make("div",undefined,"value-grid");for(const field of item.values){const card=make("div",undefined,"value-card"),number=make("p",field.value===null?"—":typeof field.value==="number"?new Intl.NumberFormat("en-GB",{maximumFractionDigits:6}).format(field.value):String(field.value),"value-number");number.append(make("span",field.unit,"value-unit"));card.append(make("p",field.label,"value-label"),number);if(field.issue)card.append(make("p",field.issue,"value-issue"));grid.append(card);}block.append(grid);target.append(block);
      }
    }catch(error){if(generation===valuesGeneration)target.replaceChildren(make("p",error.message,"hint"));}
  }
  document.getElementById("dashboard-camera-start").addEventListener("click",()=>{if(camera.querySelector("iframe"))return;const frame=document.createElement("iframe");frame.src="/camera-preview?t="+Date.now();frame.title="Site camera test";frame.allow="autoplay; fullscreen";camera.replaceChildren(frame);get("camera-view-state").textContent="Viewer open";});
  document.getElementById("dashboard-camera-stop").addEventListener("click",()=>{camera.replaceChildren(make("p","Camera test stopped.","camera-placeholder"));get("camera-view-state").textContent="Viewer closed";});
  window.addEventListener('edgeway:factory-change',()=>{
    valuesGeneration++;target.replaceChildren();
    // Shared WebRTC session persists; changing factory data does not switch the camera viewpoint.
    refresh();
  });
  refresh();setInterval(refresh,2000);document.addEventListener("visibilitychange",()=>{if(!document.hidden)refresh();});
})();
