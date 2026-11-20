/* Display-only state transitions and discrete events. Raw telemetry is unchanged. */
const primaryState=typeof RobotState!=='undefined'?RobotState.primary:require('./robot-state.js').primary;
const EventLogSummary=(()=>{
  const allowedStates=new Set(['idle','to_pick','loading','to_drop','unloading','done','anomaly_detected','inspection_wait','cross_check']);
  const states={anomaly_detected:'Suspected robot fault',inspection_wait:'Robot fault',cross_check:'Cross-checking',loading:'Picking up',unloading:'Unloading',to_drop:'Moving with load',done:'Task complete',moving:'Moving',navigating:'Moving',to_pick:'Moving',idle:'Stopped',stopped:'Stopped',paused:'Stopped',waiting:'Stopped',
    picking:'Picking up',pickup:'Picking up',dropping:'Dropping off',dropoff:'Dropping off',carrying:'Carrying',loaded:'Carrying',
    error:'Robot error',fault:'Robot fault',failed:'Task failed',collision:'Collision',blocked:'Path blocked',aborted:'Task aborted',reset:'Resetting',arrived:'Arrived',completed:'Task complete'};
  const types={object_pickup:'Pickup',pickup:'Pickup',object_dropoff:'Drop-off',dropoff:'Drop-off',object_carried:'Carrying',
    obstacle_wait:'Waiting for obstacle',robot_fault:'Robot fault',robot_error:'Robot error',collision:'Collision'};
  function activity(e){
    let p=e.content??e.map_content??e.preview;try{if(typeof p==='string')p=JSON.parse(p);}catch{p=null;}
    const d=e.display||{},state=p?.state??p?.nav??d.reported_state,phase=primaryState(state);
    const type=(p?.event?.type==='object_interaction'?p.event.action:p?.event?.type)??p?.event_type??d.event_type;
    const canonical=phase?(states[phase]||phase):null;
    const warning=!!d.restriction||['warning','error'].includes(d.level);
    const explicit=!!type||d.kind==='Event'||warning&&!canonical;
    const title=explicit?(types[type]||d.title||String(type)):warning?(d.title||canonical||'Alert'):canonical;
    const stamp=p?.t_wall??d.state_time;
    return {title,phase,state:canonical,explicit,priority:explicit||warning||canonical&&!['Moving','Stopped'].includes(canonical)?1:0,
      signature:canonical?JSON.stringify([phase,warning?d.level:null,d.restriction||null]):null,
      stamp:Number.isFinite(stamp)?stamp:null,statePriority:typeof p?.state==='string'?2:typeof p?.nav==='string'?1:d.state_priority||0};
  }
  const track=e=>JSON.stringify([e.source||'',e.robot_id]);
  const key=e=>JSON.stringify([e.source||'',e.robot_id,e.id]);
  // Keep hidden states internally so ordering and real state transitions remain correct.
  const visible=g=>allowedStates.has(activity(g.event).phase)&&(g.kind==='event'||!g.covered&&(!g.initial||g.priority>0));
  function rows(groups){
    // Hidden turn/reset states do not create a new visible Moving transition.
    const last=new Map(),result=[];
    for(const g of [...groups.values()].sort((a,b)=>a.event.id-b.event.id)){
      const phase=activity(g.event).phase;
      if(!allowedStates.has(phase))continue;
      if(last.get(g.track)===phase)continue;
      last.set(g.track,phase);
      if(visible(g))result.push({...g.event,summaryKey:g.key,summaryCount:1,display:{...g.event.display,title:phase}});
    }
    return result.reverse();
  }
  function add(groups,e){
    if(!Number.isSafeInteger(e?.id)||!e.robot_id)return false;
    const a=activity(e),t=track(e);let changed=false;
    if(a.explicit){
      const k='event:'+key(e);
      if(!groups.has(k)){groups.set(k,{key:k,track:t,kind:'event',event:e,title:a.title,priority:a.priority,lastId:e.id});changed=true;}
    }
    if(!a.state)return changed;
    const phases=[...groups.values()].filter(g=>g.kind==='state'&&g.track===t).sort((x,y)=>x.event.id-y.event.id);
    const last=phases.at(-1);
    // Older telemetry must not undo the newer status from the other container.
    if(last&&e.id>last.endId&&a.stamp!==null&&last.latest.stamp!==null&&
       (a.stamp<last.latest.stamp||a.stamp===last.latest.stamp&&a.statePriority<last.latest.statePriority))return changed;
    // Loaded history pages overlap; an already covered interval needs no new row.
    if(phases.some(g=>e.id>=g.event.id&&e.id<=g.endId))return changed;
    const prev=phases.findLast(g=>g.endId<e.id),next=phases.find(g=>g.event.id>e.id);
    if(prev?.signature===a.signature){
      prev.endId=e.id;prev.latest=a;
      if(next?.signature===a.signature){prev.endId=next.endId;prev.latest=next.latest;groups.delete(next.key);changed=changed||visible(next);}
      return changed;
    }
    if(next?.signature===a.signature){
      changed=changed||visible(next);
      next.event=e;next.lastId=e.id;next.covered=a.explicit;next.initial=!prev;
      return changed||visible(next);
    }
    const k='state:'+key(e),g={key:k,track:t,kind:'state',event:e,endId:e.id,lastId:e.id,latest:a,
      signature:a.signature,title:a.title||a.state,priority:a.priority,initial:!prev,covered:a.explicit};
    if(next?.initial){next.initial=false;changed=changed||visible(next);}
    groups.set(k,g);return changed||visible(g);
  }
  function trim(groups,limit=200){
    const anchors=new Map();for(const g of groups.values())if(g.kind==='state'&&(!anchors.has(g.track)||anchors.get(g.track).endId<g.endId))anchors.set(g.track,g);
    const keep=new Set(anchors.values());let before=0;
    for(const g of [...groups.values()].sort((a,b)=>a.event.id-b.event.id)){
      if(groups.size<=limit+keep.size)break;if(keep.has(g))continue;
      groups.delete(g.key);before=Math.max(before,g.event.id+1);
    }
    return before;
  }
  return {activity,key,add,rows,trim};
})();
if(typeof module!=='undefined'&&module.exports)module.exports=EventLogSummary;
