/* Latest received telemetry only; no robot commands. Pure geometry helpers also run in Node. */
const FloorplanData = (() => {
  const primaryState=typeof RobotState!=='undefined'?RobotState.primary:require('./robot-state.js').primary;
  const finite = v => typeof v === 'number' && Number.isFinite(v);
  const valid = p => finite(p[1]) && finite(p[2]);
  const factoryMaps={
    'scenario-1':{image:'siteA.png',width:631,height:1075,resolution:0.05,origin:[0,0,0]},
    'scenario-2':{image:'siteB.png',width:390,height:600,resolution:0.05,origin:[0,0,0]},
    // Bundled maps are synthetic examples with their own YAML calibration.
    'scenario-3':{image:'siteC.png',width:390,height:600,resolution:0.05,origin:[0,0,0]}
  };
  // Find the white navigable area, retaining two pixels for its surrounding wall.
  // The PNG/YAML and world coordinates stay unchanged; this is a display window.
  function mapContentBounds(data,width,height){
    const full={x:0,y:0,w:width,h:height};
    let x0=width,y0=height,x1=-1,y1=-1,count=0;
    for(let y=0;y<height;y++)for(let x=0;x<width;x++){
      const i=(y*width+x)*4;
      if(data[i+3]>=250&&data[i]>=240&&data[i+1]>=240&&data[i+2]>=240){
        x0=Math.min(x0,x);x1=Math.max(x1,x);y0=Math.min(y0,y);y1=Math.max(y1,y);count++;
      }
    }
    if(count<16||x1-x0<3||y1-y0<3)return full;
    x0=Math.max(0,x0-2);y0=Math.max(0,y0-2);
    x1=Math.min(width,x1+3);y1=Math.min(height,y1+3);
    return {x:x0,y:y0,w:x1-x0,h:y1-y0};
  }
  function mapCorners(map){
    const b=map.contentBounds||{x:0,y:0,w:map.image.width,h:map.image.height};
    return [[b.x,b.y],[b.x+b.w,b.y],[b.x+b.w,b.y+b.h],[b.x,b.y+b.h]]
      .map(([u,v])=>mapPoint(u,v,map.image.height,map));
  }
  function factoryScenario(content,robot){
    const site=/^site([ABC])_/.exec(robot||'')?.[1];
    return content?.scenario_id??(site?`scenario-${'ABC'.indexOf(site)+1}`:'scenario-1');
  }
  function normalize(input) {
    const source = input?.schema === 'edgeway.recorded-poses.v1' ? input.runs : Array.isArray(input) ? input : [input];
    if (!Array.isArray(source) || !source.length || source.length > 1000) throw Error('Expected 1 to 1,000 trials.');
    let total = 0;
    const runs = source.map((r, n) => {
      if (!r || typeof r !== 'object') throw Error('Invalid trial format.');
      let points = [];
      if (input?.schema === 'edgeway.recorded-poses.v1') {
        if (!Array.isArray(r.points)) throw Error('No coordinate arrays.');
        points = r.points;
      } else {
        if (!Array.isArray(r.cins) || !r.cins.length) throw Error('Select trial JSON containing a CIN list.');
        for (const cin of r.cins) {
          const d = typeof cin.con === 'string' ? JSON.parse(cin.con) : cin.con;
          if (!Array.isArray(d?.t_sim) || !['x','y','yaw'].every(k => Array.isArray(d?.pose?.[k]) && d.pose[k].length === d.t_sim.length)) throw Error('Time and position arrays have different lengths.');
          for (let i=0;i<d.t_sim.length;i++) points.push([d.t_sim[i],d.pose.x[i],d.pose.y[i],d.pose.yaw[i]]);
        }
      }
      total += points.length;
      if (!points.length || total > 500000) throw Error('Empty trial or more than 500,000 samples.');
      let previous = -Infinity, distance = 0, validCount = 0, last = null;
      const breaks = [], normalized = points.map((p,i) => {
        if (!Array.isArray(p) || p.length !== 4 || !finite(p[0]) || Math.abs(p[0]) > 1e15 || p[0] < previous) throw Error('Invalid or decreasing time within a trial.');
        if(p.slice(1).some(v=>finite(v)&&Math.abs(v)>1e9))throw Error('Coordinates exceed the supported range.');
        const q = [p[0], ...p.slice(1).map(v => finite(v) ? v : null)];
        const gap = i > 0 && q[0] - previous > .25;
        if (gap) breaks.push(i);
        if (valid(q)) {validCount++; if(last && !gap) distance += Math.hypot(q[1]-last[1],q[2]-last[2]); last=q;} else last=null;
        previous=q[0];return q;
      });
      // Raw source labels use cond:/load:/spd:/scn: when available; unknown stays unknown.
      const labels = Array.isArray(r.lbl) ? r.lbl : [];
      const label = key => {const v=labels.find(v=>typeof v==='string' && v.startsWith(key+':'));return v?.slice(key.length+1);};
      const str = (v,fallback) => typeof v==='string' && v ? v.slice(0,180) : fallback;
      return {id:str(r.id || r.run_id,`import-${n+1}`),robot:str(r.robot || r.ae,'Unknown'),scenarioId:r.scenarioId||r.scenario_id||null,receivedAt:r.receivedAt||'',source:r.source||'',eventId:r.eventId||0,robotState:typeof r.robotState==='string'?r.robotState:null,robotStateAt:r.robotStateAt||'',
        cond:str(r.cond || label('cond'),'Unclassified'),load:str(r.load || label('load'),'Unclassified'),spd:str(r.spd || label('spd'),'Unclassified'),scn:str(r.scn || label('scn'),'Unclassified'),
        order:Number.isInteger(r.order) ? r.order : n+1,points:normalized,breaks,valid_xy:validCount,distance,
        source_file:str(r.source_file,''),duration:normalized.at(-1)[0]-normalized[0][0]};
    });
    return runs;
  }
  function yaml(text) {
    if (text.length > 65536) throw Error('Map YAML is too large.');
    const lines = text.split(/\r?\n/).map(x=>x.replace(/\s+#.*$/,'').trim());
    const get = key => {const matches=lines.filter(l=>l.startsWith(key+':'));if(matches.length!==1)throw Error(`${key} requires one value.`);return matches[0].slice(key.length+1).trim();};
    const resolution=Number(get('resolution')), raw=get('origin');
    if (!/^\[[^\]]+\]$/.test(raw)) throw Error('origin must use the format [x, y, yaw].');
    const entries=raw.slice(1,-1).split(',').map(s=>s.trim());
    if(entries.some(s=>!s))throw Error('Origin value is empty.');
    const origin=entries.map(Number);
    if(!finite(resolution)||resolution<=0||resolution>1e6||origin.length!==3||!origin.every(v=>finite(v)&&Math.abs(v)<=1e9))throw Error('Invalid map scale or origin.');
    return {resolution,origin};
  }
  function mapPoint(u,v,height,meta) {
    const [x,y,a]=meta.origin, r=meta.resolution;
    return [x+r*(u*Math.cos(a)-(height-v)*Math.sin(a)),y+r*(u*Math.sin(a)+(height-v)*Math.cos(a))];
  }
  function csv(run) {
    const cell=v=>{const s=String(v??'');return '"'+s.replaceAll('"','""')+'"';};
    // Formula-like identifiers are prefixed for spreadsheet safety.
    const safe=s=>/^[=+\-@\t\r]/.test(s)?"'"+s:s;
    return '\uFEFFrun_id,robot,t_sim,x,y,yaw,position_valid\r\n'+run.points.map(p=>[cell(safe(run.id)),cell(safe(run.robot)),...p.map(v=>v===null?'':v),valid(p)].join(',')).join('\r\n');
  }
  function linkEvent(event,runs,currentId) {
    const robotId=typeof event?.robot_id==='string'?event.robot_id:'';
    let content=event?.content;
    try {if(typeof content==='string')content=JSON.parse(content);}catch {content=null;}
    const runId=content?.run_id ?? content?.event?.run_id;
    const rawTime=content?.t_sim ?? content?.event?.t_sim;
    const t=Array.isArray(rawTime)?rawTime.at(-1):rawTime;
    const candidates=runs.map((r,i)=>({r,i})).filter(({r})=>r.robot===robotId&&(!r.scenarioId||r.scenarioId===(factoryScenario(content,event.robot_id))));
    if(!candidates.length)return {robotId,runIndex:-1,timeIndex:-1,reason:'No position records'};
    const exact=typeof runId==='string'?candidates.filter(({r})=>r.id===runId):[];
    if(typeof runId==='string'&&exact.length!==1)return {robotId,runIndex:-1,timeIndex:-1,reason:exact.length?'Trial ID duplicate · Cannot link timestamp':'No matching trial records'};
    const chosen=exact[0]||candidates.find(({r})=>r.id===currentId)||candidates[0];
    if(!exact.length||!finite(t))return {robotId,runIndex:chosen.i,timeIndex:-1,reason:'Robot linked · Event time unknown'};
    const pts=chosen.r.points;
    if(t<pts[0][0]||t>pts.at(-1)[0])return {robotId,runIndex:chosen.i,timeIndex:-1,reason:'Event time outside recorded range'};
    let nearest=0;
    for(let i=1;i<pts.length;i++)if(Math.abs(pts[i][0]-t)<Math.abs(pts[nearest][0]-t))nearest=i;
    if(Math.abs(pts[nearest][0]-t)>.25)return {robotId,runIndex:chosen.i,timeIndex:-1,reason:'No sample at event time'};
    const delta=Math.abs(pts[nearest][0]-t);
    const timeLabel=delta<1e-6?'Event time linked':`Nearest sample linked · ${delta.toFixed(3)} s difference`;
    return {robotId,runIndex:chosen.i,timeIndex:nearest,reason:timeLabel+(valid(pts[nearest])?'':' · No position received')};
  }
  function motionIndex(run) {
    let first=-1,last=-1;
    const previous=run.points.map((p,i)=>{if(valid(p)){last=i;if(first<0)first=i;}return last;});
    return {first,previous};
  }
  function displayPose(run,index,time,history) {
    const p=run.points[index],next=run.points[index+1];
    if(!p)return {state:'unavailable',point:null,age:0};
    if(!valid(p)){
      const last=history.previous[index];
      if(last>=0)return {state:'held',point:run.points[last],age:Math.max(0,time-run.points[last][0])};
      return history.first>=0?{state:'preview',point:run.points[history.first],age:0}:{state:'unavailable',point:null,age:0};
    }
    const dt=next?next[0]-p[0]:0;
    if(next&&valid(next)&&dt>0&&dt<=.25&&time>p[0]&&time<next[0]){
      const f=(time-p[0])/dt;
      return {state:'interpolated',point:[time,p[1]+(next[1]-p[1])*f,p[2]+(next[2]-p[2])*f,p[3]],age:0};
    }
    if(next&&dt>.25&&time-p[0]>.25)return {state:'held',point:p,age:time-p[0]};
    return {state:'observed',point:p,age:0};
  }
  function sceneBounds(runs) {
    let x0=Infinity,x1=-Infinity,y0=Infinity,y1=-Infinity;
    for(const r of runs)for(const p of r.points)if(valid(p)){x0=Math.min(x0,p[1]);x1=Math.max(x1,p[1]);y0=Math.min(y0,p[2]);y1=Math.max(y1,p[2]);}
    if(!Number.isFinite(x0))return {x:0,y:0,extent:2};
    return {x:(x0+x1)/2,y:(y0+y1)/2,extent:Math.max(2,x1-x0,y1-y0)};
  }
  function trailSegments(run,index,time,seconds=Infinity) {
    const segments=[];let segment=null,previous=null;
    const cutoff=time-seconds;
    for(let i=0;i<=index;i++){
      const p=run.points[i];
      if(!valid(p)){previous=null;segment=null;continue;}
      const connected=previous&&p[0]-previous[0]<=.25;
      if(p[0]>=cutoff){
        if(!connected||!segment){segment=[];segments.push(segment);
          if(connected&&previous[0]<cutoff&&p[0]>previous[0]){
            const f=(cutoff-previous[0])/(p[0]-previous[0]);segment.push([cutoff,previous[1]+(p[1]-previous[1])*f,previous[2]+(p[2]-previous[2])*f]);
          }
        }
        segment.push(p);
      }
      previous=p;
    }
    return segments;
  }
  function movementAngle(run,index,display) {
    if(!['observed','interpolated'].includes(display.state))return null;
    const b=display.state==='interpolated'?run.points[index+1]:run.points[index];
    const a=display.state==='interpolated'?run.points[index]:run.points[index-1];
    if(!a||!b||!valid(a)||!valid(b)||b[0]<=a[0]||b[0]-a[0]>.25)return null;
    const dx=b[1]-a[1],dy=b[2]-a[2];
    return Math.hypot(dx,dy)<.002?null:Math.atan2(dy,dx);
  }
  function predictedPath(run,index,display) {
    // Constant-velocity display estimate from past samples only; never a navigation plan.
    if(!['observed','interpolated'].includes(display.state)||!display.point)return null;
    const latest=run.points[index];if(!latest||!valid(latest))return null;
    let oldest=latest,recent=latest;
    for(let i=index-1;i>=0;i--){
      const p=run.points[i],next=run.points[i+1];
      if(!valid(p)||next[0]-p[0]>.25||next[0]<=p[0]||latest[0]-p[0]>.5)break;
      oldest=p;if(recent===latest||latest[0]-p[0]<=.12)recent=p;
    }
    const dt=latest[0]-oldest[0];
    if(dt<.1||Math.hypot(latest[1]-recent[1],latest[2]-recent[2])<.002)return null;
    const vx=(latest[1]-oldest[1])/dt,vy=(latest[2]-oldest[2])/dt;
    if(Math.hypot(vx,vy)<.01)return null;
    const start=[display.point[1],display.point[2]],seconds=3;
    return {start,end:[start[0]+vx*seconds,start[1]+vy*seconds],seconds};
  }
  function smoothArrow(previous,target,dt) {
    if(dt===Infinity)return target?{...target,opacity:1}:null;
    if(!previous&&!target)return null;
    const old=previous||{...target,opacity:0},next=target||old;
    const turn=1-Math.exp(-Math.max(0,dt)/.16),fade=1-Math.exp(-Math.max(0,dt)/.10);
    const delta=Math.atan2(Math.sin(next.angle-old.angle),Math.cos(next.angle-old.angle));
    const result={angle:old.angle+delta*turn,length:old.length+(next.length-old.length)*turn,
      opacity:old.opacity+((target?1:0)-old.opacity)*fade};
    return !target&&result.opacity<.015?null:result;
  }
  function indexAt(run,time){
    let lo=0,hi=run.points.length;
    while(lo<hi){const mid=(lo+hi)>>>1;if(run.points[mid][0]<=time+1e-9)lo=mid+1;else hi=mid;}
    return Math.max(0,lo-1);
  }
  function missingIntervals(run){
    const result=[];
    for(let i=0;i<run.points.length-1;i++){
      const p=run.points[i],q=run.points[i+1];
      const start=!valid(p)?p[0]:q[0]-p[0]>.25?p[0]+.25:null;
      if(start===null||q[0]<=start)continue;
      const last=result.at(-1);if(last&&last[1]>=start-1e-9)last[1]=q[0];else result.push([start,q[0]]);
    }
    return result;
  }
  function followView(view,point,scale,width,height,dt){
    const dx=point[0]-view.x,dy=point[1]-view.y;
    const excess=(d,limit)=>Math.sign(d)*Math.max(0,Math.abs(d)-limit);
    const ex=excess(dx,Math.min(60,width*.10)/scale),ey=excess(dy,Math.min(40,height*.10)/scale);
    const settled=Math.hypot(ex,ey)*scale<.1,alpha=settled?1:1-Math.exp(-Math.max(0,dt)/.18);
    return {x:view.x+ex*alpha,y:view.y+ey*alpha,settled};
  }
  function eventLocation(event,runs){
    if(!Number.isSafeInteger(event?.id)||event.id<=0)return null;
    let content=event.content;try{if(typeof content==='string')content=JSON.parse(content);}catch{return null;}
    if(content?.pose&&!content.event&&!content.event_type)return null; // Telemetry samples are not incident pins.
    let runId=content?.run_id??content?.event?.run_id;
    if(!runId){try{const raw=typeof event.notification_raw==='string'?JSON.parse(event.notification_raw):null,cin=raw?.['m2m:sgn']?.nev?.rep?.['m2m:cin']||raw?.['m2m:cin'];runId=cin?.lbl?.find(v=>typeof v==='string'&&v.startsWith('run:'))?.slice(4);}catch{}}
    if(!runId)runId=(Array.isArray(event.map_labels)?event.map_labels.find(v=>typeof v==='string'&&v.startsWith('run:'))?.slice(4):null)||`live:${event.robot_id}`;
    const rawTime=content?.event?.t_sim??content?.t_sim;
    const time=Array.isArray(rawTime)?rawTime.at(-1):rawTime;
    const candidates=runs.filter(r=>r.robot===event.robot_id&&r.id===runId&&(!r.source||r.source===event.source)&&(!r.scenarioId||r.scenarioId===(factoryScenario(content,event.robot_id))));
    if(candidates.length!==1||!finite(time))return null;
    const run=candidates[0],i=indexAt(run,time),recorded=run.points[i],pose=content?.event?.pose??content?.pose;
    if(pose&&(!finite(pose.x)||!finite(pose.y)))return null;
    const point=pose&&finite(pose.x)&&finite(pose.y)?[time,pose.x,pose.y]:recorded;
    if(!point||Math.abs(point[0]-time)>1e-6||!valid(point)||point.slice(1).some(v=>Math.abs(v)>1e9))return null;
    return {id:event.id,runId:run.id,robotId:run.robot,source:run.source||'',scenarioId:run.scenarioId||null,time,x:point[1],y:point[2],kind:incidentKind(content),title:String(event.display?.title||'Event').slice(0,100)};
  }
  function incidentKind(content){
    const type=content?.event?.type??content?.event_type;
    const types={collision:'collision',robot_fault:'fault',robot_error:'fault',object_pickup:'pickup',pickup:'pickup',object_dropoff:'dropoff',dropoff:'dropoff',object_carried:'carrying',obstacle_wait:'blocked'};
    if(type==='object_interaction')return ['pickup','dropoff'].includes(content.event?.action)?content.event.action:null;
    return Object.hasOwn(types,type)?types[type]:null;
  }
  function incidentVisible(marker,time){return marker.time===null||marker.time<=time+1e-6;}
  function receivedCoordinates(event){
    let d=event?.content;try{if(typeof d==='string')d=JSON.parse(d);}catch{return null;}
    if(!d||typeof d!=='object'||!d.pose)return null;
    const robot=event.robot_id;if(typeof robot!=='string'||!robot||robot.length>180)throw Error('Received robot ID is missing.');
    if(typeof d.robot==='string'&&d.robot!==robot)throw Error('Payload robot differs from the registered robot.');
    let lbl=[],cinId=null;try{const raw=typeof event.notification_raw==='string'?JSON.parse(event.notification_raw):null,cin=raw?.['m2m:sgn']?.nev?.rep?.['m2m:cin']||raw?.['m2m:cin'];lbl=cin?.lbl||[];cinId=cin?.ri||null;}catch{}
    if(!lbl.length&&Array.isArray(event.map_labels))lbl=event.map_labels;
    const label=key=>Array.isArray(lbl)?lbl.find(x=>typeof x==='string'&&x.startsWith(key+':'))?.slice(key.length+1):null;
    const suppliedId=d.run_id||label('run');
    const implicitRun=!suppliedId;
    const id=suppliedId||`live:${robot}`;
    if(typeof id!=='string'||id.length>180)throw Error('Invalid run_id (trial ID).');
    const batch=Array.isArray(d.t_sim),times=batch?d.t_sim:[d.t_sim];
    if(!times.length||times.length>2000)throw Error('Expected 1 to 2,000 coordinates per message.');
    const axes={};for(const k of ['x','y','yaw']){axes[k]=batch?(d.pose[k]??(k==='yaw'?times.map(()=>null):null)):[d.pose[k]??null];if(!Array.isArray(axes[k])||axes[k].length!==times.length)throw Error('Time and coordinate arrays have different lengths.');}
    let previous=-Infinity;
    const points=times.map((t,i)=>{if(!finite(t)||Math.abs(t)>1e15||t<previous)throw Error('Invalid received time.');previous=t;
      const p=[t,...['x','y','yaw'].map(k=>axes[k][i])];if(p.slice(1).some(v=>v!==null&&(!finite(v)||Math.abs(v)>1e9)))throw Error('Received coordinates must be numbers or null.');return p;});
    const scenarioId=factoryScenario(d,robot);if(!['scenario-1','scenario-2','scenario-3'].includes(scenarioId))throw Error('scenario_id must be scenario-1, scenario-2 or scenario-3.');
    const state=typeof d.state==='string'?d.state:typeof d.nav==='string'?d.nav:null;
    return {id,robot,scenarioId,points,implicitRun,isBatch:batch,cinId,observedAt:finite(d.t_wall)?d.t_wall:null,
      coordinateStream:batch?'telemetry':d.event||d.event_type?'event':'status',
      statePriority:typeof d.state==='string'?2:typeof d.nav==='string'?1:0,
      robotState:state?.slice(0,80)??null,robotStateAt:event.received_at||'',keepRobotState:!!(d.event||d.event_type)&&state===null,cond:label('cond')||'Received',load:label('load')||'Unclassified',spd:label('spd')||'Unclassified',scn:label('scn')||'Received coordinates',order:1,receivedAt:event.received_at||'',source:event.source||''};
  }
  // Independent of the latest-position/status selection: status must not erase telemetry.
  function rollingTrail(previous,incoming,eventId){
    const stamp=incoming.observedAt,key=incoming.cinId||String(eventId),end=incoming.points.at(-1)[0];
    const tracked=!!incoming.liveOrder;
    if(tracked&&finite(stamp)&&finite(incoming.liveOrder.epochStamp)&&stamp<incoming.liveOrder.epochStamp)return previous;
    const newer=!previous||(finite(stamp)&&finite(previous.stamp)?stamp>previous.stamp:eventId>previous.eventId);
    if(previous?.seen.includes(key))return previous;
    // A status read can arrive before an older, full telemetry batch from the other CNT.
    // Accept that batch inside this run's retained window; pose ordering is separate.
    if(previous&&finite(stamp)&&finite(previous.stamp)&&stamp<previous.stamp&&
       (!incoming.isBatch||end<=previous.end-20||finite(previous.epochStamp)&&stamp<previous.epochStamp))return previous;
    if(previous&&!finite(stamp)&&eventId<previous.eventId)return previous;
    const reset=previous&&(tracked?incoming.liveOrder.epochId!==previous.epochId:
      newer&&(end<previous.end||primaryState(incoming.robotState)==='reset'&&primaryState(previous.state)!=='reset'));
    if(previous?.batch&&!incoming.isBatch&&!reset)return {...previous,state:incoming.robotState};
    let points=reset?[]:(previous?.points||[]);
    let expires=reset?[]:(previous?.expires||points.map(()=>Infinity));
    let batch=reset?false:!!previous?.batch;
    const retainedEnd=incoming.isBatch&&!batch?-Infinity:previous?.end??-Infinity;
    // Once array telemetry exists, scalar status updates only affect reset detection.
    if(incoming.isBatch){if(!batch){points=[];expires=[];}batch=true;}
    if(incoming.isBatch||!batch){
      const received=Date.parse(incoming.receivedAt),now=Date.now(),base=Number.isFinite(received)?Math.min(received,now):now;
      const deadlines=incoming.points.map(p=>base+20000-Math.max(0,end-p[0])*1000);
      const latest=Math.max(end,reset?-Infinity:retainedEnd,...points.map(p=>p[0]));
      if(!points.length||incoming.points[0][0]>points.at(-1)[0]){
        // Ordered batches append directly; only late/overlapping batches need sorting.
        points=points.concat(incoming.points);expires=expires.concat(deadlines);
      }else{
        const byTime=new Map(points.map((p,i)=>[p[0],{point:p,expires:expires[i]}]));
        incoming.points.forEach((p,i)=>byTime.set(p[0],{point:p,expires:deadlines[i]}));
        const sorted=[...byTime.values()].sort((a,b)=>a.point[0]-b.point[0]);
        points=sorted.map(v=>v.point);expires=sorted.map(v=>v.expires);
      }
      const cutoff=points.findIndex(p=>p[0]>latest-20+1e-9),first=Math.max(cutoff<0?points.length:cutoff,points.length-1200);
      points=points.slice(first);expires=expires.slice(first);
    }
    return {points,expires,batch,end:Math.max(end,reset?-Infinity:retainedEnd),
      stamp:finite(stamp)?Math.max(stamp,previous?.stamp??-Infinity):previous?.stamp??null,
      epochStamp:tracked?incoming.liveOrder.epochStamp:reset?stamp:previous?.epochStamp??null,
      epochId:incoming.liveOrder?.epochId,eventId:Math.max(eventId,previous?.eventId||0),
      state:incoming.robotState,seen:[...(previous?.seen||[]),key].slice(-100)};
  }
  function expireTrail(trail,now=Date.now()){
    if(!trail?.expires?.some(t=>t<=now))return trail;
    const keep=trail.points.map((_,i)=>i).filter(i=>trail.expires[i]>now);
    return {...trail,points:keep.map(i=>trail.points[i]),expires:keep.map(i=>trail.expires[i])};
  }
  function robotColor(robot,source=''){
    const palette=['#008b89','#6d5bc5','#be6b13','#2975bc','#b94977','#54842b','#c24736','#966c00'];
    const m=/^site([ABC])_AMR_([1-4])(?:_mock)?$/.exec(robot);
    if(m&&(m[1]==='A'||+m[2]<=2))return palette[({A:0,B:4,C:6})[m[1]]+Number(m[2])-1];
    let hash=0;for(const c of source+'|'+robot)hash=(hash*31+c.charCodeAt(0))>>>0;
    return `hsl(${hash%360} 65% 38%)`;
  }
  function mergeReceived(previous,incoming,eventId){
    if(incoming.implicitRun){
      // A batch's wall timestamp can precede a status while its LAST pose is newer.
      // Order positions by simulation time; order state independently by wall time.
      const order=previous?.liveOrder,stream=incoming.coordinateStream||(incoming.isBatch?'telemetry':'status');
      const current={stamp:incoming.observedAt,eventId,priority:incoming.statePriority||0,end:incoming.points.at(-1)[0]};
      const compare=(a,b)=>!b?1:finite(a.stamp)&&finite(b.stamp)&&a.stamp!==b.stamp?a.stamp-b.stamp:a.eventId-b.eventId;
      const compareState=(a,b)=>!b?1:finite(a.stamp)&&finite(b.stamp)&&a.stamp===b.stamp?
        a.priority-b.priority||a.eventId-b.eventId:compare(a,b);
      const priorStream=order?.streams[stream];
      if(order&&finite(current.stamp)&&finite(order.epochStamp)&&current.stamp<order.epochStamp)return previous;
      if(order&&eventId<order.epochId)return previous;
      if(priorStream&&compare(current,priorStream)<=0)return previous;
      // A late batch may advance the pose only when it overlaps the current time.
      // A distant high t_sim from an older run must not revive that run on reload.
      const lastTime=previous?.points.at(-1)[0];
      const latestStamp=Math.max(...Object.values(order?.streams||{}).map(s=>finite(s.stamp)?s.stamp:-Infinity));
      if(previous&&finite(current.stamp)&&current.stamp<latestStamp&&current.end>lastTime&&
         (!incoming.isBatch||incoming.points[0][0]>lastTime))return previous;
      // Cross-CNT skew is not a restart. Detect time rollback within the SAME stream,
      // or an explicit newer reset state, and fence off delayed pre-reset messages.
      const reset=!!previous&&(stream!=='event'&&priorStream&&current.end<priorStream.end||
        primaryState(incoming.robotState)==='reset'&&primaryState(previous.robotState)!=='reset'&&compareState(current,order?.state)>0);
      const replacePose=!previous||reset||current.end>previous.points.at(-1)[0]||
        current.end===previous.points.at(-1)[0]&&compareState(current,order?.pose)>0;
      const replaceState=!incoming.keepRobotState&&(!previous||reset||compareState(current,order?.state)>0);
      const stateSource=replaceState?incoming:reset?null:previous;
      const samples=replacePose?incoming.points.map(point=>({point,eventId,receivedAt:incoming.receivedAt,
        robotState:stateSource?.robotState??null,robotStateAt:stateSource?.robotStateAt||''})):previous.samples;
      // Preserve a last known point for null-only messages, without substituting (0,0).
      const held=replacePose&&!reset&&previous?.samples?.findLast(s=>valid(s.point));
      if(held&&!samples.some(s=>valid(s.point))&&held.point[0]<samples[0].point[0])samples.unshift(held);
      return {...(replacePose?incoming:previous),eventId:Math.max(eventId,previous?.eventId||0),samples,
        points:replacePose?samples.map(s=>s.point):previous.points,
        robotState:stateSource?.robotState??null,robotStateAt:stateSource?.robotStateAt||'',
        liveOrder:{streams:{...(reset?{}:order?.streams),[stream]:current},
          epochId:reset?eventId:order?.epochId??0,epochStamp:reset?current.stamp:order?.epochStamp??null,
          pose:replacePose?current:order.pose,state:replaceState?current:reset?null:order?.state}};
    }
    const points=new Map((previous?.samples||[]).map(p=>[p.point[0],p]));
    const stateSource=incoming.keepRobotState?previous:incoming;
    for(const point of incoming.points){const old=points.get(point[0]);if(!old||eventId>old.eventId)points.set(point[0],{point,eventId,receivedAt:incoming.receivedAt,robotState:stateSource?.robotState??null,robotStateAt:stateSource?.robotStateAt||''});}
    const samples=[...points.values()].sort((a,b)=>a.point[0]-b.point[0]).slice(-5000);
    return {...incoming,receivedAt:eventId>(previous?.eventId||0)?incoming.receivedAt:previous.receivedAt,eventId:Math.max(eventId,previous?.eventId||0),robotState:samples.at(-1)?.robotState??null,robotStateAt:samples.at(-1)?.robotStateAt||'',samples,points:samples.map(p=>p.point)};
  }
  function fleetLatest(sessions,scenarioId,now=Date.now()){
    const latest=new Map();
    for(const r of sessions){if(r.scenarioId!==scenarioId)continue;const key=JSON.stringify([r.source||'',r.robot]);if(!latest.has(key)||r.eventId>latest.get(key).eventId)latest.set(key,r);}
    return [...latest.values()].map(r=>{
      const sample=r.samples?.findLast(s=>valid(s.point)),point=sample?.point||r.points.findLast(valid)||null;
      const stamp=Date.parse(r.receivedAt),age=Number.isFinite(stamp)?Math.max(0,(now-stamp)/1000):null;
      const missing=!valid(r.points.at(-1)),state=!point?'unavailable':missing?'missing':age===null?'unknown':age>=15?'stale':age>=5?'delayed':'fresh';
      let hash=0;for(const c of r.robot)hash=(hash*31+c.charCodeAt(0))>>>0;
      return {id:r.id,robot:r.robot,source:r.source||'',scenarioId,point,time:r.points.at(-1)?.[0]??0,eventId:r.eventId,robotState:r.robotState??null,robotStateAt:r.robotStateAt||'',receivedAt:r.receivedAt,positionAt:sample?.receivedAt||r.receivedAt,age,state,color:['#008b89','#6d5bc5','#be6b13','#2975bc','#b94977','#54842b'][hash%6]};
    });
  }
  const poseHistories=new WeakMap();
  function poseAt(run,time,history=null){
    if(!run.points.length||time<run.points[0][0])return {state:'unavailable',point:null,age:0};
    if(!history){history=poseHistories.get(run);if(!history){history=motionIndex(run);poseHistories.set(run,history);}}
    const i=indexAt(run,time),pose=displayPose(run,i,time,history);
    if(pose.state==='preview')return {state:'unavailable',point:null,age:0};
    if(pose.point&&pose.state==='observed'&&time-pose.point[0]>.25)return {...pose,state:'held',age:time-pose.point[0]};
    return pose;
  }
  function fleetSessions(sessions,scenarioId){
    const latest=new Map();for(const r of sessions){if(r.scenarioId!==scenarioId)continue;const key=JSON.stringify([r.source||'',r.robot]);if(!latest.has(key)||r.eventId>latest.get(key).eventId)latest.set(key,r);}return [...latest.values()];
  }
  function fleetTimeline(sessions,scenarioId){
    const selected=fleetSessions(sessions,scenarioId),times=[...new Set(selected.flatMap(r=>r.points.map(p=>p[0])))].sort((a,b)=>a-b);
    const start=times[0]??0,end=times.at(-1)??start,coverage=selected.flatMap(r=>r.points.filter(valid).map(p=>[p[0],Math.min(end,p[0]+.25)])).sort((a,b)=>a[0]-b[0]);
    const gaps=[];let cursor=start;for(const [a,b] of coverage){if(a>cursor)gaps.push([cursor,a]);cursor=Math.max(cursor,b);}if(cursor<end)gaps.push([cursor,end]);
    return {start,end,duration:end-start,times,gaps};
  }
  function fleetAt(sessions,scenarioId,time,now=Date.now()){
    const selected=fleetSessions(sessions,scenarioId);
    return fleetLatest(selected,scenarioId,now).map(entry=>{
      const r=selected.find(r=>r.id===entry.id&&r.robot===entry.robot&&(r.source||'')===entry.source),display=poseAt(r,time);
      const current=r.points[indexAt(r,time)],state=!display.point?'unavailable':!valid(current)?'missing':entry.state==='unavailable'||entry.state==='missing'?'unknown':entry.state;
      return {...entry,point:display.point,time,displayState:display.state,state};
    });
  }
  // Live Isaac pose.yaw is radians; old fixed-heading display settings do not override it.
  function liveBodyAngle(body,point){
    return body&&finite(point?.[3])?point[3]+body.angle*Math.PI/180:null;
  }
  function receiptLabel(receivedAt,now=Date.now()){
    const stamp=Date.parse(receivedAt);
    if(!Number.isFinite(stamp)||stamp>now)return 'Last update time unavailable';
    return `Last update ${Math.floor((now-stamp)/1000)}s ago`;
  }
  // One unresolved floor report per source/factory/robot. Never infer a cross-check result.
  function floorAlert(previous,incoming,eventId){
    const stamp=incoming.observedAt,priority=incoming.statePriority||0;
    if(previous){
      const ordered=finite(stamp)&&finite(previous.stamp);
      if(ordered?(stamp<previous.stamp||stamp===previous.stamp&&(priority<previous.priority||priority===previous.priority&&eventId<=previous.eventId)):eventId<=previous.eventId)return previous;
    }
    const state=primaryState(incoming.robotState);
    if(!state)return previous;
    let pin=previous?.pin||null,dismissed=previous?.dismissed||false;
    if(state==='inspection_wait'){pin=null;dismissed=false;}
    else if(state==='anomaly_detected'&&!pin&&(!dismissed||previous?.state!==state)){
      const point=incoming.points.at(-1);
      if(valid(point))pin={id:eventId,runId:incoming.id,robotId:incoming.robot,source:incoming.source||'',scenarioId:incoming.scenarioId,
        time:point[0],x:point[1],y:point[2],kind:'attention',title:'Suspected floor anomaly',floorAlert:true};
      dismissed=false;
    }
    return {stamp,priority,eventId,state,pin,dismissed};
  }
  function pickupBadge(previous,kind,state,now){
    state=primaryState(state);
    if(['anomaly_detected','inspection_wait','cross_check','fault','error','failed','collision','aborted','reset'].includes(state))
      return {lastKind:kind,kind,until:0};
    // Preserve the minimum visibility on done; a new phase replaces Drop-off immediately.
    if(kind==='unloading'){
      const until=previous?.lastKind==='unloading'?previous.until:now+1000;
      return {lastKind:kind,kind,until};
    }
    if(previous?.kind==='unloading'){
      if(!state||(state==='done'&&now<previous.until))
        return {lastKind:state==='done'?kind:previous.lastKind,kind:'unloading',until:previous.until};
      previous=null;
    }
    const pickup=kind==='loading'||kind==='pickup';
    if(pickup){
      const until=previous?.lastKind===kind?previous.until:now+3000;
      return {lastKind:kind,kind,until};
    }
    // Keep the just-observed Pickup visible for its minimum duration, then show transport.
    if(kind==='carrying'&&['loading','pickup'].includes(previous?.kind)&&now<previous.until)
      return {lastKind:kind,kind:previous.kind,until:previous.until};
    // A different active operation takes precedence over the lingering pickup.
    if(kind)return {lastKind:kind,kind,until:0};
    return {lastKind:kind,kind:previous&&now<previous.until?previous.kind:null,until:previous?.until||0};
  }
  function robotStateIcon(state){
    state=primaryState(state);
    // Hide Pause overlays only; recorded states remain available elsewhere.
    if(['idle','waiting','blocked','stopped','paused','aborted'].includes(state))return null;
    // Transport labels are inferred from the recorded task phase, not a load sensor.
    // An incident does not change the current state.
    const kinds={anomaly_detected:'warning',inspection_wait:'inspection',cross_check:'cross_check',turn_out:'carrying',to_drop:'carrying',turn_drop:'carrying',turn_unload:'carrying',loading:'loading',unloading:'unloading',idle:'blocked',waiting:'blocked',blocked:'blocked',stopped:'blocked',paused:'blocked',aborted:'blocked',carrying:'carrying',loaded:'carrying',pickup:'pickup',picking:'pickup',dropoff:'dropoff',dropping:'dropoff',error:'fault',fault:'fault',failed:'fault',collision:'collision'};
    return Object.hasOwn(kinds,state)?kinds[state]:null;
  }
  function reportedStopLabel(state){
    state=primaryState(state);
    // Reported operating state only; unchanged coordinates do not establish this state.
    const labels={idle:'Idle',waiting:'Idle',blocked:'Path blocked',stopped:'Stopped',paused:'Paused'};return Object.hasOwn(labels,state)?labels[state]:null;
  }
  function stationaryIntervals(run,{tolerance=.01,minDuration=1,maxGap=.25}={}){
    if(!finite(tolerance)||tolerance<=0||!finite(minDuration)||minDuration<=0||!finite(maxGap)||maxGap<=0)throw Error('Invalid stop detection settings.');
    const result=[];let candidate=null,previous=null;
    const finish=reason=>{
      if(candidate?.confirmed!==null&&candidate?.confirmed!==undefined)result.push({id:`stop-${candidate.startIndex}`,runId:run.id,robotId:run.robot,kind:'stop',
        x:candidate.first[1],y:candidate.first[2],startTime:candidate.first[0],time:candidate.confirmed,endTime:candidate.last[0],endReason:reason});
      candidate=null;
    };
    run.points.forEach((p,i)=>{
      if(!valid(p)){finish('gap');previous=null;return;}
      if(previous&&(p[0]-previous[0]>maxGap||p[0]<=previous[0]))finish('gap');
      if(candidate){
        const minX=Math.min(candidate.minX,p[1]),maxX=Math.max(candidate.maxX,p[1]),minY=Math.min(candidate.minY,p[2]),maxY=Math.max(candidate.maxY,p[2]);
        if(Math.hypot(maxX-minX,maxY-minY)>tolerance)finish('movement');
        else Object.assign(candidate,{minX,maxX,minY,maxY,last:p});
      }
      if(!candidate)candidate={first:p,last:p,startIndex:i,minX:p[1],maxX:p[1],minY:p[2],maxY:p[2],confirmed:null};
      if(candidate.confirmed===null&&p[0]-candidate.first[0]>=minDuration-1e-6)candidate.confirmed=p[0];
      previous=p;
    });
    finish('end');return result;
  }
  function incidentPlacement(x,y,size,width,height,occupied=[]){
    // Keep symbols within 36 CSS pixels of their true location, at every zoom.
    const r=size/2+3,clamp=(v,a,b)=>Math.max(a,Math.min(b,v));
    const candidates=[[18,-25],[-18,-25],[30,4],[-30,4],[18,25],[-18,25],[0,-34],[0,34],[0,0]];
    let best=null;
    for(const [dx,dy] of candidates){
      const px=clamp(x+dx,r,width-r),py=clamp(y+dy,r,height-r);
      if(Math.hypot(px-x,py-y)>36)continue;
      const overlap=occupied.reduce((sum,p)=>sum+Math.max(0,r+p.r-Math.hypot(px-p.px,py-p.py)),0);
      const next={px,py,r,overlap};if(!best||overlap<best.overlap)best=next;
      if(overlap===0)break;
    }
    return best||{px:x,py:y,r,overlap:0};
  }
  function geometryProfile(value){
    if(!value||value.version!==1||typeof value.template!=='boolean'||!Array.isArray(value.obstacles)||value.obstacles.length>100)throw Error('Invalid obstacle settings.');
    const number=(v,label,min,max)=>{if(!finite(v)||v<min||v>max)throw Error(`${label}: check the value.`);return v;};
    const size=(v,label)=>{number(v,label,0,1e6);if(v===0)throw Error(`${label} must be greater than zero.`);return v;};
    const ids=new Set();
    const obstacles=value.obstacles.map(o=>{
      if(!o||typeof o.id!=='string'||!o.id||o.id.length>80||ids.has(o.id)||typeof o.name!=='string'||!o.name.trim()||o.name.length>60||!['obstacle','wall','shelf'].includes(o.kind))throw Error('Check the obstacle name and type.');
      ids.add(o.id);return {id:o.id,name:o.name.trim(),kind:o.kind,x:number(o.x,'Center X',-1e9,1e9),y:number(o.y,'Center Y',-1e9,1e9),width:size(o.width,'Width'),height:size(o.height,'Height'),angle:number(o.angle,'Rotation',-360,360)};
    });
    let body=null;
    if(value.body!==null){const b=value.body;if(!b||!['fixed','yaw-rad','yaw-deg'].includes(b.heading))throw Error('Check the heading source.');body={length:size(b.length,'Robot length'),width:size(b.width,'Robot width'),heading:b.heading,angle:number(b.angle,'Robot angle',-360,360)};}
    return {version:1,template:value.template,obstacles,body};
  }
  function rectangleCorners(x,y,width,height,angle){
    const a=angle*Math.PI/180,c=Math.cos(a),s=Math.sin(a);
    return [[-1,-1],[1,-1],[1,1],[-1,1]].map(([u,v])=>[x+u*width/2*c-v*height/2*s,y+u*width/2*s+v*height/2*c]);
  }
  function bodyAngle(body,run,index,time,display){
    const offset=body.angle*Math.PI/180;if(body.heading==='fixed')return offset;
    if(!['observed','interpolated'].includes(display.state))return null;
    const p=run.points[index],q=run.points[index+1],factor=body.heading==='yaw-deg'?Math.PI/180:1;
    if(!finite(p?.[3]))return null;let angle=p[3]*factor;
    if(display.state==='interpolated'){
      if(!finite(q?.[3])||q[0]<=p[0])return null;
      const delta=Math.atan2(Math.sin(q[3]*factor-angle),Math.cos(q[3]*factor-angle));
      angle+=delta*Math.max(0,Math.min(1,(time-p[0])/(q[0]-p[0])));
    }
    return angle+offset;
  }
  const symbolKinds={robot:'Default robot',collision:'Collision',pickup:'Pickup',carrying:'Carrying point',dropoff:'Drop-off',blocked:'Waiting point',fault:'Robot fault'};
  function symbolProfile(value){
    if(value?.version!==1||typeof value.robotImage!=='boolean'||!Array.isArray(value.states)||value.states.length>1000||!Array.isArray(value.markers)||value.markers.length>100)throw Error('Invalid marker settings.');
    const kinds=Object.keys(symbolKinds),ids=new Set(),runs=new Set(),str=(v,n)=>typeof v==='string'&&v.length>0&&v.length<=n;
    const states=value.states.map(s=>{if(!str(s?.runId,180)||runs.has(s.runId)||!kinds.includes(s.kind))throw Error('Check the robot state.');runs.add(s.runId);return {runId:s.runId,kind:s.kind};});
    const markers=value.markers.map(m=>{
      if(!str(m?.id,80)||ids.has(m.id)||!str(m.runId,180)||!kinds.slice(1).includes(m.kind)||typeof m.name!=='string'||m.name.length>40||![m.x,m.y].every(v=>finite(v)&&Math.abs(v)<=1e9)||(m.time!==null&&(!finite(m.time)||Math.abs(m.time)>1e15)))throw Error('Check the marker type, coordinates and time.');
      ids.add(m.id);return {id:m.id,runId:m.runId,kind:m.kind,name:m.name.trim(),x:m.x,y:m.y,time:m.time};
    });
    return {version:1,robotImage:value.robotImage,states,markers};
  }
  function zoneContains(zone,x,y){
    return !!zone&&['x1','y1','x2','y2'].every(key=>finite(zone[key]))&&zone.x1!==zone.x2&&zone.y1!==zone.y2&&
      x>=Math.min(zone.x1,zone.x2)&&x<=Math.max(zone.x1,zone.x2)&&y>=Math.min(zone.y1,zone.y2)&&y<=Math.max(zone.y1,zone.y2);
  }
  function handoffFloorAttention(reports,zones){
    const source=value=>String(value||'').replace(/\/$/,'');
    const retained=[],retired=[];
    for(const report of reports){
      const matches=zones.filter(zone=>source(zone.source)===source(report.source)&&zone.scenarioId===report.scenarioId&&
        zoneContains(zone,report.x,report.y))
        .sort((a,b)=>Math.hypot(report.x-a.x,report.y-a.y)-Math.hypot(report.x-b.x,report.y-b.y));
      if(matches.length)retired.push({report,zone:matches[0]});
      else retained.push({...report,kind:'floor_suspected',title:'Suspected floor anomaly'});
    }
    // This is display handoff only: never send zone-clear or erase an AE zone here.
    return {pins:[...retained,...zones],retired};
  }
  return {zoneContains,handoffFloorAttention,floorAlert,pickupBadge,liveBodyAngle,receiptLabel,mapContentBounds,mapCorners,factoryMaps,factoryScenario,normalize,yaml,mapPoint,csv,valid,linkEvent,motionIndex,displayPose,sceneBounds,trailSegments,movementAngle,predictedPath,smoothArrow,indexAt,missingIntervals,followView,eventLocation,geometryProfile,rectangleCorners,bodyAngle,symbolProfile,symbolKinds,incidentKind,incidentVisible,incidentPlacement,stationaryIntervals,robotStateIcon,reportedStopLabel,receivedCoordinates,rollingTrail,expireTrail,robotColor,mergeReceived,fleetLatest,poseAt,fleetSessions,fleetTimeline,fleetAt};
})();
if (typeof module !== 'undefined' && module.exports) module.exports=FloorplanData;
if (typeof document !== 'undefined') (() => {
  'use strict';
  const $=id=>document.getElementById(id), root=$('floorplan-replay');if(!root)return;
  const select=$('floorplan-run'),canvas=$('floorplan-canvas'),ctx=canvas.getContext('2d'),empty=$('floorplan-empty');
  const fields=['robot','cond','load','spd','scn'];
  const labels={idle:'Stopped',straight:'Straight',left:'Turn left',right:'Turn right',low:'Low speed',mid:'Medium',high:'High speed',none:'Unloaded',max:'Maximum',normal:'normal · Normal'};
  let runs=[],filtered=[],run=null,index=0;
  const playing=false; // Rendering never advances a simulated clock.
  let view={x:0,y:0,zoom:1,extent:2},projection=null,breaks=new Set(),map=null,pendingImage=null,imageVersion=0,yamlVersion=0,drag=null;
  let viewScenario=null,viewDataset=null;
  let scene=null,scenes=new Map(),fitted=true,follow=false,selectedRobot=null,eventSelected=null,robotHit=null,selectionSync=false;
  let playhead=0,history={first:-1,previous:[]};
  let arrowMotion=null,robotMotion=null,motionRun=null,motionIndex=-1,motionTime=-Infinity,motionClock=0,motionReset=true;
  let arrowPixels=null,arrowMode=null,arrowScale=null;
  let cameraFrame=0,cameraClock=0,linkIssue=false,lastIssue='',autoHud=false;
  const eventPins=new Map(),seenEvents=new Map();let pinHits=[],robotBoxes=[];
  let geometryDataset='received-v2',geometryEdit=null,geometrySerial=0;
  const geometryPersistent=!!globalThis.crypto?.subtle;
  const geometryCache=new Map(),geometryWarnings=new Map();
  const symbolCache=new Map(),symbolWarnings=new Map();
  let symbolEdit=null,symbolPlacement=null,symbolHits=[];
  const incidentLayouts=[];let stopIntervals=[],stopHits=[];
  const liveMode=true,liveFollow=true;
  let liveBusy=false;
  const receivedRuns=new Map(),receivedDone=new Set(),receivedQueue=new Map(),receivedTrails=new Map();
  const normalizedRuns=new WeakMap(),fieldChoices=new Map(),receivedPending=new Set(),trailQueue=new Map();
  let renderFrame=0,pendingReceived=null,renderingFrame=false,drawing=false,frameFleet=null,trailTimer=0;
  let activeScenario=window.edgewayFactory||'scenario-1',scenarioProfiles={},scenarioWarning='',fleetHits=[];
  const scenarioIds=['scenario-1','scenario-2','scenario-3'];
  for(let i=0;i<3;i++)scenarioProfiles[scenarioIds[i]]={name:`Factory ${'ABC'[i]}`,geometry:i===0?null:{version:1,template:false,obstacles:[]},bodies:{}};
  try{const raw=localStorage.getItem('edgeway.scenarios.v1');if(raw){const saved=JSON.parse(raw),validated={};for(const id of scenarioIds){const v=saved[id];if(!v||typeof v.name!=='string'||!v.name.trim()||v.name.length>40)throw Error('Scenario format');if(v.geometry)FloorplanData.geometryProfile({...v.geometry,body:null});if(!v.bodies||typeof v.bodies!=='object'||Array.isArray(v.bodies))throw Error('Robot dimension format');for(const b of Object.values(v.bodies))FloorplanData.geometryProfile({version:1,template:true,obstacles:[],body:b});validated[id]={...v,name:/^공장 [ABC]$/.test(v.name)?v.name.replace("공장 ","Factory "):v.name};}scenarioProfiles=validated;}}catch{scenarioWarning='Could not read scenario settings. Using defaults.';}
  function saveScenario(id,next){
    const updated={...scenarioProfiles,[id]:next};
    try{localStorage.setItem('edgeway.scenarios.v1',JSON.stringify(updated));}catch{throw Error('Could not save scenario. Previous settings retained.');}
    scenarioProfiles=updated;scenarioWarning='';syncScenarioControls();
  }
  function syncScenarioControls(){
    $('fp-scenario').replaceChildren(...scenarioIds.map((id,i)=>new Option(`Factory ${'ABC'[i]}`,id)));$('fp-scenario').value=activeScenario;
    $('fp-scenario-name').value=scenarioProfiles[activeScenario].name;root.dataset.scenarioId=activeScenario;
    const preset=FloorplanData.factoryMaps[activeScenario];$('fp-map-preset').disabled=!preset;
    $('fp-map-preset-info').textContent=preset?`${preset.image} · 0.05 m/pixel · origin [${preset.origin.join(', ')}]`:'Factory C map is not available yet. Register an image and YAML manually.';
  }
  const fleetColors=new Map();
  let timelineRuns=null,timelineScenario=null,timelineValue=null;
  function commonTimeline(){if(timelineRuns!==runs||timelineScenario!==activeScenario){timelineRuns=runs;timelineScenario=activeScenario;timelineValue=FloorplanData.fleetTimeline(runs,activeScenario);}return timelineValue;}
  function shownPose(){return run?(liveMode?FloorplanData.poseAt(run,playhead,history):FloorplanData.displayPose(run,index,playhead,history)):{state:'unavailable',point:null,age:0};}
  function seekCommon(time){const clock=commonTimeline();playhead=Math.max(clock.start,Math.min(clock.end,time));if(run)index=FloorplanData.indexAt(run,playhead);resetMotion();render(false);}
  function fleet(){if(!liveMode)return [];if(drawing&&frameFleet)return frameFleet;const result=FloorplanData.fleetAt(runs,activeScenario,playhead).map(e=>{const key=JSON.stringify([e.source,e.robot]);if(!fleetColors.has(key))fleetColors.set(key,FloorplanData.robotColor(e.robot,e.source));return {...e,color:fleetColors.get(key)};});if(drawing)frameFleet=result;return result;}
  function fleetStateLabel(entry){const label={fresh:'Live',delayed:'Delayed',stale:'Stale position',missing:'No coordinates',unavailable:'No position',unknown:'Reception time unknown'}[entry.state];return label+(entry.age===null?'':` · ${Math.floor(entry.age)} s ago`);}
  function selectFleet(entry){
    const target=runs.find(r=>r.robot===entry.robot&&r.id===entry.id&&r.source===entry.source&&r.scenarioId===activeScenario);if(!target)return;
    const oldView={...view},wasFitted=fitted;for(const k of fields)$('fp-'+k).value='';filter(target.id);
    select.value=String(filtered.indexOf(target));choose();selectedRobot=target.robot;
    if(liveFollow)receivedTail();if(!wasFitted){view=oldView;fitted=false;}draw();
  }
  function robotStateLabel(state){const phase=RobotState.primary(state),labels={anomaly_detected:'Suspected robot fault',inspection_wait:'Robot fault',cross_check:'Cross-checking',loading:'Picking up',unloading:'Unloading',to_drop:'Moving to drop-off',reset:'Resetting',to_pick:'Moving to pickup',aborted:'Task aborted'};return labels[phase]||phase||'Not provided';}
  function oldRobotState(entry){const t=Date.parse(entry?.robotStateAt);return entry?.state!=='fresh'||!Number.isFinite(t)||Date.now()-t>=5000;}
  function robotName(id){const m=/^site([ABC])_AMR_(\d+)(?:_|$)/.exec(id||'');return m?m[1]+'-'+m[2]:id;}
  const floorClearPending=new Set();
  function confirmFloorClear(){
    const dialog=$('floor-clear-dialog');
    if(dialog.open)return Promise.resolve(false);
    dialog.returnValue='';
    return new Promise(resolve=>{
      dialog.addEventListener('close',()=>resolve(dialog.returnValue==='send'),{once:true});
      dialog.showModal();
    });
  }
  async function clearFloorAlert(key,pin=null){
    const target=pin;
    if(!target)return;
    const pendingKey=target.judgementZoneId||key;
    if(floorClearPending.has(pendingKey))return;
    floorClearPending.add(pendingKey);
    try{
      if(!await confirmFloorClear())return;
      if(!window.FloorJudgement?.clearAttention)throw Error('The clear action is still loading. Try again in a moment.');
      const result=await window.FloorJudgement.clearAttention(target);
      if(result.state!=='sent')throw Error('The CSE did not confirm the clear request. The marker is still on the map.');
      draw();message('Floor anomaly clear request sent to the Judgement AE.');
    }catch(error){message(error.message||'Could not send the clear request. The marker is still on the map.');}
    finally{floorClearPending.delete(pendingKey);}
  }
  const pickupBadges=new Map();
  function visibleRobotKind(entry){
    const kind=FloorplanData.robotStateIcon(entry?.robotState);
    if(!liveMode||!entry)return kind;
    const key=JSON.stringify([entry.scenarioId,entry.source,entry.id,entry.robot]);
    const next=FloorplanData.pickupBadge(pickupBadges.get(key),kind,entry.robotState,performance.now());
    pickupBadges.set(key,next);
    while(pickupBadges.size>128)pickupBadges.delete(pickupBadges.keys().next().value);
    return next.kind;
  }
  function robotBadge(id,x,y,color,selected=false,entry=null){
    if(x<0||x>projection.w||y<0||y>projection.h)return;
    ctx.save();ctx.setLineDash([]);ctx.globalAlpha=1;
    const kind=visibleRobotKind(entry),old=oldRobotState(entry);
    const robotOutline=kind==='warning'?'#a56b00':kind==='inspection'?'#c82e43':'#246fd1';
    const body=geometry(id).body,target=runs.find(r=>r.robot===id&&(!entry||r.id===entry.id&&r.source===entry.source));
    const point=entry?.point,at=point?.[0]??playhead;
    const angle=liveMode?FloorplanData.liveBodyAngle(body,point):(body&&target?FloorplanData.bodyAngle(body,target,FloorplanData.indexAt(target,at),at,{state:'observed',point}):null);
    const theta=(angle??0)+projection.rotation,lengthPx=body?body.length*projection.s:0,widthPx=body?body.width*projection.s:0;
    const ringDiameter=body?Math.max(lengthPx,widthPx):0;
    const ringBox=body?{x:x-ringDiameter/2,y:y-ringDiameter/2,w:ringDiameter,h:ringDiameter}:null;
    let footprint=null,corners=[],bodyBox={x:x-4,y:y-4,w:8,h:8};
    if(body){
      // Metres are converted using the map scale only: never clamp the physical body to a pixel size.
      const c=Math.cos(theta),s=Math.sin(theta);
      corners=[[-1,-1],[1,-1],[1,1],[-1,1]].map(([u,v])=>{const dx=u*lengthPx/2,dy=v*widthPx/2;return [x+dx*c+dy*s,y-dx*s+dy*c];});
      const xs=corners.map(p=>p[0]),ys=corners.map(p=>p[1]);
      bodyBox={x:Math.min(...xs),y:Math.min(...ys),w:Math.max(...xs)-Math.min(...xs),h:Math.max(...ys)-Math.min(...ys)};
      const drawn=true;
      // All details stay inside the physical 3.12 x 1.14 m footprint.
      ctx.save();ctx.translate(x,y);ctx.rotate(-theta);
      const edge=Math.min(1.2,widthPx*.08);
      ctx.fillStyle='#303940';
      for(const side of [-1,1])for(const end of [-1,1])
        ctx.fillRect(end*lengthPx*.34-lengthPx*.07,side*widthPx*.37-widthPx*.13,lengthPx*.14,widthPx*.26);
      ctx.fillStyle='#d6dadd';ctx.strokeStyle='#606b73';ctx.lineWidth=edge;
      ctx.beginPath();ctx.roundRect(-lengthPx/2+edge/2,-widthPx*.38,lengthPx-edge,widthPx*.76,widthPx*.12);ctx.fill();ctx.stroke();
      ctx.fillStyle='#f1f3f4';ctx.fillRect(-lengthPx*.28,-widthPx*.24,lengthPx*.55,widthPx*.12);
      ctx.fillStyle='#37434b';ctx.fillRect(lengthPx*.39,-widthPx*.2,lengthPx*.055,widthPx*.4);
      ctx.fillStyle='#f5ac4d';ctx.fillRect(lengthPx*.41,-widthPx*.12,lengthPx*.025,widthPx*.1);ctx.restore();
      if(run?.robot===id)canvas.dataset.robotImage=String(drawn);
      ctx.globalAlpha=1;ctx.save();ctx.translate(x,y);
      const stroke=Math.min(selected?1.7:1.4,lengthPx/4,widthPx/4);
      ctx.strokeStyle=robotOutline;ctx.lineWidth=stroke;if(old||angle===null)ctx.setLineDash([3,2]);
      ctx.beginPath();ctx.arc(0,0,(ringDiameter-stroke)/2,0,Math.PI*2);ctx.stroke();ctx.restore();
      footprint={length:body.length,width:body.width,angle,radius:Math.hypot(lengthPx,widthPx)/2,scale:projection.s,screenAngle:theta};
    }else{
      // Without dimensions, show a location cross instead of inventing a physical robot footprint.
      ctx.strokeStyle=robotOutline;ctx.lineWidth=2;ctx.beginPath();ctx.moveTo(x-4,y);ctx.lineTo(x+4,y);ctx.moveTo(x,y-4);ctx.lineTo(x,y+4);ctx.stroke();
    }
    const annotationBox=ringBox||bodyBox;
    robotBoxes.push({x:bodyBox.x-3,y:bodyBox.y-3,w:bodyBox.w+6,h:bodyBox.h+6});
    if(ringBox)robotBoxes.push({x:ringBox.x-3,y:ringBox.y-3,w:ringBox.w+6,h:ringBox.h+6});
    const labelScale=annotationScale();ctx.font=`600 ${12*labelScale}px Pretendard,sans-serif`;const text=robotName(id),tw=ctx.measureText(text).width;
    const idBox=annotationPlacement(x,annotationBox.y-17*labelScale,tw+12*labelScale,22*labelScale,robotBoxes);
    ctx.strokeStyle=color;ctx.lineWidth=1;ctx.beginPath();ctx.moveTo(x,annotationBox.y);ctx.lineTo(idBox.x+idBox.w/2,idBox.y+idBox.h);ctx.stroke();
    ctx.fillStyle='#ffffff';ctx.lineWidth=1.6*labelScale;ctx.beginPath();ctx.roundRect(idBox.x,idBox.y,idBox.w,idBox.h,4*labelScale);ctx.fill();ctx.stroke();
    ctx.fillStyle='#173c40';ctx.textAlign='left';ctx.fillText(text,idBox.x+6*labelScale,idBox.y+15*labelScale);
    robotBoxes.push(idBox);
    let stateBox=null;
    if(kind){
      const size=badgeDimensions(kind,old);
      stateBox=annotationPlacement(annotationBox.x+annotationBox.w+8+size.w/2,y,size.w,size.h,robotBoxes);
      ctx.save();ctx.strokeStyle=badgeStyle(kind).ink;ctx.globalAlpha=.45;ctx.lineWidth=1;
      ctx.beginPath();ctx.moveTo(x,y);ctx.lineTo(stateBox.x+stateBox.w/2,stateBox.y+stateBox.h/2);ctx.stroke();ctx.restore();
      statusBadge(kind,stateBox,old);robotBoxes.push(stateBox);
    }
    canvas.dataset.robotIcons=JSON.stringify([...JSON.parse(canvas.dataset.robotIcons||'[]'),{robot:id,heading:angle,screenHeading:theta,kind:kind||'robot',x,y,lengthM:body?.length??null,widthM:body?.width??null,lengthPx,widthPx,corners,bodyBox,ringBox,ringDiameter,badgeSize:stateBox?.h||0,stateBox,idBox,representation:'metric-robot-with-state-badge',style:'compact-vector',previous:old}]);
    ctx.restore();
    return footprint;
  }
  function drawFleet(X,Y,h){
    fleetHits=[];canvas.dataset.robotIcons='[]';const entries=fleet();canvas.dataset.fleet=JSON.stringify(entries);
    const stops=entries.filter(e=>e.point&&FloorplanData.reportedStopLabel(e.robotState)).map(e=>({robot:e.robot,state:e.robotState,label:FloorplanData.reportedStopLabel(e.robotState),previous:oldRobotState(e)}));
    canvas.dataset.reportedStops=JSON.stringify(stops);
    const selected=entries.find(e=>e.robot===run?.robot&&e.id===run?.id);
    if(selected){$('fp-robot-detail').textContent=selected.robot+' · Received state: '+(robotStateLabel(selected.robotState))+(selected.point?' · X '+selected.point[1].toFixed(3)+' · Y '+selected.point[2].toFixed(3):'');}
    const list=$('fp-fleet');list.hidden=!liveMode;
    const signature=JSON.stringify(entries.map(e=>[e.robot,e.id,e.source,e.state,Math.floor(e.age??-1),run?.robot===e.robot]));
    if(list.dataset.signature!==signature){list.dataset.signature=signature;list.replaceChildren();for(const e of entries){const b=document.createElement('button');b.type='button';b.className='secondary';b.style.borderLeft=`4px solid ${e.color}`;b.textContent=`${e.robot} · ${fleetStateLabel(e)}`;b.setAttribute('aria-pressed',String(run?.robot===e.robot&&run?.id===e.id));b.onclick=()=>selectFleet(e);list.append(b);}if(!entries.length)list.textContent='No robots received in this scenario.';}
    for(const e of entries){
      if(!e.point)continue;
      const x=X(e.point[1],e.point[2]),y=Y(e.point[2],e.point[1]);
      if(run?.robot===e.robot&&run?.source===e.source){fleetHits.push({...e,x,y,r:10,footprint:null});continue;}
      const footprint=robotBadge(e.robot,x,y,e.color,false,e);
      fleetHits.push({...e,x,y,r:10,footprint});
    }
    canvas.dataset.fleetHits=JSON.stringify(fleetHits);
  }
  let scenarioDBPromise=null;
  function scenarioDB(){if(!scenarioDBPromise)scenarioDBPromise=new Promise((resolve,reject)=>{const req=indexedDB.open('edgeway-scenario-maps',1);req.onupgradeneeded=()=>req.result.createObjectStore('maps',{keyPath:'id'});req.onsuccess=()=>resolve(req.result);req.onerror=()=>reject(req.error);});return scenarioDBPromise;}
  async function mapRecord(id,value){const db=await scenarioDB();return new Promise((resolve,reject)=>{const tx=db.transaction('maps',value===undefined?'readonly':'readwrite'),store=tx.objectStore('maps');const req=value===undefined?store.get(id):value===null?store.delete(id):store.put({...value,id});let result;req.onsuccess=()=>{result=req.result;};tx.oncomplete=()=>resolve(result);tx.onerror=()=>reject(tx.error);tx.onabort=()=>reject(tx.error);});}
  function contentBounds(image){
    const surface=document.createElement('canvas');surface.width=image.width;surface.height=image.height;
    try{const context=surface.getContext('2d',{willReadFrequently:true});context.drawImage(image,0,0);
      return FloorplanData.mapContentBounds(context.getImageData(0,0,image.width,image.height).data,image.width,image.height);
    }catch{return {x:0,y:0,w:image.width,h:image.height};}
  }
  async function bundledMap(id){
    const preset=FloorplanData.factoryMaps[id];if(!preset)return null;
    const base='/factory-maps/'+preset.image.slice(0,-4);
    const [png,yaml]=await Promise.all([fetch(base+'.png',{cache:'no-cache',signal:AbortSignal.timeout(10000)}),fetch(base+'.yaml',{cache:'no-cache',signal:AbortSignal.timeout(10000)})]);
    if(!png.ok||!yaml.ok)throw Error('Bundled map files are unavailable. Add a map PNG and YAML in Map settings.');
    const meta=FloorplanData.yaml(await yaml.text());
    if(meta.resolution!==preset.resolution||meta.origin.some((v,i)=>Math.abs(v-(preset.sourceOrigin||preset.origin)[i])>1e-8))throw Error('Server map YAML differs from the confirmed coordinate settings.');
    return {name:preset.image,blob:await png.blob(),...meta,origin:preset.origin,bundled:true};
  }
  // At most three decoded maps; in-flight loads are shared per factory.
  const preparedMaps=new Map(),mapLoads=new Map(),scenarioViews=new Map();
  let mapLoading=false,displayedScenario=null;
  function releaseMap(value){
    if(value?.image&&value.image!==map?.image&&value.image!==pendingImage?.image
      &&![...preparedMaps.values()].some(m=>m.image===value.image))value.image.close?.();
  }
  function invalidateMaps(id){
    // Each factory owns its saved image, origin and view.
    for(const key of [id]){
      const old=preparedMaps.get(key);preparedMaps.delete(key);mapLoads.delete(key);
      scenarioViews.delete(key);releaseMap(old);
    }
  }
  function prepareMap(id){
    if(preparedMaps.has(id))return Promise.resolve(preparedMaps.get(id));
    if(mapLoads.has(id))return mapLoads.get(id);
    const task=(async()=>{
      let saved=null;try{saved=await mapRecord(id);}catch{/* Server fallback. */}
      if(id==='scenario-3'&&saved?.name==='siteB.png'&&saved.origin?.every((v,i)=>Math.abs(v-FloorplanData.factoryMaps['scenario-2'].origin[i])<1e-8))saved=null;
      saved=saved||await bundledMap(id);
      if(!saved)return null;
      if(!Number.isFinite(saved.resolution)||saved.resolution<=0||!Array.isArray(saved.origin)||saved.origin.length!==3||!saved.origin.every(Number.isFinite)||!(saved.blob instanceof Blob))throw Error('Background settings error');
      const image=/\.pgm$/i.test(saved.name)?readPGM(new Uint8Array(await saved.blob.arrayBuffer())):await createImageBitmap(saved.blob);
      const preset=FloorplanData.factoryMaps[id];
      if(saved.bundled&&(image.width!==preset.width||image.height!==preset.height)){image.close?.();throw Error('Original map dimensions do not match. Resized previews are not supported.');}
      if(mapLoads.get(id)!==task){image.close?.();return null;}
      const prepared={image,name:saved.name,file:saved.blob,resolution:saved.resolution,origin:saved.origin,bundled:!!saved.bundled,contentBounds:contentBounds(image)};
      preparedMaps.set(id,prepared);return prepared;
    })();
    mapLoads.set(id,task);
    task.finally(()=>{if(mapLoads.get(id)===task)mapLoads.delete(id);}).catch(()=>{});
    return task;
  }
  async function restoreScenarioMap(){
    const id=activeScenario,version=++imageVersion;++yamlVersion;
    const previous=map,draft=pendingImage;pendingImage=null;
    if(draft?.image!==previous?.image)releaseMap(draft);
    mapLoading=true;projection=null;drag=null;canvas.setAttribute('aria-busy','true');
    for(const key of ['fp-image','fp-yaml','fp-map-apply','fp-map-remove'])$(key).disabled=true;
    canvas.dataset.mapState='loading';root.dataset.loadingFactory=id;
    $('fp-map-info').textContent='Loading factory map';
    if(!preparedMaps.has(id)){empty.textContent='Loading Factory '+ 'ABC'[scenarioIds.indexOf(id)]+'…';empty.hidden=false;}
    try{
      const prepared=preparedMaps.get(id)||await prepareMap(id);
      if(version!==imageVersion||id!==activeScenario)return;
      if(!prepared)throw Error('Factory map is not available.');
      map=prepared;pendingImage={image:map.image,name:map.name,file:map.file};
      $('fp-resolution').value=map.resolution;['x','y','yaw'].forEach((k,i)=>$('fp-origin-'+k).value=map.origin[i]);$('fp-frame-confirm').checked=true;
      $('fp-map-info').textContent=scenarioProfiles[id].name+' · '+map.name+' · '+map.image.width+'×'+map.image.height+'px · '+(map.bundled?'Bundled map':'Saved custom map');
      if(liveMode)refreshReceived({scenarioId:id});else filter();
      const saved=scenarioViews.get(id);
      if(saved){followRobot(false);view={...saved.view};fitted=saved.fitted;viewScenario=id;viewDataset=geometryDataset;}
      else fit();
      displayedScenario=id;canvas.dataset.displayedScenario=id;
    }catch(error){
      if(version===imageVersion){map=null;pendingImage=null;displayedScenario=id;canvas.dataset.displayedScenario=id;
        if(liveMode)refreshReceived({scenarioId:id});else filter();
        fit();$('fp-map-info').textContent=error.message;message(error.message,true);
      }
    }finally{
      if(version===imageVersion){
        mapLoading=false;canvas.setAttribute('aria-busy','false');delete root.dataset.loadingFactory;
        for(const key of ['fp-image','fp-yaml','fp-map-apply','fp-map-remove'])$(key).disabled=false;
        empty.hidden=!!map||!!run?.valid_xy||(liveMode&&fleet().some(e=>e.point));
        if(!empty.hidden)empty.textContent='Waiting for live coordinates';
        canvas.dataset.mapName=map?.name||'';canvas.dataset.mapState=map?'ready':'missing';
        canvas.dataset.mapWidth=String(map?.image.width||0);canvas.dataset.mapHeight=String(map?.image.height||0);
        releaseMap(previous);draw();
      }
    }
  }
  async function switchScenario(id){
    if(!scenarioIds.includes(id)||id===activeScenario)return;
    if(!mapLoading&&displayedScenario===activeScenario&&view.bounds)scenarioViews.set(activeScenario,{view:{...view},fitted});
    cancelPlacement();followRobot(false);activeScenario=id;syncScenarioControls();
    for(const k of fields)$('fp-'+k).value='';
    await restoreScenarioMap();
    if(activeScenario!==id)return;
    if(liveMode&&run)receivedTail();
    if($('fp-settings').open&&!liveBusy){renderGeometry();renderSymbols();}
  }

  function receivedBadge(){
    $('fp-mode').textContent='Live data';
    root.dataset.dataMode='received';
  }
  function receivedTail(){seekCommon(commonTimeline().end);}
  function refreshReceived(incoming){
    if(!receivedRuns.size){receivedBadge();filter();return;}
    const oldRun=run,oldTime=playhead,oldView={...view},oldFitted=fitted;
    runs=[...receivedRuns.values()].filter(r=>r.scenarioId===activeScenario).map(r=>{let normalized=normalizedRuns.get(r);if(!normalized){normalized=FloorplanData.normalize({schema:'edgeway.recorded-poses.v1',runs:[r]})[0];normalizedRuns.set(r,normalized);}return normalized;});
    scenes=new Map([...new Set(runs.map(r=>r.robot))].map(robot=>[robot,FloorplanData.sceneBounds(runs.filter(r=>r.robot===robot))]));
    for(const k of fields){const values=[...new Set(runs.map(r=>r[k]))],signature=JSON.stringify(values);if(fieldChoices.get(k)===signature)continue;fieldChoices.set(k,signature);const el=$('fp-'+k),value=el.value;el.replaceChildren(new Option('All',''));for(const v of values)el.add(new Option(labels[v]||v,v));el.value=[...el.options].some(o=>o.value===value)?value:'';}
    const target=liveFollow&&incoming.scenarioId===activeScenario&&oldRun?.robot===incoming.robot?incoming.id:oldRun?.id;filter(target);
    const exact=filtered.findIndex(r=>r.id===target&&r.robot===oldRun?.robot);if(exact>=0&&run!==filtered[exact]){select.value=String(exact);choose();}
    if(run?.robot===oldRun?.robot&&!oldFitted){view=oldView;fitted=false;}
    if(!liveFollow&&run){playhead=oldTime;index=FloorplanData.indexAt(run,playhead);render(false);}
    rememberEvents([...seenEvents.values()]);draw();receivedBadge();
  }
  let trailQueuedPoints=0;
  function applyReceived(event){
    const id=event.id;if(receivedDone.has(id))return;
    receivedDone.add(id);while(receivedDone.size>1000)receivedDone.delete(receivedDone.values().next().value);
    rememberEvents([event]);const incoming=FloorplanData.receivedCoordinates(event);
    if(!incoming){if(FloorplanData.factoryScenario(event.content,event.robot_id)===activeScenario)draw();return;}
    const key=JSON.stringify([incoming.source,incoming.scenarioId,incoming.robot,incoming.id]),previous=receivedRuns.get(key),merged=FloorplanData.mergeReceived(previous,incoming,id);
    receivedRuns.delete(key);receivedRuns.set(key,merged);
    while(receivedRuns.size>20){const oldest=receivedRuns.keys().next().value;receivedRuns.delete(oldest);receivedTrails.delete(oldest);}
    if(merged===previous&&incoming.points.at(-1)[0]>merged.points.at(-1)[0])return;
    // Latest pose first. Trail merges run in bounded tasks after the position frame.
    trailQueue.set(id,{incoming:{...incoming,liveOrder:merged.liveOrder},key,id});trailQueuedPoints+=incoming.points.length;
    // Bound deferred work by coordinates: 10 seconds at 60 Hz for eight robots.
    while(trailQueuedPoints>4800&&trailQueue.size>1){const oldest=trailQueue.keys().next().value;trailQueuedPoints-=trailQueue.get(oldest).incoming.points.length;trailQueue.delete(oldest);}
    if(incoming.scenarioId===activeScenario){if(merged!==previous)pendingReceived=incoming;draw();}
    else scheduleTrails();
  }
  function scheduleTrails(){if(!trailTimer&&trailQueue.size&&!renderFrame&&!pendingReceived&&!document.hidden)trailTimer=setTimeout(flushTrails,0);}
  function flushTrails(){
    trailTimer=0;if(renderFrame||pendingReceived||document.hidden)return;
    const start=performance.now();let count=0,changed=false;
    for(const [id,item] of trailQueue){
      trailQueue.delete(id);const {incoming,key}=item;trailQueuedPoints-=incoming.points.length;if(!receivedRuns.has(key))continue;
      receivedTrails.set(key,FloorplanData.expireTrail(FloorplanData.rollingTrail(receivedTrails.get(key),incoming,id)));
      changed=changed||incoming.scenarioId===activeScenario;
      if(++count>=8||performance.now()-start>=4)break;
    }
    if(changed)draw();scheduleTrails();
  }
  function queueReceived(events){
    for(const e of events){if(!Number.isSafeInteger(e?.id)||e.id<=0||receivedDone.has(e.id))continue;
      if(e.content!==undefined||e.map_content!==undefined){receivedQueue.delete(e.id);try{applyReceived(e.content!==undefined?e:{...e,content:e.map_content});}catch(error){$('fp-received-status').textContent='Coordinates not applied: '+error.message;}}
      else if(!receivedPending.has(e.id))receivedQueue.set(e.id,e);
    }
    while(receivedQueue.size>200)receivedQueue.delete(receivedQueue.keys().next().value);
    drainReceived();
  }
  async function drainReceived(){
    if(liveBusy)return;liveBusy=true;
    try{while(receivedQueue.size){
      const id=Math.max(...receivedQueue.keys()),summary=receivedQueue.get(id);receivedQueue.delete(id);if(receivedDone.has(id))continue;receivedPending.add(id);
      try{
        const response=summary.content!==undefined?null:await fetch(`/api/events/${id}`,{signal:AbortSignal.timeout(5000)});
        if(response&&!response.ok)throw Error('Could not load coordinate details');
        applyReceived(response?await response.json():summary);
      }catch(e){$('fp-received-status').textContent=`Coordinates not applied · ${e.message}`;}finally{receivedPending.delete(id);}
    }}finally{liveBusy=false;}
  }
  const symbolKey=()=>geometryKey().replace('edgeway.geometry.v1:','edgeway.symbols.v1:');
  const emptySymbols=()=>({version:1,robotImage:true,states:[],markers:[]});
  function symbols(){
    const key=symbolKey();if(!key)return emptySymbols();
    if(!symbolCache.has(key)){
      let value=emptySymbols();try{const raw=geometryPersistent?localStorage.getItem(key):null;if(raw){if(raw.length>100000)throw Error('Size exceeded');value=FloorplanData.symbolProfile(JSON.parse(raw));}}
      catch{symbolWarnings.set(key,'Could not read marker settings. Using defaults.');}
      symbolCache.set(key,value);
    }
    return symbolCache.get(key);
  }
  function badgeStyle(kind){
    if(kind==='attention')return {ink:'#c82e43',fill:'#fff0f2'};
    if(kind==='warning'||kind==='suspected'||kind==='floor_suspected')return {ink:'#a56b00',fill:'#fff4cb'};
    if(kind==='inspection')return {ink:'#c82e43',fill:'#fff0f2'};
    if(kind==='cross_check')return {ink:'#1766b5',fill:'#e9f3ff'};
    if(['fault','collision'].includes(kind))return {ink:'#c82e43',fill:'#fff0f2'};
    if(['blocked','stop'].includes(kind))return {ink:'#7540aa',fill:'#f1eafa'};
    return {ink:'#007f78',fill:'#eaf8f4'};
  }
  function annotationScale(){const panel=root.closest('.map-panel');return panel&&!panel.classList.contains('is-focused') ? .8 : 1;}
  function mapSymbolSize(kind){
    const abnormal=['attention','warning','suspected','floor_suspected','inspection','cross_check','fault','collision'].includes(kind);
    return abnormal?{diameter:36,glyph:28}:{diameter:30,glyph:24};
  }
  function badgeDimensions(kind,muted=false,robot=''){
    const label=({attention:'Attention',warning:'Suspected robot fault',suspected:'Suspected robot fault',floor_suspected:'Suspected floor anomaly',inspection:'Robot fault',cross_check:'Cross-check',loading:'Picking up',unloading:'Unloading',blocked:'Pause',stop:'Pause',pickup:'Pickup',carrying:'Carrying',dropoff:'Drop-off',fault:'Fault',collision:'Collision'})[kind]||'Event';
    const text=robot?label+' · '+robot:label;
    const scale=annotationScale(),size=mapSymbolSize(kind).diameter;return {w:size*scale,h:size*scale,text,scale};
  }
  function statusBadge(kind,box,muted=false,robot=''){
    const {ink,fill}=badgeStyle(kind),{scale}=badgeDimensions(kind,muted,robot);
    ctx.save();ctx.globalAlpha=muted ? .7 : 1;ctx.fillStyle=fill;ctx.strokeStyle=ink;ctx.lineWidth=1.4*scale;
    if(kind!=='floor_suspected'&&kind!=='attention'){ctx.setLineDash(muted?[3,2]:[]);ctx.beginPath();ctx.arc(box.x+box.w/2,box.y+box.h/2,Math.max(1,Math.min(box.w,box.h)/2-scale),0,Math.PI*2);ctx.fill();ctx.stroke();ctx.setLineDash([]);}
    incidentGlyph(kind,box.x+box.w/2,box.y+box.h/2,mapSymbolSize(kind).glyph*scale);ctx.restore();
  }
  function paintMapSymbolLegend(){
    for(const icon of document.querySelectorAll('#map-symbol-legend canvas[data-map-symbol]')){
      const target=icon.getContext('2d');if(!target)continue;
      target.setTransform(2,0,0,2,0,0);target.clearRect(0,0,38,38);
      const kind=icon.dataset.mapSymbol;
      if(kind==='zone'){
        target.fillStyle='#d9424238';target.strokeStyle='#c43c3c';target.lineWidth=1.5;
        target.fillRect(5,10,28,18);target.strokeRect(5,10,28,18);
        continue;
      }
      const style=badgeStyle(kind),size=mapSymbolSize(kind);
      target.fillStyle=style.fill;target.strokeStyle=style.ink;target.lineWidth=1.4;
      if(kind!=='floor_suspected'&&kind!=='attention'){target.beginPath();target.arc(19,19,size.diameter/2-1,0,Math.PI*2);target.fill();target.stroke();}
      // Reuse the actual map renderer so the guide cannot drift from map artwork.
      incidentGlyph(kind,19,19,size.glyph,target);
    }
  }
  function annotationPlacement(anchorX,anchorY,bw,bh,occupied){
    const {w,h}=projection,gap=4,candidates=[];
    const add=(x,y)=>candidates.push({x:Math.max(2,Math.min(w-bw-2,x-bw/2)),y:Math.max(2,Math.min(h-bh-2,y-bh/2)),w:bw,h:bh});
    const overlaps=(a,b)=>a.x<b.x+b.w+gap&&a.x+a.w+gap>b.x&&a.y<b.y+b.h+gap&&a.y+a.h+gap>b.y;
    const distance=b=>Math.hypot(b.x+bw/2-anchorX,b.y+bh/2-anchorY);
    add(anchorX,anchorY);
    for(const o of occupied){
      const left=o.x-bw/2-gap,right=o.x+o.w+bw/2+gap,top=o.y-bh/2-gap,bottom=o.y+o.h+bh/2+gap;
      add(left,anchorY);add(right,anchorY);add(anchorX,top);add(anchorX,bottom);
      for(const x of [left,right])for(const y of [top,bottom])add(x,y);
    }
    const clear=b=>!occupied.some(o=>overlaps(b,o));
    let best=candidates.filter(clear).sort((a,b)=>distance(a)-distance(b))[0];
    if(!best){
      for(let y=bh/2+2;y<=h-bh/2-2;y+=bh+gap)for(let x=bw/2+2;x<=w-bw/2-2;x+=bw+gap)add(x,y);
      best=candidates.filter(clear).sort((a,b)=>distance(a)-distance(b))[0];
    }
    return best||candidates.sort((a,b)=>occupied.filter(o=>overlaps(a,o)).length-occupied.filter(o=>overlaps(b,o)).length||distance(a)-distance(b))[0];
  }
  function incidentGlyph(kind,x,y,size,target=ctx,muted=false){
    if(kind==='loading')kind='pickup';else if(kind==='unloading')kind='dropoff';
    target.save();target.translate(x,y);target.scale(size/32,size/32);target.lineWidth=2;target.lineJoin='round';target.lineCap='round';
    target.strokeStyle=badgeStyle(kind).ink;target.fillStyle=badgeStyle(kind).fill;
    if(muted){target.strokeStyle='#728184';target.fillStyle='#edf0f1';}
    if(kind==='attention'||kind==='floor_suspected'){
      target.beginPath();target.moveTo(0,-13);target.lineTo(13,11);target.lineTo(-13,11);target.closePath();target.fill();target.stroke();
      target.beginPath();target.moveTo(0,-5);target.lineTo(0,3);target.stroke();target.beginPath();target.arc(0,7,1,0,Math.PI*2);target.fillStyle=target.strokeStyle;target.fill();
    }
    else if(kind==='cross_check'){
      target.beginPath();target.arc(-3,-3,7,0,Math.PI*2);target.moveTo(2,2);target.lineTo(10,10);target.stroke();
    }
    else if(kind==='collision'){target.beginPath();target.moveTo(-6,-6);target.lineTo(6,6);target.moveTo(6,-6);target.lineTo(-6,6);target.stroke();}
    else if(kind==='fault'||kind==='inspection'||kind==='warning'||kind==='suspected'){target.beginPath();target.moveTo(0,-7);target.lineTo(0,2);target.stroke();target.beginPath();target.arc(0,7,1.5,0,Math.PI*2);target.fillStyle=target.strokeStyle;target.fill();}
    else if(kind==='blocked'||kind==='stop'){target.beginPath();target.moveTo(-4,-6);target.lineTo(-4,6);target.moveTo(4,-6);target.lineTo(4,6);target.stroke();}
    else{target.strokeRect(-6,-3,12,10);target.beginPath();target.moveTo(-6,0);target.lineTo(6,0);target.moveTo(0,0);target.lineTo(0,7);target.stroke();
      if(kind==='pickup'||kind==='dropoff'){const sign=kind==='pickup'?-1:1;target.beginPath();target.moveTo(0,-8-sign*3);target.lineTo(0,-8+sign*3);target.moveTo(-3,-8);target.lineTo(0,-8+sign*3);target.lineTo(3,-8);target.stroke();}}
    target.restore();
  }
  function incidentPin(marker,source,X,Y,height){
    // Hide historical/manual Pause pins as well as robot state badges.
    if(marker.kind==='blocked'||marker.kind==='stop')return null;
    const x=X(marker.x,marker.y),y=Y(marker.y,marker.x);
    if(x<0||x>projection.w||y<0||y>height)return null;
    const size=badgeDimensions(marker.kind),occupied=[...robotBoxes,...incidentLayouts.map(p=>p.box).filter(Boolean)];
    const box=annotationPlacement(x,y,size.w,size.h,occupied),px=box.x+box.w/2,py=box.y+box.h/2;
    incidentLayouts.push({px,py,r:size.h/2,box});
    ctx.save();ctx.strokeStyle=badgeStyle(marker.kind).ink;ctx.fillStyle=ctx.strokeStyle;ctx.lineWidth=1.2;ctx.setLineDash([4,3]);
    ctx.beginPath();ctx.moveTo(x,y);ctx.lineTo(px,py);ctx.stroke();ctx.setLineDash([]);ctx.beginPath();ctx.arc(x,y,2.5,0,Math.PI*2);ctx.fill();ctx.restore();
    statusBadge(marker.kind,box);
    return {...marker,screenX:px,screenY:py,anchorX:x,anchorY:y,r:size.h/2,box,labelVisible:true};
  }
  const emptyGeometry=()=>({version:1,template:true,obstacles:[],body:null});
  const geometryKey=(robot=run?.robot)=>geometryDataset&&robot?`edgeway.geometry.v1:${geometryDataset}:${encodeURIComponent(robot)}${activeScenario==='scenario-1'?'':':'+activeScenario}`:'';
  function geometry(robot=run?.robot){
    const key=geometryKey(robot);if(!key)return {...emptyGeometry(),...(scenarioProfiles[activeScenario].geometry||{})};
    if(!geometryCache.has(key)){
      let value={...emptyGeometry(),body:/^site[ABC]_/.test(robot||'')?{length:3.12,width:1.14,heading:'fixed',angle:0}:null};try{const raw=geometryPersistent?localStorage.getItem(key):null;if(raw){if(raw.length>100000)throw Error('Settings size exceeded');value=FloorplanData.geometryProfile(JSON.parse(raw));}}
      catch{geometryWarnings.set(key,'Could not read saved settings. Using defaults. Please save again.');}
      geometryCache.set(key,value);
    }
    const base=geometryCache.get(key),profile=scenarioProfiles[activeScenario];return {...base,...(profile.geometry||{}),body:Object.hasOwn(profile.bodies,key)?profile.bodies[key]:base.body};
  }
  async function geometryFingerprint(next){
    // Plain HTTP on a remote host may lack Web Crypto; keep replay and session editing usable.
    if(!geometryPersistent)return `session-${++geometrySerial}`;
    const bytes=new TextEncoder().encode(JSON.stringify(next.map(r=>[r.id,r.robot,r.points])));
    const digest=await crypto.subtle.digest('SHA-256',bytes);
    return Array.from(new Uint8Array(digest),b=>b.toString(16).padStart(2,'0')).join('');
  }
  function resetMotion(){motionReset=true;arrowMotion=robotMotion=null;arrowPixels=arrowMode=arrowScale=null;}
  const background=document.createElement('canvas');
  let backgroundKey='',backgroundMap=null;
  const valid=FloorplanData.valid;
  for(const id of ['fp-prev','fp-next','fp-csv','fp-png','fp-center'])$(id).disabled=true;
  const message=(text,dialog=false)=>{$(dialog?'fp-dialog-message':'fp-feedback').textContent=text;};
  function download(blob,name){const url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);}
  function followRobot(enabled){cancelAnimationFrame(cameraFrame);cameraFrame=0;cameraClock=performance.now();follow=enabled;$('fp-follow').checked=enabled;root.dataset.follow=String(enabled);if(enabled)fitted=false;}
  function fit(){
    followRobot(false);fitted=true;viewScenario=activeScenario;viewDataset=geometryDataset;
    scene=liveMode?FloorplanData.sceneBounds(runs.filter(r=>r.scenarioId===activeScenario)):(scenes.get(run?.robot)||{x:0,y:0,extent:2});
    let bounds={x:scene.x,y:scene.y,width:scene.extent*2.8,height:scene.extent*1.9};
    if(map){
      const points=FloorplanData.mapCorners(map); // Interior fit ignores off-map telemetry.
      let x0=Infinity,x1=-Infinity,y0=Infinity,y1=-Infinity;
      for(const [x,y] of points){x0=Math.min(x0,x);x1=Math.max(x1,x);y0=Math.min(y0,y);y1=Math.max(y1,y);}
      bounds={x:(x0+x1)/2,y:(y0+y1)/2,width:Math.max(.1,x1-x0)*1.01,height:Math.max(.1,y1-y0)*1.01};
    }
    const profile=geometry(),extra=profile.obstacles.flatMap(o=>FloorplanData.rectangleCorners(o.x,o.y,o.width,o.height,o.angle));
    for(const m of symbols().markers)if(m.runId===run?.id)extra.push([m.x,m.y]);
    if(liveMode)for(const pin of visibleEventPins())extra.push([pin.x,pin.y]);
    if(profile.body){const radius=Math.hypot(profile.body.length,profile.body.width)/2;extra.push([scene.x-scene.extent/2-radius,scene.y-scene.extent/2-radius],[scene.x+scene.extent/2+radius,scene.y+scene.extent/2+radius]);}
    if(!map&&extra.length){let x0=bounds.x-bounds.width/2,x1=bounds.x+bounds.width/2,y0=bounds.y-bounds.height/2,y1=bounds.y+bounds.height/2;for(const [x,y] of extra){x0=Math.min(x0,x);x1=Math.max(x1,x);y0=Math.min(y0,y);y1=Math.max(y1,y);}bounds={x:(x0+x1)/2,y:(y0+y1)/2,width:x1-x0,height:y1-y0};}
    view={x:bounds.x,y:bounds.y,zoom:1,extent:2,bounds};draw();
  }
  function visibleEventPins(){
    if(!liveMode)return [...eventPins.values()].filter(pin=>run&&pin.runId===run.id&&pin.robotId===run.robot&&FloorplanData.incidentVisible(pin,playhead));
    const entries=fleet();
    const judgementPins=window.FloorJudgement?.markers('ABC'[scenarioIds.indexOf(activeScenario)])||[];
    const floorPins=window.FloorJudgement?.analysisMarkers('ABC'[scenarioIds.indexOf(activeScenario)])||[];
    const handoff=FloorplanData.handoffFloorAttention(floorPins,judgementPins);
    return [...eventPins.values(),...handoff.pins].flatMap(pin=>{
      if(pin.judgementZoneId||pin.analysisVerdictId)return [{...pin,robotLabel:'',startTimeBase:0}];
      const entry=entries.find(e=>e.robot===pin.robotId&&e.source===pin.source&&e.scenarioId===pin.scenarioId);
      if(!entry||!pin.floorAlert&&pin.id>entry.eventId||(Number.isFinite(pin.expiresAt)&&Date.now()>=pin.expiresAt)||(!pin.floorAlert&&entry.id===pin.runId&&!FloorplanData.incidentVisible(pin,entry.time)))return [];
      return [{...pin,robotLabel:pin.robotId,startTimeBase:0}];
    });
  }
  function eventPinRobotLabel(pin){return pin.attentionRobotIds?.length?pin.attentionRobotIds.map(robotName).join(' / '):robotName(pin.robotId);}
  function drawEventPins(X,Y,w,h){
    const canvasRect=canvas.getBoundingClientRect();
    const points=visibleEventPins().map(p=>({...p,anchorX:X(p.x,p.y),anchorY:Y(p.y,p.x)}))
      .filter(p=>p.anchorX>=0&&p.anchorX<=w&&p.anchorY>=0&&p.anchorY<=h).sort((a,b)=>a.id-b.id);
    const occupied=[...robotBoxes];
    for(const p of points){
      const size=badgeDimensions(p.kind,false,eventPinRobotLabel(p));
      const box=annotationPlacement(p.anchorX,p.anchorY,size.w,size.h,occupied);
      occupied.push(box);
      const screenX=box.x+box.w/2,screenY=box.y+box.h/2,offset=Math.hypot(screenX-p.anchorX,screenY-p.anchorY);
      const stored=eventPins.get(p.id);
      const onScreen=!document.hidden&&canvasRect.left+box.x<innerWidth&&canvasRect.left+box.x+box.w>0&&canvasRect.top+box.y<innerHeight&&canvasRect.top+box.y+box.h>0;
      if(stored&&onScreen&&!Number.isFinite(stored.firstShownAt)){
        stored.firstShownAt=Date.now();
        if(Number.isFinite(stored.expiresAt))stored.expiresAt=Math.max(stored.expiresAt,stored.firstShownAt+3000);
      }
      pinHits.push({...p,firstShownAt:stored?.firstShownAt,expiresAt:stored?.expiresAt,screenX,screenY,r:size.h/2,size:size.h,box,label:badgeDimensions(p.kind,false,eventPinRobotLabel(p)).text,fixed:true,displaced:offset>.5,offset});
    }
    // Draw leaders first, so no connector paints over another event symbol.
    for(const p of pinHits){
      if(!p.displaced)continue;
      const color=badgeStyle(p.kind).ink;
      ctx.save();ctx.globalAlpha=.75;ctx.strokeStyle=color;ctx.lineWidth=1.2;ctx.setLineDash([4,3]);
      ctx.beginPath();ctx.moveTo(p.anchorX,p.anchorY);ctx.lineTo(p.screenX,p.screenY);ctx.stroke();
      ctx.setLineDash([]);ctx.globalAlpha=1;ctx.fillStyle=color;ctx.beginPath();ctx.arc(p.anchorX,p.anchorY,2.5,0,Math.PI*2);ctx.fill();ctx.restore();
    }
    for(const p of pinHits){
      statusBadge(p.kind,p.box,false,eventPinRobotLabel(p));
    }
    canvas.dataset.eventPins=JSON.stringify(pinHits);
    canvas.dataset.robotBoxes=JSON.stringify(robotBoxes);
  }
  function draw(){
    if(renderingFrame||renderFrame||document.hidden)return;
    renderFrame=requestAnimationFrame(()=>{renderFrame=0;renderingFrame=true;
      try{const incoming=pendingReceived;pendingReceived=null;if(incoming)refreshReceived(incoming);paint();}
      finally{renderingFrame=false;scheduleTrails();}
    });
  }
  function paint(){drawing=true;frameFleet=null;try{paintBody();}finally{drawing=false;frameFleet=null;}}
  function paintBody(){
    if(mapLoading){empty.hidden=false;empty.textContent='Loading Factory '+'ABC'[scenarioIds.indexOf(activeScenario)]+'…';return;} // Keep the previous canvas until ready.
    const rect=canvas.getBoundingClientRect();if(!rect.width||!rect.height)return;
    const ratio=window.devicePixelRatio||1,w=rect.width,h=rect.height;
    const pixelWidth=Math.round(w*ratio),pixelHeight=Math.round(h*ratio);
    if(canvas.width!==pixelWidth||canvas.height!==pixelHeight){canvas.width=pixelWidth;canvas.height=pixelHeight;}
    ctx.setTransform(ratio,0,0,ratio,0,0);
    const display=shownPose();
    const css=getComputedStyle(root),ink=css.getPropertyValue('--muted').trim(),green=css.getPropertyValue('--accent').trim();
    const pad=map?10:(h<110?12:28),aw=Math.max(1,w-2*pad),ah=Math.max(1,h-2*pad);
    // Display rotation only: preserve world pose, YAML origin and metric body dimensions.
    const rotation=map&&(map.contentBounds?.h||map.image.height)>(map.contentBounds?.w||map.image.width)?Math.PI/2:0,c=Math.cos(rotation),sn=Math.sin(rotation);
    const bw=view.bounds?Math.abs(c)*view.bounds.width+Math.abs(sn)*view.bounds.height:1,bh=view.bounds?Math.abs(sn)*view.bounds.width+Math.abs(c)*view.bounds.height:1;
    if(fitted&&view.bounds)view.extent=Math.min(aw,ah)/Math.min(aw/bw,ah/bh);
    const s=Math.max(.00000001,Math.min(aw,ah)/view.extent*view.zoom);
    const cameraNow=performance.now(),cameraDt=Math.min(.05,Math.max(0,(cameraNow-cameraClock)/1000));cameraClock=cameraNow;
    if(follow&&display.point){
      const next=FloorplanData.followView(view,[display.point[1],display.point[2]],s,rotation?h:w,rotation?w:h,cameraDt);
      view.x=next.x;view.y=next.y;canvas.dataset.cameraSettled=String(next.settled);
      if(!next.settled&&!playing&&!cameraFrame)cameraFrame=requestAnimationFrame(()=>{cameraFrame=0;draw();});
    }else canvas.dataset.cameraSettled='true';
    const X=(x,y=view.y)=>w/2+((x-view.x)*c-(y-view.y)*sn)*s;
    const Y=(y,x=view.x)=>h/2-((x-view.x)*sn+(y-view.y)*c)*s;
    const delta=(dx,dy,scale=s)=>({x:(c*dx-sn*dy)/scale,y:(-sn*dx-c*dy)/scale});
    const world=(x,y)=>{const d=delta(x-w/2,y-h/2);return {x:view.x+d.x,y:view.y+d.y};};
    projection={s,w,h,X,Y,rotation,delta,world};canvas.dataset.rotation=String(rotation);canvas.dataset.scale=String(s);pinHits=[];canvas.dataset.zoom=view.zoom.toFixed(2);$('fp-zoom-range').value=String(Math.log2(view.zoom));$('fp-zoom-value').textContent=`${Math.round(view.zoom*100)}%`;$('fp-zoom-range').setAttribute('aria-valuetext',`${Math.round(view.zoom*100)}%`);canvas.dataset.scene=JSON.stringify(scene);canvas.dataset.viewX=String(view.x);canvas.dataset.viewY=String(view.y);robotHit=null;$('fp-return').hidden=true;
    const profile=geometry();stopHits=[];canvas.dataset.stopMarkers='[]';symbolHits=[];incidentLayouts.length=0;canvas.dataset.manualMarkers='[]';canvas.dataset.robotImage='false';canvas.dataset.manualState='robot';canvas.dataset.obstacles=JSON.stringify(profile.obstacles);canvas.dataset.body='';canvas.dataset.directionVisible='false';
    const mapLabel=$('fp-scene-label'),factory=`Factory ${'ABC'[scenarioIds.indexOf(activeScenario)]}`;
    mapLabel.dataset.applied=String(!!map);
    mapLabel.textContent=map?(map.contentBounds.w*map.resolution).toFixed(2)+' × '+(map.contentBounds.h*map.resolution).toFixed(2)+' m':factory+' · Map not applied';
    mapLabel.setAttribute('aria-label',map?factory+' Map applied · Extent '+mapLabel.textContent:mapLabel.textContent);
    mapLabel.title=map?`${map.name} · Displayed interior extent; original YAML coordinates retained.`:$('fp-map-info').textContent;
    canvas.dataset.mapContentBounds=JSON.stringify(map?.contentBounds||null);

    const key=[w,h,ratio,view.x,view.y,view.extent,view.zoom,scene?.x,scene?.y,scene?.extent,$('fp-grid').checked,$('fp-coordinates').checked,ink,green,css.getPropertyValue('--surface'),JSON.stringify(profile)].join('|');
    if(key===backgroundKey&&map===backgroundMap){ctx.drawImage(background,0,0,w,h);}
    else {
    ctx.fillStyle=map?'#edf1f2':'#f5f9fc';ctx.fillRect(0,0,w,h);
    if(!map&&scene&&profile.template){
      const e=scene.extent,aspect=1.5,rw=e*.43*aspect,rh=e*.19;
      ctx.lineWidth=1;
      for(const row of [-1,1])for(const col of [-1,0,1]){
        const x=X(scene.x+col*e*.59*aspect-rw/2),y=Y(scene.y+row*e*.65+rh/2),sw=rw*s,sh=rh*s;
        ctx.fillStyle='#e6eef4';ctx.strokeStyle='#cfdee8';ctx.beginPath();ctx.roundRect(x,y,sw,sh,Math.min(4,sh/5));ctx.fill();ctx.stroke();
        ctx.beginPath();ctx.moveTo(x+sw/3,y+5);ctx.lineTo(x+sw/3,y+sh-5);ctx.moveTo(x+sw*2/3,y+5);ctx.lineTo(x+sw*2/3,y+sh-5);ctx.stroke();
        if(sh>24&&sw>55){ctx.fillStyle='#738a9b';ctx.font='11px Pretendard, sans-serif';ctx.textAlign='center';ctx.fillText('Storage area',x+sw/2,y+sh/2+4);}
      }
    }
    if(map){
      const a=map.origin[2]+rotation,r=map.resolution,top=FloorplanData.mapPoint(0,0,map.image.height,map);
      ctx.save();ctx.transform(s*r*Math.cos(a),-s*r*Math.sin(a),s*r*Math.sin(a),s*r*Math.cos(a),X(top[0],top[1]),Y(top[1],top[0]));
      const b=map.contentBounds;ctx.globalAlpha=1;
      ctx.drawImage(map.image,b.x,b.y,b.w,b.h,b.x,b.y,b.w,b.h);ctx.restore();
    }
    ctx.font='11px Pretendard, sans-serif';ctx.fillStyle=ink;
    if($('fp-grid').checked&&!map){
      const raw=($('fp-coordinates').checked?52:24)/s,power=Math.pow(10,Math.floor(Math.log10(raw))),stride=power*([1,2,5,10].find(n=>n*power>=raw)||10);
      const minX=view.x-w/(2*s),maxX=view.x+w/(2*s),minY=view.y-h/(2*s),maxY=view.y+h/(2*s);
      ctx.strokeStyle='#e5eef4';ctx.lineWidth=.6;
      for(let x=Math.ceil(minX/stride)*stride,count=0;x<=maxX&&count++<200;x+=stride){ctx.beginPath();ctx.moveTo(X(x),pad);ctx.lineTo(X(x),h-pad);ctx.stroke();if($('fp-coordinates').checked&&h>=110&&X(x)>25&&X(x)<w-25){ctx.textAlign='center';ctx.fillText(Number(x.toFixed(3)),X(x),h-8);}}
      for(let y=Math.ceil(minY/stride)*stride,count=0;y<=maxY&&count++<200;y+=stride){ctx.beginPath();ctx.moveTo(pad,Y(y));ctx.lineTo(w-pad,Y(y));ctx.stroke();if($('fp-coordinates').checked&&h>=110&&Y(y)>25&&Y(y)<h-25){ctx.textAlign='left';ctx.fillText(Number(y.toFixed(3)),2,Y(y)+4);}}
    }
    if($('fp-coordinates').checked){ctx.textAlign='right';ctx.fillText(rotation?'X ↑':'X →',w-5,h<110?12:h-8);ctx.textAlign='left';ctx.fillText(rotation?'Y ←':'Y ↑',5,12);}
    for(const o of profile.obstacles){
      const corners=FloorplanData.rectangleCorners(o.x,o.y,o.width,o.height,o.angle);
      ctx.save();ctx.beginPath();corners.forEach(([x,y],i)=>i?ctx.lineTo(X(x,y),Y(y,x)):ctx.moveTo(X(x,y),Y(y,x)));ctx.closePath();
      ctx.fillStyle=o.kind==='wall'?'#a4b0b9bb':o.kind==='shelf'?'#c5d9e4cc':'#edcf92aa';ctx.strokeStyle=o.kind==='obstacle'?'#9a650a':'#526e80';ctx.lineWidth=1.5;ctx.fill();ctx.stroke();
      if(h>=180){ctx.fillStyle='#29434f';ctx.font='11px Pretendard, sans-serif';ctx.textAlign='center';ctx.fillText(o.name,X(o.x,o.y),Y(o.y,o.x)+4,Math.max(30,o.width*s));}ctx.restore();
    }
    background.width=canvas.width;background.height=canvas.height;background.getContext('2d').drawImage(canvas,0,0);
    backgroundKey=key;backgroundMap=map;
    }
    // Outline the displayed interior rectangle, retaining original world coordinates.
    canvas.dataset.mapBoundary='[]';
    if(map){
      const corners=FloorplanData.mapCorners(map);
      const screen=corners.map(([x,y])=>[X(x,y),Y(y,x)]);canvas.dataset.mapBoundary=JSON.stringify(screen);
      ctx.save();ctx.beginPath();screen.forEach(([x,y],i)=>i?ctx.lineTo(x,y):ctx.moveTo(x,y));ctx.closePath();
      ctx.lineJoin='round';ctx.strokeStyle='#ffffff';ctx.lineWidth=4;ctx.stroke();ctx.strokeStyle='#087f79';ctx.lineWidth=2;ctx.stroke();

      ctx.restore();
    }
    window.FloorJudgement?.draw({ctx,canvas,X,Y,scale:s,factory:'ABC'[scenarioIds.indexOf(activeScenario)]});
    // Received positions are dots, never interpolated or connected into a path.
    const trailInfo=[];
    if(liveMode&&$('fp-trail').checked)for(const e of fleet()){
      const key=JSON.stringify([e.source,e.scenarioId,e.robot,e.id]);
      const trail=FloorplanData.expireTrail(receivedTrails.get(key));
      if(!trail)continue;receivedTrails.set(key,trail);if(!trail.points.length)continue;
      const points=trail.points;let drawn=0,last=null;
      ctx.save();ctx.fillStyle=e.color;ctx.globalAlpha=.8;ctx.beginPath();
      for(let i=points.length-1;i>=0;i--){
        const p=points[i];if(!valid(p))continue;const x=X(p[1],p[2]),y=Y(p[2],p[1]);
        // Retain every sample in the buffer; skip overlapping dots only on screen.
        if(last&&Math.hypot(x-last.x,y-last.y)<6)continue;
        ctx.moveTo(x+1.8,y);ctx.arc(x,y,1.8,0,Math.PI*2);last={x,y};drawn++;
      }
      ctx.fill();ctx.restore();
      trailInfo.push({robot:e.robot,count:points.length,drawn,start:points[0][0],end:points.at(-1)[0],mode:'dots',color:e.color});
    }
    canvas.dataset.trails=JSON.stringify(trailInfo);
    // Reserve every metric body before positioning labels, including robots drawn later.
    robotBoxes=fleet().filter(e=>e.point).flatMap(e=>{
      const b=geometry(e.robot).body;if(!b)return [];
      const d=Math.hypot(b.length,b.width)*s,x=X(e.point[1],e.point[2]),y=Y(e.point[2],e.point[1]);
      return [{x:x-d/2-3,y:y-d/2-3,w:d+6,h:d+6}];
    });drawFleet(X,Y,h);
    if(!run){drawEventPins(X,Y,w,h);resetMotion();canvas.dataset.displayPrediction='';canvas.dataset.arrowAngle='';canvas.dataset.arrowOpacity='0';$('fp-prediction-label').hidden=true;canvas.dataset.prediction='';canvas.dataset.predictionMode='';delete canvas.dataset.markerX;delete canvas.dataset.markerY;canvas.dataset.displayState='unavailable';$('fp-motion-state').textContent='';syncCompact('unavailable');return;}
    canvas.dataset.displayState=display.state;canvas.dataset.playhead=String(playhead);
    const stateLabel={observed:'Received position',interpolated:'Interpolated between samples',held:`Last confirmed position · ${display.age.toFixed(1)} s gap`,preview:'First observed position preview · No coordinates yet',unavailable:'No position records'}[display.state];
    if($('fp-motion-state').textContent!==stateLabel)$('fp-motion-state').textContent=stateLabel;
    if($('fp-motion-state').dataset.state!==display.state)$('fp-motion-state').dataset.state=display.state;
    if(!liveMode&&$('fp-trail').checked){
      const strokeTrail=(seconds,color,width)=>{
        const segments=FloorplanData.trailSegments(run,index,playhead,seconds);
        const last=segments.at(-1);
        if(display.state==='interpolated'&&last?.at(-1)===run.points[index])last.push(display.point);
        ctx.beginPath();
        for(const segment of segments)for(let i=0;i<segment.length;i++){const p=segment[i];if(i===0)ctx.moveTo(X(p[1],p[2]),Y(p[2],p[1]));else ctx.lineTo(X(p[1],p[2]),Y(p[2],p[1]));}
        ctx.strokeStyle=color;ctx.lineWidth=width;ctx.lineJoin='round';ctx.lineCap='round';ctx.stroke();
      };
      // Keep the entire observed trail visible at a constant contrast; never age it out.
      strokeTrail(Infinity,'#008b89',h<180?2.5:3);
    }
    const currentFleet=liveMode?fleet().find(e=>e.robot===run.robot&&e.source===run.source):null,positionFresh=!liveMode||currentFleet?.state==='fresh';
    const rawPrediction=!liveMode&&$('fp-prediction').checked&&positionFresh?FloorplanData.predictedPath(run,index,display):null;
    const rawAngle=FloorplanData.movementAngle(run,index,display),now=performance.now();
    let discontinuity=motionReset||motionRun!==run||playhead<motionTime;
    if(!discontinuity)for(let i=motionIndex+1;i<=index;i++){
      if(!valid(run.points[i])||(i>0&&(!valid(run.points[i-1])||run.points[i][0]-run.points[i-1][0]>.25))){discontinuity=true;break;}
    }
    const observed=['observed','interpolated'].includes(display.state)&&positionFresh;
    if(!observed){arrowMotion=robotMotion=null;arrowPixels=arrowMode=arrowScale=null;}
    const dt=discontinuity?Infinity:playing?Math.min(.1,Math.max(0,(now-motionClock)/1000)):0;
    const vector=rawPrediction?[rawPrediction.end[0]-rawPrediction.start[0],rawPrediction.end[1]-rawPrediction.start[1]]:null;
    if(observed){
      arrowMotion=FloorplanData.smoothArrow(arrowMotion,vector?{angle:Math.atan2(vector[1],vector[0]),length:Math.hypot(...vector)}:null,dt);
      robotMotion=FloorplanData.smoothArrow(robotMotion,rawAngle===null?null:{angle:rawAngle,length:1},dt);
    }
    motionRun=run;motionIndex=index;motionTime=playhead;motionClock=now;motionReset=false;
    const prediction=arrowMotion&&display.point?{start:[display.point[1],display.point[2]],
      end:[display.point[1]+Math.cos(arrowMotion.angle)*arrowMotion.length,display.point[2]+Math.sin(arrowMotion.angle)*arrowMotion.length],seconds:3}:null;
    const radius=h<180?9:14,physicalLength=arrowMotion?arrowMotion.length*s:0;
    // Separate enter/exit thresholds prevent mode flicker near the marker boundary.
    if(!prediction){arrowMode=null;arrowPixels=null;}
    else {
      if(discontinuity||arrowMode===null)arrowMode=physicalLength<radius+16?'direction':'path';
      else if(arrowMode==='direction'&&physicalLength>radius+24)arrowMode='path';
      else if(arrowMode==='path'&&physicalLength<radius+12)arrowMode='direction';
      const targetPixels=arrowMode==='direction'?radius+29:physicalLength;
      const blend=discontinuity||(!playing&&s!==arrowScale)?1:1-Math.exp(-dt/.16);
      arrowPixels=arrowPixels===null?targetPixels:arrowPixels+(targetPixels-arrowPixels)*blend;arrowScale=s;
    }
    const directional=arrowMode==='direction'||(prediction&&Math.abs(arrowPixels-physicalLength)>.75);
    canvas.dataset.prediction=rawPrediction?JSON.stringify(rawPrediction):'';
    canvas.dataset.displayPrediction=prediction?JSON.stringify(prediction):'';
    canvas.dataset.predictionMode=prediction?(directional?'direction':'path'):'';
    canvas.dataset.arrowAngle=arrowMotion?String(arrowMotion.angle):'';
    canvas.dataset.arrowOpacity=String(arrowMotion?.opacity||0);canvas.dataset.arrowPixels=String(arrowPixels||0);
    $('fp-prediction-label').hidden=liveMode||!$('fp-prediction').checked;
    $('fp-prediction-label').textContent=prediction?(directional?'Red arrow: estimated direction · Zoom in to see the path':'Red dashed line: 3-second motion estimate · Not a planned route'):'Waiting for motion samples';
    if(prediction){
      const x=X(prediction.start[0],prediction.start[1]),y=Y(prediction.start[1],prediction.start[0]);
      ctx.save();ctx.translate(x,y);ctx.rotate(-arrowMotion.angle-rotation);ctx.globalAlpha=arrowMotion.opacity;
      ctx.strokeStyle='#d64141';ctx.fillStyle='#d64141';ctx.lineWidth=2.5;ctx.setLineDash([6,4]);
      ctx.beginPath();ctx.moveTo(radius+2,0);ctx.lineTo(arrowPixels,0);ctx.stroke();ctx.setLineDash([]);
      ctx.beginPath();ctx.moveTo(arrowPixels,0);ctx.lineTo(arrowPixels-9,-5);ctx.lineTo(arrowPixels-9,5);ctx.closePath();ctx.fill();ctx.restore();
    }
    const first=run.points[history.first],p=run.points[index],shown=display.point;
    if(!liveMode&&first&&history.first<=index){ctx.beginPath();ctx.arc(X(first[1],first[2]),Y(first[2],first[1]),4,0,Math.PI*2);ctx.fillStyle='#f5f9fc';ctx.fill();ctx.strokeStyle=green;ctx.lineWidth=1.5;ctx.stroke();}
    if(shown){
      const x=X(shown[1],shown[2]),y=Y(shown[2],shown[1]),radius=h<180?9:14,chosen=selectedRobot===run.robot,ghost=['held','preview'].includes(display.state);
      canvas.dataset.markerX=String(shown[1]);canvas.dataset.markerY=String(shown[2]);canvas.dataset.screenX=String(x);canvas.dataset.screenY=String(y);
      if(x<radius||x>w-radius||y<radius+28||y>h-radius-48){
        const direction=Math.atan2(h/2-y,x-w/2),arrows=['→','↗','↑','↖','←','↙','↓','↘'];
        $('fp-return').textContent=arrows[(Math.round(direction/(Math.PI/4))+8)%8]+' Return to robot';$('fp-return').hidden=false;
      }
      ctx.save();const fleetEntry=liveMode?fleet().find(e=>e.robot===run.robot&&e.id===run.id):null;
      const footprint=robotBadge(run.robot,x,y,fleetEntry?.color||'#246fd1',chosen,fleetEntry);
      if(footprint)canvas.dataset.body=JSON.stringify({...footprint,x:shown[1],y:shown[2],state:display.state});
      const angle=robotMotion?.angle??null;
      canvas.dataset.movementAngle=angle===null?'':String(angle);
      canvas.dataset.movementOpacity=String(robotMotion?.opacity||0);
      canvas.dataset.directionVisible='false';
      robotHit={x,y,r:10,footprint};
      const selectedHit=fleetHits.find(e=>e.robot===run.robot&&e.source===run.source);if(selectedHit)selectedHit.footprint=footprint;
      canvas.dataset.fleetHits=JSON.stringify(fleetHits);
      ctx.restore();
    }else{for(const key of ['markerX','markerY','screenX','screenY','movementAngle'])delete canvas.dataset[key];}
    // Event callouts avoid robot annotations; leader dots retain exact event coordinates.
    drawEventPins(X,Y,w,h);
    for(const m of symbols().markers){
      if(m.runId!==run.id||!FloorplanData.incidentVisible(m,playhead))continue;
      const pin=incidentPin(m,'Manual',X,Y,h);if(pin)symbolHits.push(pin);
    }
    canvas.dataset.manualMarkers=JSON.stringify(symbolHits);
    if($('fp-stops').checked)for(const marker of stopIntervals){
      if(playhead<marker.time)continue;
      const duration=Math.max(0,Math.min(playhead,marker.endTime)-marker.startTime);
      const rendered=incidentPin({...marker,name:`Stationary position ${duration.toFixed(1)} s`},'Estimated position',X,Y,h);
      if(rendered)stopHits.push({...rendered,duration});
    }
    canvas.dataset.stopMarkers=JSON.stringify(stopHits);syncCompact(display.state);
    canvas.dataset.positionValid=String(liveMode?!!shown&&valid(shown):valid(p));canvas.setAttribute('aria-label',(FloorplanData.reportedStopLabel(currentFleet?.robotState)?'Received robot state '+FloorplanData.reportedStopLabel(currentFleet.robotState)+(currentFleet.state==='fresh'?'':' (Earlier data)')+'. ':'')+(shown?`${stateLabel}. X ${shown[1]} Y ${shown[2]}. Drag or use arrow keys to pan`:'No position for the current time'));
  }
  function syncCompact(state){
    if(liveMode){
      const entries=fleet(),entry=entries.find(e=>e.robot===run?.robot&&e.id===run?.id&&e.source===run?.source);
      if(entry){
        const age=FloorplanData.receiptLabel(entry.receivedAt);
        $('floorplan-readout').textContent=`${entries.length} robots · ${robotName(entry.robot)} · ${age}`;
        $('floorplan-readout').title='Time since the dashboard received this record; not network latency.';
      }
    }
    const issue=['held','preview','unavailable'].includes(state)?state:linkIssue?'link':'';
    if(issue&&issue!==lastIssue){root.classList.add('hud-open');autoHud=true;}
    else if(!issue&&lastIssue&&autoHud){root.classList.remove('hud-open');autoHud=false;}
    lastIssue=issue;root.classList.toggle('has-map-issue',!!issue);
    const compact=$('fp-compact-status');compact.textContent=issue?(linkIssue?'Could not load event':'Position needs verification'):(liveMode?'Received position':'Received position');
    compact.setAttribute('aria-expanded',String(root.classList.contains('hud-open')));
    compact.title=['Blue ring: current robot · Badge: state · Dashed line endpoint: event location · Select an event icon for details (ordinary events: 10 s, at least 3 s visible; faults and collisions retained)',$('fp-motion-state').textContent,$('fp-prediction-label').textContent,$('fp-link-status').textContent].filter(Boolean).join(' / ');
  }
  function configureTimeline(){root.dataset.sampleCount=commonTimeline().times.length;}
  function rememberEvents(events){
    for(const event of events){if(Number.isSafeInteger(event?.id)&&event.id>0){seenEvents.delete(event.id);seenEvents.set(event.id,event);}const pin=FloorplanData.eventLocation(event,runs);if(!pin||eventPins.has(pin.id))continue;
      const received=Date.parse(event.received_at);
      pin.expiresAt=['blocked','pickup','carrying','dropoff'].includes(pin.kind)?(Number.isFinite(received)?received:Date.now())+10000:null;
      eventPins.set(pin.id,pin);}
    while(seenEvents.size>100)seenEvents.delete(seenEvents.keys().next().value);
    while(eventPins.size>50)eventPins.delete(eventPins.keys().next().value);
  }
  function render(snap=true){
    if(!run){$('fp-robot-detail').textContent='No robot selected.';$('floorplan-readout').textContent='No robots received';draw();return;}
    if(liveMode)index=FloorplanData.indexAt(run,playhead);
    const p=run.points[index],shown=shownPose(),q=shown.point;
    root.dataset.commonTime=String(playhead);
    const kind={observed:liveMode?(liveFollow?'Latest received position':'Recorded position at selected time'):'Received position',interpolated:'Displayed position · Interpolated',held:'No position received · Last confirmed',preview:'No position received · First position preview',unavailable:'No position received'}[shown.state];
    $('floorplan-readout').textContent=`${run.robot} · ${kind}`+(q?` · X ${q[1].toFixed(3)} · Y ${q[2].toFixed(3)}`:'');
    $('floorplan-readout').title=`Original t_sim ${p[0]} · X ${p[1]??'Not received'} · Y ${p[2]??'Not received'} · yaw ${p[3]??'Not received'}`;
    empty.hidden=run.valid_xy>0||!!map||(liveMode&&fleet().some(e=>e.point));empty.textContent=`${run.robot} · No valid positions in this trial.`;
    root.dataset.runId=run.id;root.dataset.sampleIndex=index;
    $('fp-robot-link').textContent=`${run.robot} · View events`;$('fp-robot-link').disabled=!/^[A-Za-z0-9_-]{1,180}$/.test(run.robot);
    $('fp-robot-link').setAttribute('aria-pressed',String(selectedRobot===run.robot));
    $('fp-center').disabled=!q;$('fp-follow').disabled=!q;
    draw();if(liveMode){const entry=fleet().find(e=>e.robot===run.robot&&e.id===run.id);$('fp-robot-detail').textContent=run.robot+' · Received state: '+(robotStateLabel(entry?.robotState))+(q?' · X '+q[1].toFixed(3)+' · Y '+q[2].toFixed(3):'');$('floorplan-readout').textContent=fleet().length+' robots · '+robotName(run.robot)+(entry&&entry.state!=='fresh'?' · Delayed':'');$('fp-source').textContent=''; }
  }
  function choose(){
    const previousRobot=run?.robot;cancelPlacement();resetMotion();run=filtered[Number(select.value)]||null;stopIntervals=run?FloorplanData.stationaryIntervals(run):[];index=0;
    const preserveView=liveMode?!!view.bounds&&viewScenario===activeScenario&&viewDataset===geometryDataset:run&&previousRobot===run.robot&&scene===scenes.get(run.robot);
    if(!selectionSync){selectedRobot=run?.robot||null;eventSelected=null;$('fp-link-status').textContent=liveMode?'All robot positions and events · Select for details':'Recorded position · Select a robot to view related events';root.dataset.selectedRobot=selectedRobot||'';delete root.dataset.linkedEvent;}
    for(const id of ['fp-csv','fp-png','fp-center','fp-follow'])$(id).disabled=!run;
    $('fp-prev').disabled=!run||Number(select.value)===0;$('fp-next').disabled=!run||Number(select.value)===filtered.length-1;
    if(!run){$('fp-robot-link').disabled=true;empty.hidden=!!map||(liveMode&&fleet().some(e=>e.point));empty.textContent=receivedRuns.size?'No robots match this background and filters.':'Waiting for live coordinates';$('floorplan-readout').textContent='No coordinates received';$('fp-summary').textContent=`0 / ${runs.length} trials`;delete root.dataset.runId;canvas.dataset.positionValid='false';fit();return;}
    history=FloorplanData.motionIndex(run);if(liveMode){const clock=commonTimeline();playhead=liveFollow?clock.end:Math.max(clock.start,Math.min(clock.end,playhead));index=FloorplanData.indexAt(run,playhead);}else{index=Math.max(0,history.first);playhead=run.points[index][0];}
    breaks=new Set(run.breaks);configureTimeline();
    $('fp-summary').textContent=`${filtered.length} / ${runs.length} trials · Valid coordinates ${(100*run.valid_xy/run.points.length).toFixed(1)}% · Missing ${run.points.length-run.valid_xy} samples · Observed path length ${run.distance.toFixed(2)} (original units)`;
    if(!preserveView)fit();render();if($('fp-settings').open&&!liveBusy){renderGeometry();renderSymbols();}
  }
  function filter(prefer=null){
    const old=prefer||run?.id,latest=liveMode?fleet():[];filtered=runs.filter(r=>(!liveMode||latest.some(e=>e.id===r.id&&e.robot===r.robot&&e.source===r.source))&&(!liveMode||r.scenarioId===activeScenario)&&(!r.scenarioId||r.scenarioId===activeScenario)&&fields.every(k=>!$('fp-'+k).value||r[k]===$('fp-'+k).value));select.replaceChildren();
    filtered.forEach((r,i)=>{const o=document.createElement('option');o.value=i;o.textContent=liveMode?`Details: ${r.robot} · ${r.id}`:`${r.cond} / ${labels[r.load]||r.load} / ${labels[r.spd]||r.spd} / ${String(r.order).padStart(2,'0')} ${labels[r.scn]||r.scn}`;select.append(o);});
    select.disabled=!filtered.length;select.value=Math.max(0,filtered.findIndex(r=>r.id===old));choose();
  }
  select.onchange=choose;
  for(const k of fields)$('fp-'+k).onchange=()=>filter();
  $('fp-filter-reset').onclick=()=>{for(const k of fields)$('fp-'+k).value='';filter();};
  for(const [id,delta] of [['fp-prev',-1],['fp-next',1]])$(id).onclick=()=>{select.value=Math.max(0,Math.min(filtered.length-1,Number(select.value)+delta));choose();};
  function zoom(factor,pointer=null){
    if(!projection)return;
    const shown=shownPose().point;
    let anchor=pointer||{x:projection.w/2,y:projection.h/2};
    if(follow&&shown)anchor={x:projection.X(shown[1],shown[2]),y:projection.Y(shown[2],shown[1])};
    const anchorWorld=projection.world(anchor.x,anchor.y);
    const next=Math.max(.25,Math.min(8,view.zoom*factor)),scale=projection.s*next/view.zoom;
    const offset=projection.delta(anchor.x-projection.w/2,anchor.y-projection.h/2,scale);fitted=false;view.zoom=next;view.x=anchorWorld.x-offset.x;view.y=anchorWorld.y-offset.y;
    cameraClock=performance.now();draw();
  }
  $('fp-zoom-range').oninput=e=>zoom(2**Number(e.target.value)/view.zoom);
  $('fp-zoom-in').onclick=()=>zoom(1.5);$('fp-zoom-out').onclick=()=>zoom(1/1.5);$('fp-fit').onclick=fit;
  function centerRobot(){const p=shownPose().point;if(p){fitted=false;view.x=p[1];view.y=p[2];draw();}}
  $('fp-center').onclick=centerRobot;$('fp-return').onclick=centerRobot;
  $('fp-follow').onchange=()=>{followRobot($('fp-follow').checked);draw();};
  $('fp-mobile-tools').onclick=()=>{const open=root.classList.toggle('tools-open');$('fp-mobile-tools').setAttribute('aria-expanded',String(open));$('fp-mobile-tools').textContent=open?'Hide tools':'Filters & tools';};
  $('fp-compact-status').onclick=()=>{if(!root.closest('.is-focused')){document.querySelector('[data-focus-target="map"]').click();return;}autoHud=false;root.classList.toggle('hud-open');syncCompact(canvas.dataset.displayState);};
  $('fp-stops').onchange=draw;$('fp-grid').onchange=draw;$('fp-trail').onchange=draw;$('fp-coordinates').onchange=draw;$('fp-prediction').onchange=()=>{resetMotion();draw();};
  function requestRobotEvents(){
    if(!run||!/^[A-Za-z0-9_-]{1,180}$/.test(run.robot))return;
    $('fp-robot-detail').parentElement.open=true;
    selectedRobot=run.robot;eventSelected=null;root.dataset.selectedRobot=run.robot;delete root.dataset.linkedEvent;
    $('fp-link-status').textContent=`${run.robot} · Loading related events`;
    draw();$('fp-robot-link').setAttribute('aria-pressed','true');
    window.dispatchEvent(new CustomEvent('edgeway:robot-request',{detail:{robotId:run.robot}}));
  }
  $('fp-robot-link').onclick=requestRobotEvents;
  function connectEvent(event){
    let content=event?.content;try{if(typeof content==='string')content=JSON.parse(content);}catch{content=null;}
    const eventScenario=FloorplanData.factoryScenario(content,event.robot_id);
    if(eventScenario&&eventScenario!==activeScenario){$('fp-link-status').textContent=`Event from another scenario · ${scenarioProfiles[eventScenario]?.name||eventScenario} : view this background.`;return;}
    eventSelected=event;linkIssue=false;rememberEvents([event]);
    root.dataset.linkedEvent=String(event.id);
    $('fp-link-status').textContent='Event details · Latest received position retained';
    draw();
  }
  window.addEventListener('edgeway:selection',({detail})=>{
    if(detail.event)connectEvent(detail.event);
    else {linkIssue=!!detail.error;eventSelected=null;delete root.dataset.linkedEvent;delete root.dataset.timeLinked;
      if(detail.error)$('fp-link-status').textContent='Could not load event details · Select again.';
      else if(detail.loading)$('fp-link-status').textContent='Loading event details';
      syncCompact(canvas.dataset.displayState);
    }
  });
  window.addEventListener('edgeway:events-loaded',({detail})=>{
    if(Array.isArray(detail.events)){queueReceived(detail.events);}
    if(detail.error&&(!detail.robotId||detail.robotId===selectedRobot||detail.robotId===run?.robot)){linkIssue=true;$('fp-link-status').textContent='Could not load event · Try again.';syncCompact(canvas.dataset.displayState);return;}
    if(detail.robotId!==selectedRobot||!detail.robotId)return;
    linkIssue=!!detail.error;
    $('fp-link-status').textContent=detail.error?`${detail.robotId} · Could not load event`:`${detail.robotId} · Related events ${detail.total} records${detail.total?' · Select in the log.':' · Matching robot ID has no records.'}`;syncCompact(canvas.dataset.displayState);
  });
  window.addEventListener('edgeway:telemetry-received',({detail})=>{if(detail?.event)queueReceived([detail.event]);});
  window.addEventListener('edgeway:event-records',({detail})=>{if(Array.isArray(detail?.events))queueReceived(detail.events);});
  canvas.onpointerdown=e=>{if(e.button!==0||!projection)return;drag={x:e.clientX,y:e.clientY,startX:e.clientX,startY:e.clientY,moved:false};canvas.setPointerCapture(e.pointerId);};
  canvas.onpointermove=e=>{if(!projection)return;if(drag){
    if(Math.hypot(e.clientX-drag.startX,e.clientY-drag.startY)>4)drag.moved=true;
    if(drag.moved){followRobot(false);fitted=false;const d=projection.delta(e.clientX-drag.x,e.clientY-drag.y);view.x-=d.x;view.y-=d.y;draw();}
    drag.x=e.clientX;drag.y=e.clientY;
  }const rect=canvas.getBoundingClientRect(),point=projection.world(e.clientX-rect.left,e.clientY-rect.top);$('fp-pointer').textContent='X '+point.x.toFixed(2)+' · Y '+point.y.toFixed(2);};
  canvas.onpointerup=e=>{
    const click=drag&&!drag.moved;drag=null;if(!click)return;
    const r=canvas.getBoundingClientRect(),x=e.clientX-r.left,y=e.clientY-r.top;
    if(symbolPlacement){
      const draft=symbolPlacement;
      if(draft.key!==symbolKey()||draft.runId!==run?.id){cancelPlacement();return;}
      const m={...draft.marker,...projection.world(x,y)};
      const next=symbols(),markers=draft.edit?next.markers.map(old=>old.id===m.id?m:old):[...next.markers,m];
      if(saveSymbols({...next,markers},'Manual event location saved.')){cancelPlacement();message('Manual event location saved. Select the marker to edit it.');}
      else{message($('fp-symbol-message').textContent);cancelPlacement();}
      return;
    }
    const fleetHit=fleetHits.filter(p=>{const b=p.footprint,dx=x-p.x,dy=y-p.y;return Math.hypot(dx,dy)<=p.r||(b&&Math.abs(dx*Math.cos(b.screenAngle)-dy*Math.sin(b.screenAngle))<=b.length*b.scale/2&&Math.abs(dx*Math.sin(b.screenAngle)+dy*Math.cos(b.screenAngle))<=b.width*b.scale/2);}).sort((a,b)=>Math.hypot(x-a.x,y-a.y)-Math.hypot(x-b.x,y-b.y))[0];
    const incident=[...symbolHits.map(p=>({...p,manual:true})),...pinHits,...stopHits.map(p=>({...p,derivedStop:true}))].map(p=>({...p,distance:Math.hypot(x-p.screenX,y-p.screenY)}))
      .filter(p=>p.box?(x>=p.box.x&&x<=p.box.x+p.box.w&&y>=p.box.y&&y<=p.box.y+p.box.h):p.distance<=(p.r||12)).sort((a,b)=>a.distance-b.distance)[0];
    if(incident?.derivedStop){
      const seenDuration=incident.duration;
      message(`Coordinate-based stop · Start ${(incident.startTime-run.points[0][0]).toFixed(2)} s · Observed up to the selected time ${seenDuration.toFixed(2)} s · X ${incident.x.toFixed(3)} / Y ${incident.y.toFixed(3)} · 1 s or more, within 0.01 original units. Rotation and stop commands are not inferred.`);return;
    }
    if(incident?.floorAlert){clearFloorAlert(incident.alertKey,incident);return;}
    if(incident){if(incident.manual)editSymbol(incident,true);else window.dispatchEvent(new CustomEvent('edgeway:event-request',{detail:{id:incident.id}}));return;}
    if(fleetHit&&!(run?.robot===fleetHit.robot&&run?.id===fleetHit.id)){selectFleet(fleetHit);return;}
    if(robotHit){
      const dx=x-robotHit.x,dy=y-robotHit.y,b=robotHit.footprint;
      const insideBody=b&&(b.screenAngle===null?Math.hypot(dx,dy)<=b.radius:Math.abs(dx*Math.cos(b.screenAngle)-dy*Math.sin(b.screenAngle))<=b.length*b.scale/2&&Math.abs(dx*Math.sin(b.screenAngle)+dy*Math.cos(b.screenAngle))<=b.width*b.scale/2);
      if(insideBody||Math.hypot(dx,dy)<=robotHit.r)requestRobotEvents();
    }
  };
  canvas.onpointercancel=()=>{drag=null;};canvas.onpointerleave=()=>{$('fp-pointer').textContent='';};
  canvas.addEventListener('wheel',e=>{if(root.closest('.is-focused')){e.preventDefault();const r=canvas.getBoundingClientRect();zoom(e.deltaY<0?1.12:1/1.12,{x:e.clientX-r.left,y:e.clientY-r.top});}},{passive:false});
  canvas.onkeydown=e=>{if(e.key==='Enter'||e.key===' '){e.preventDefault();requestRobotEvents();return;}if(!projection)return;const moves={ArrowLeft:[-30,0],ArrowRight:[30,0],ArrowUp:[0,-30],ArrowDown:[0,30]},move=moves[e.key];if(move){followRobot(false);fitted=false;const d=projection.delta(...move);view.x+=d.x;view.y+=d.y;}else if(e.key==='+'||e.key==='=')return zoom(1.5);else if(e.key==='-')return zoom(1/1.5);else return;e.preventDefault();draw();};
  $('fp-csv').onclick=()=>{if(run)download(new Blob([FloorplanData.csv(run)],{type:'text/csv;charset=utf-8'}),'edgeway-coordinates.csv');};
  $('fp-png').onclick=async()=>{if(!run)return;draw();await new Promise(requestAnimationFrame);const out=document.createElement('canvas');out.width=canvas.width;out.height=canvas.height+64;const c=out.getContext('2d');c.fillStyle=getComputedStyle(root).getPropertyValue('--surface').trim();c.fillRect(0,0,out.width,out.height);c.drawImage(canvas,0,64);c.fillStyle=getComputedStyle(root).getPropertyValue('--ink').trim();c.font='14px sans-serif';c.fillText(`EdgeWay · ${liveMode?'Received coordinates':'Recorded playback'} · Coordinate units / map alignment unverified`,12,22);c.fillText(`${run.robot} · ${run.id} · Display t_sim ${playhead.toFixed(3)}`,12,44);out.toBlob(blob=>{if(blob)download(blob,'edgeway-recorded-map.png');else message('Could not save the image.');});};
  function geometryMessage(text){$('fp-geometry-message').textContent=text;}
  function geometryNumber(id){const raw=$(id).value;if(!raw.trim())throw Error('Enter all coordinates, dimensions and angles.');const value=Number(raw);if(!Number.isFinite(value))throw Error('Enter a valid number.');return value;}
  function resetObstacle(){geometryEdit=null;for(const k of ['name','x','y','width','height'])$('fp-ob-'+k).value='';$('fp-ob-angle').value='0';$('fp-ob-kind').value='obstacle';$('fp-ob-save').textContent='Add obstacle';$('fp-ob-cancel').hidden=true;}
  function renderGeometry(){
    const profile=geometry();$('fp-geometry-fields').disabled=false;$('fp-body-save').disabled=!run;
    $('fp-geometry-scope').textContent=run?`${scenarioProfiles[activeScenario].name} · Shared obstacles / Dimensions apply to ${run.robot} · ${geometryPersistent?'Saved in this browser (persists on reload)':'Background and obstacles are saved / Robot dimensions apply only to current data'}`:`${scenarioProfiles[activeScenario].name} · Obstacles can be configured · Select a robot to set dimensions`;
    $('fp-template').checked=profile.template;resetObstacle();
    const list=$('fp-ob-list');list.replaceChildren();
    if(!profile.obstacles.length){const empty=document.createElement('p');empty.className='hint';empty.textContent='No obstacles added.';list.append(empty);}
    for(const o of profile.obstacles){
      const row=document.createElement('div');row.className='fp-ob-row';const text=document.createElement('span');text.textContent=`${o.name} · Center (${o.x}, ${o.y}) · ${o.width} × ${o.height} · ${o.angle}°`;
      const edit=document.createElement('button');edit.type='button';edit.className='secondary';edit.textContent='Edit';edit.setAttribute('aria-label',`${o.name} Edit`);
      edit.onclick=()=>{geometryEdit=o.id;for(const k of ['name','kind','x','y','width','height','angle'])$('fp-ob-'+k).value=o[k];$('fp-ob-save').textContent='Save changes';$('fp-ob-cancel').hidden=false;$('fp-ob-name').focus();};
      const remove=document.createElement('button');remove.type='button';remove.className='secondary';remove.textContent='Delete';remove.setAttribute('aria-label',`${o.name} Delete`);remove.onclick=()=>saveGeometry({...geometry(),obstacles:geometry().obstacles.filter(v=>v.id!==o.id)},'Obstacle deleted.');
      row.append(text,edit,remove);list.append(row);
    }
    const body=profile.body;$('fp-body-enabled').checked=!!body;$('fp-body-length').value=body?.length??'';$('fp-body-width').value=body?.width??'';$('fp-body-heading').value=body?.heading??'fixed';$('fp-body-angle').value=body?.angle??0;bodyFields();
    geometryMessage(geometryWarnings.get(geometryKey())||scenarioWarning||'');
  }
  function saveGeometry(next,notice){
    try{

      const value=FloorplanData.geometryProfile(next);
      try{const profile=scenarioProfiles[activeScenario];saveScenario(activeScenario,{...profile,geometry:{version:1,template:value.template,obstacles:value.obstacles},bodies:geometryPersistent&&geometryKey()?{...profile.bodies,[geometryKey()]:value.body}:profile.bodies});}catch{throw Error('Could not save in this browser. Check storage permissions and capacity. Previous settings retained.');}
      geometryCache.set(geometryKey(),value);geometryWarnings.delete(geometryKey());backgroundKey='';
      if(fitted)fit();else draw();renderGeometry();geometryMessage(notice+' Use Fit map if the robot is off screen.');
      return true;
    }catch(e){geometryMessage(e.message);return false;}
  }
  function bodyFields(){for(const id of ['length','width','heading','angle'])$('fp-body-'+id).disabled=!$('fp-body-enabled').checked;}
  $('fp-body-enabled').onchange=bodyFields;
  $('fp-template').onchange=()=>{if(!saveGeometry({...geometry(),template:$('fp-template').checked},'Background display saved.'))$('fp-template').checked=geometry().template;};
  $('fp-ob-cancel').onclick=resetObstacle;
  $('fp-ob-save').onclick=()=>{try{
    const profile=geometry(),obstacle={id:geometryEdit||(globalThis.crypto?.randomUUID?.()??`obstacle-${Date.now()}-${++geometrySerial}`),name:$('fp-ob-name').value,kind:$('fp-ob-kind').value};
    for(const k of ['x','y','width','height','angle'])obstacle[k]=geometryNumber('fp-ob-'+k);
    const obstacles=geometryEdit?profile.obstacles.map(o=>o.id===geometryEdit?obstacle:o):[...profile.obstacles,obstacle];
    saveGeometry({...profile,template:!geometryEdit&&!profile.obstacles.length?false:profile.template,obstacles},geometryEdit?'Obstacle updated.':'Obstacle added.');
  }catch(e){geometryMessage(e.message);}};
  $('fp-body-save').onclick=()=>{try{const body=$('fp-body-enabled').checked?{length:geometryNumber('fp-body-length'),width:geometryNumber('fp-body-width'),heading:$('fp-body-heading').value,angle:geometryNumber('fp-body-angle')}:null;saveGeometry({...geometry(),body},body?'Robot dimensions saved.':'Robot footprint hidden.');}catch(e){geometryMessage(e.message);}};
  function symbolMessage(text){$('fp-symbol-message').textContent=text;}
  function recordIncidentHere(kind){
    if(!run)return;const display=shownPose();
    if(!display.point||!['observed','interpolated'].includes(display.state)){symbolMessage('Cannot record an event without a current position.');return;}
    const marker={id:symbolId(),runId:run.id,kind,name:'',x:display.point[1],y:display.point[2],time:playhead};
    if(saveSymbols({...symbols(),markers:[...symbols().markers,marker]},'Event location and time recorded.')){
      $('fp-settings').close();message(`${FloorplanData.symbolKinds[kind]} location recorded · ${(playhead-(liveMode?0:run.points[0][0])).toFixed(1)} s · Remains here as the robot moves.`);
    }
  }
  function cancelPlacement(){if(!symbolPlacement)return;symbolPlacement=null;canvas.classList.remove('is-placing');root.dataset.placing='false';message('');}
  function symbolId(){return globalThis.crypto?.randomUUID?.()??`symbol-${Date.now()}-${++geometrySerial}`;}
  function resetSymbol(){symbolEdit=null;$('fp-symbol-kind').value='collision';$('fp-symbol-name').value='';$('fp-symbol-x').value='';$('fp-symbol-y').value='';$('fp-symbol-timed').checked=true;$('fp-symbol-time').disabled=false;$('fp-symbol-time').value=run?(liveMode?playhead:Math.max(0,playhead-run.points[0][0])).toFixed(3):'0';$('fp-symbol-save').textContent='Add marker';$('fp-symbol-cancel').hidden=true;}
  function editSymbol(m,open=false){
    if(open){
      renderGeometry();renderSymbols();if(!$('fp-settings').open)$('fp-settings').showModal();
    }
    symbolEdit=m.id;$('fp-symbol-kind').value=m.kind;$('fp-symbol-name').value=m.name;$('fp-symbol-x').value=m.x;$('fp-symbol-y').value=m.y;$('fp-symbol-timed').checked=m.time!==null;$('fp-symbol-time').disabled=m.time===null;$('fp-symbol-time').value=m.time===null?'0':String(m.time-(liveMode?0:run.points[0][0]));$('fp-symbol-save').textContent='Save changes';$('fp-symbol-cancel').hidden=false;$('fp-symbol-kind').focus();
  }
  function renderSymbols(){
    $('fp-symbol-time').parentElement.firstChild.textContent=liveMode?'Shared simulation time (s)':'Elapsed trial time (s)';$('fp-symbol-time').min=liveMode?String(commonTimeline().start):'0';
    const profile=symbols();$('fp-symbol-fields').disabled=!run||!geometryDataset;
    $('fp-symbol-scope').textContent=run?`${run.robot} · Trial ${run.id} · Manual marker · ${geometryPersistent?'Saved in this browser':'Current window only'}`:'Select recorded data first.';
    $('fp-robot-image').checked=profile.robotImage;resetSymbol();const cards=$('fp-state-cards');cards.replaceChildren();
    const display=run&&shownPose(),canRecord=display&&['observed','interpolated'].includes(display.state);
    for(const [kind,label] of Object.entries(FloorplanData.symbolKinds)){
      if(kind==='robot')continue;
      const button=document.createElement('button');button.type='button';button.className='fp-state-card';button.dataset.kind=kind;button.disabled=!canRecord;button.setAttribute('aria-label','Record '+label+' at current position');
      const image=document.createElement('canvas');image.width=image.height=116;image.setAttribute('aria-hidden','true');incidentGlyph(kind,58,58,64,image.getContext('2d'));const text=document.createElement('span');text.textContent='Record '+label;button.append(image,text);
      button.onclick=()=>recordIncidentHere(kind);cards.append(button);
    }
    const list=$('fp-symbol-list');list.replaceChildren();const markers=profile.markers.filter(m=>m.runId===run?.id);
    if(!markers.length){const p=document.createElement('p');p.className='hint';p.textContent='No manual markers in this trial.';list.append(p);}
    for(const m of markers){
      const row=document.createElement('div');row.className='fp-ob-row';const text=document.createElement('span');text.textContent=`Manual · ${m.name||FloorplanData.symbolKinds[m.kind]} · (${m.x.toFixed(2)}, ${m.y.toFixed(2)})${m.time===null?'':` · ${(m.time-(liveMode?0:run.points[0][0])).toFixed(2)} s`}`;
      const edit=document.createElement('button');edit.type='button';edit.textContent='Edit';edit.className='secondary';edit.setAttribute('aria-label',(m.name||FloorplanData.symbolKinds[m.kind])+' Edit marker');edit.onclick=()=>editSymbol(m);
      const remove=document.createElement('button');remove.type='button';remove.textContent='Delete';remove.className='secondary';remove.setAttribute('aria-label',(m.name||FloorplanData.symbolKinds[m.kind])+' Delete marker');remove.onclick=()=>saveSymbols({...symbols(),markers:symbols().markers.filter(v=>v.id!==m.id)},'Manual marker deleted.');row.append(text,edit,remove);list.append(row);
    }
    symbolMessage(symbolWarnings.get(symbolKey())||(!canRecord?'No valid current position. Enter coordinates manually.':''));
  }
  function saveSymbols(next,notice){
    try{if(!symbolKey())throw Error('Select recorded data first.');const value=FloorplanData.symbolProfile(next);
      try{if(geometryPersistent)localStorage.setItem(symbolKey(),JSON.stringify(value));}catch{throw Error('Could not save markers. Previous markers retained.');}
      symbolCache.set(symbolKey(),value);symbolWarnings.delete(symbolKey());draw();renderSymbols();symbolMessage(notice);return true;
    }catch(e){symbolMessage(e.message);return false;}
  }
  function symbolDraft(withCoordinates){
    if(!run)throw Error('Select recorded data first.');
    const elapsed=$('fp-symbol-timed').checked?geometryNumber('fp-symbol-time'):null;
    const range=liveMode?commonTimeline():{start:0,end:run.duration};if(elapsed!==null&&(elapsed<range.start||elapsed>range.end))throw Error('Enter a time within the received data range.');
    return {id:symbolEdit||symbolId(),runId:run.id,kind:$('fp-symbol-kind').value,name:$('fp-symbol-name').value.trim(),x:withCoordinates?geometryNumber('fp-symbol-x'):0,y:withCoordinates?geometryNumber('fp-symbol-y'):0,time:elapsed===null?null:(liveMode?0:run.points[0][0])+elapsed};
  }
  $('fp-symbol-timed').onchange=()=>{$('fp-symbol-time').disabled=!$('fp-symbol-timed').checked;};
  $('fp-to-geometry').onclick=()=>$('fp-geometry-fields').scrollIntoView({block:'start'});
  $('fp-to-symbols').onclick=()=>$('fp-symbol-fields').scrollIntoView({block:'start'});
  $('fp-symbol-cancel').onclick=resetSymbol;
  $('fp-robot-image').onchange=()=>{if(!saveSymbols({...symbols(),robotImage:$('fp-robot-image').checked},'Robot image setting saved.'))$('fp-robot-image').checked=symbols().robotImage;};
  $('fp-symbol-current').onclick=()=>{const display=run&&shownPose();if(!display?.point||!['observed','interpolated'].includes(display.state)){symbolMessage('No valid current position. Enter coordinates manually.');return;}$('fp-symbol-x').value=display.point[1];$('fp-symbol-y').value=display.point[2];$('fp-symbol-time').value=(playhead-(liveMode?0:run.points[0][0])).toFixed(3);};
  $('fp-symbol-save').onclick=()=>{try{const m=symbolDraft(true),p=symbols();saveSymbols({...p,markers:symbolEdit?p.markers.map(old=>old.id===symbolEdit?m:old):[...p.markers,m]},'Manual event location saved.');}catch(e){symbolMessage(e.message);}};
  $('fp-symbol-place').onclick=()=>{try{const m=symbolDraft(false);FloorplanData.symbolProfile({...symbols(),markers:[m]});symbolPlacement={key:symbolKey(),runId:run.id,marker:m,edit:!!symbolEdit};canvas.classList.add('is-placing');root.dataset.placing='true';$('fp-settings').close();message('Manual '+FloorplanData.symbolKinds[m.kind]+' marker: select a point on the map. Esc cancels.');canvas.focus();}catch(e){symbolMessage(e.message);}};
  window.addEventListener('keydown',e=>{if(e.key==='Escape'&&symbolPlacement){e.preventDefault();e.stopImmediatePropagation();cancelPlacement();}},true);
  // Robot and status artwork is drawn as crisp vectors; no raster asset loading.
  $('fp-settings-open').onclick=()=>{cancelPlacement();syncScenarioControls();renderGeometry();renderSymbols();$('fp-settings').showModal();};$('fp-settings-close').onclick=()=>$('fp-settings').close();
  $('fp-clear-tests').onclick=async()=>{
    const button=$('fp-clear-tests'),status=$('fp-clear-status');button.disabled=true;status.textContent='Checking test records…';
    try{
      const response=await fetch('/api/floorplan/test-records',{cache:'no-store',signal:AbortSignal.timeout(8000)});
      const preview=await response.json();if(!response.ok)throw Error(preview.error||'Could not identify test records.');
      if(!preview.count){status.textContent='No test records to clear.';return;}
      if(!window.confirm(`Clear ${preview.count} test records?\n Robots affected: ${preview.robots.join(', ')}\n Test positions, paths and event markers in all scenarios will be cleared.\n Provided records, ordinary data, robot connections and map settings are retained.\n Raw records are archived for recovery. The page will reload when complete.`)){
        status.textContent='Canceled. Records retained.';return;
      }
      status.textContent='Clearing test records…';
      const result=await fetch('/api/floorplan/test-records',{method:'DELETE',headers:{'Content-Type':'application/json','X-EdgeWay-CSRF':preview.csrf},
        body:JSON.stringify({through_id:preview.through_id,fingerprint:preview.fingerprint}),signal:AbortSignal.timeout(10000)});
      const data=await result.json();if(!result.ok)throw Error(data.error||'Could not clear test records.');
      location.reload();
    }catch(error){status.textContent=error.name==='TimeoutError'?'Response not confirmed. Reload and check the records.':error.message;}
    finally{button.disabled=false;}
  };
  $('fp-record-event').onclick=()=>{$('fp-settings-open').click();$('fp-symbol-fields').scrollIntoView({block:'start'});};
  $('fp-map-preset').onclick=()=>{const preset=FloorplanData.factoryMaps[activeScenario];if(!preset)return;$('fp-resolution').value=preset.resolution;['x','y','yaw'].forEach((k,i)=>$('fp-origin-'+k).value=preset.origin[i]);$('fp-frame-confirm').checked=false;message(`${preset.image} Select the original file, confirm the coordinate frame, then apply the map.`,true);};
  $('fp-image').onchange=async e=>{const file=e.target.files[0];if(!file)return;const version=++imageVersion;try{if(file.size>16*1024*1024)throw Error('Select an image no larger than 16 MB.');let image;if(/\.pgm$/i.test(file.name))image=readPGM(new Uint8Array(await file.arrayBuffer()));else {if(!/\.(png|jpe?g)$/i.test(file.name))throw Error('PNG·JPEG·PGM only.');image=await createImageBitmap(file);}if(image.width*image.height>16000000){image.close?.();throw Error('Maps must contain no more than 16 million pixels.');}if(version!==imageVersion){image.close?.();return;}if(pendingImage&&pendingImage.image!==map?.image)pendingImage.image.close?.();pendingImage={image,name:file.name,file};$('fp-frame-confirm').checked=false;message(`${file.name} · ${image.width} × ${image.height}. Check the scale and origin, then apply.`,true);}catch(err){if(version===imageVersion)message(err.message,true);}finally{e.target.value='';}};
  $('fp-yaml').onchange=async e=>{const file=e.target.files[0];if(!file)return;const version=++yamlVersion;try{if(file.size>65536)throw Error('Map YAML must not exceed 64 KB.');const meta=FloorplanData.yaml(await file.text());if(version!==yamlVersion)return;$('fp-resolution').value=meta.resolution;['x','y','yaw'].forEach((k,i)=>$('fp-origin-'+k).value=meta.origin[i]);$('fp-frame-confirm').checked=false;message('YAML scale and origin loaded. Confirm the image and coordinate frame, then apply.',true);}catch(err){if(version===yamlVersion)message(err.message,true);}finally{e.target.value='';}};
  $('fp-map-apply').onclick=async()=>{const id=activeScenario,version=imageVersion;try{if(!pendingImage)throw Error('Select a map image first.');if(!$('fp-frame-confirm').checked)throw Error('Confirm the coordinate frame and units of the map and pose.');const raw=['fp-resolution','fp-origin-x','fp-origin-y','fp-origin-yaw'].map(k=>$(k).value);if(raw.some(v=>!v.trim()))throw Error('Enter both scale and origin.');const values=raw.map(Number);if(!values.every(Number.isFinite)||values[0]<=0||values[0]>1e6||values.slice(1).some(v=>Math.abs(v)>1e9))throw Error('Enter a valid scale and origin.');const next={...pendingImage,resolution:values[0],origin:values.slice(1)};
    await mapRecord(id,{blob:next.file,name:next.name,resolution:next.resolution,origin:next.origin});invalidateMaps(id);if(id!==activeScenario||version!==imageVersion)return;
    const previous=map;map={...next,contentBounds:contentBounds(next.image)};preparedMaps.set(id,map);displayedScenario=id;canvas.dataset.displayedScenario=id;releaseMap(previous);canvas.dataset.mapName=map.name;canvas.dataset.mapState='ready';$('fp-map-info').textContent=`${scenarioProfiles[id].name} · ${map.name} · Scale ${map.resolution} · Origin [${map.origin.join(', ')}]`;fit();message('Scenario background saved. It persists after reload.',true);
    }catch(e){message(e.message||'Could not save background. Previous map retained.',true);}};
  $('fp-map-remove').onclick=async()=>{const id=activeScenario;try{await mapRecord(id,null);invalidateMaps(id);if(activeScenario!==id)return;await restoreScenarioMap();message('Custom map cleared. Server map loaded.',true);}catch{message('Could not clear map settings. Previous map retained.',true);}};
  window.addEventListener('edgeway:floor-zones',()=>draw());
  window.addEventListener('edgeway:factory-change',({detail})=>switchScenario(detail.factory));
  $('fp-scenario-save').onclick=()=>{try{const name=$('fp-scenario-name').value.trim();if(!name||name.length>40)throw Error('Enter a scenario name of 1 to 40 characters.');saveScenario(activeScenario,{...scenarioProfiles[activeScenario],name});message('Scenario name saved.',true);}catch(e){message(e.message,true);}};
  function readPGM(bytes){
    let at=0;const ws=b=>[9,10,13,32].includes(b);
    function token(){while(at<bytes.length){if(bytes[at]===35){while(at<bytes.length&&bytes[at]!==10)at++;}else if(ws(bytes[at]))at++;else break;}const start=at;while(at<bytes.length&&!ws(bytes[at])&&bytes[at]!==35)at++;return new TextDecoder().decode(bytes.slice(start,at));}
    const format=token(),w=Number(token()),h=Number(token()),max=Number(token());
    if(!['P2','P5'].includes(format)||!Number.isInteger(w)||!Number.isInteger(h)||w<=0||h<=0||w*h>16000000||!Number.isInteger(max)||max<1||max>255)throw Error('8 bit P2/P5 PGM only.');
    const c=document.createElement('canvas');c.width=w;c.height=h;const context=c.getContext('2d'),img=context.createImageData(w,h);
    if(format==='P5'){if(!ws(bytes[at]))throw Error('PGM header error');if(bytes[at]===13&&bytes[at+1]===10)at+=2;else at++;if(bytes.length-at!==w*h)throw Error('PGM Pixel count does not match.');}
    for(let i=0;i<w*h;i++){const text=format==='P2'?token():null;const v=format==='P5'?bytes[at++]:text?Number(text):NaN;if(!Number.isInteger(v)||v<0||v>max)throw Error('PGM Invalid pixel value.');const gray=Math.round(v/max*255);img.data.set([gray,gray,gray,255],i*4);}context.putImageData(img,0,0);return c;
  }
  document.addEventListener('visibilitychange',()=>{if(document.hidden){cancelAnimationFrame(cameraFrame);cameraFrame=0;}else{draw();scheduleTrails();}});
  new ResizeObserver(()=>draw()).observe(canvas.parentElement);
  paintMapSymbolLegend();
  syncScenarioControls();receivedBadge();filter();restoreScenarioMap();window.dispatchEvent(new CustomEvent('edgeway:map-ready'));
  setInterval(()=>{for(const [key,trail] of receivedTrails)receivedTrails.set(key,FloorplanData.expireTrail(trail));if(liveMode&&!document.hidden)draw();},1000);
})();
