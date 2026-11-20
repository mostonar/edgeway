(() => {
  'use strict';
  // One judgement/detection state per factory (A/B only when the server has them configured).
  const FACTORIES=['A','B','C'];
  const states={},analyses={};let loading=false,analysisLoading=false,busy=0,lastSignature='';
  const pending=new Map();
  const requestId=()=>Array.from(crypto.getRandomValues(new Uint8Array(16)),v=>v.toString(16).padStart(2,'0')).join('');
  async function api(method='GET',body,csrf,factory='C'){
    const response=await fetch('/api/floor-judgement'+(method==='POST'?'/clear':'?factory='+factory),{method,cache:'no-store',headers:method==='GET'?{}:{'Content-Type':'application/json','X-EdgeWay-CSRF':csrf},...(body?{body:JSON.stringify(body)}:{}),signal:AbortSignal.timeout(20000)});
    if(!response.ok){
      const action=method==='POST'?'send the floor anomaly clear request':'load the Judgement AE status';
      throw Error(`Could not ${action} (HTTP ${response.status}).`);
    }
    return response.json();
  }
  function changed(){
    const signature=JSON.stringify(FACTORIES.map(f=>[states[f]?.connection?.key,states[f]?.zones,states[f]?.geometry,analyses[f]?.connection?.key,analyses[f]?.suspects]));
    if(signature!==lastSignature){lastSignature=signature;window.dispatchEvent(new CustomEvent('edgeway:floor-zones'));}
  }
  async function refresh(){
    if(loading||busy)return;loading=true;
    try{
      for(const f of FACTORIES){try{const value=await api('GET',null,null,f);states[f]=value.enabled===false?null:value;}catch{}}
      changed();
    }finally{loading=false;}
  }
  async function refreshAnalysis(){
    if(analysisLoading)return;analysisLoading=true;
    try{
      for(const f of FACTORIES){
        try{
          const response=await fetch('/api/analysis-verdict?factory='+f,{cache:'no-store',signal:AbortSignal.timeout(5000)});
          if(!response.ok)throw Error('Detection verdicts unavailable.');
          const value=await response.json();analyses[f]=value.enabled===false?null:value;
        }catch{}
      }
      changed();
    }finally{analysisLoading=false;}
  }
  function analysisMarkers(factory){
    const analysis=analyses[factory];
    if(!analysis?.connection||analysis.factory!==factory)return [];
    return analysis.suspects.filter(item=>Number.isFinite(item.x)&&Number.isFinite(item.y)).map((item,index)=>({
      id:-200000-index,x:item.x,y:item.y,source:analysis.profile.url,scenarioId:'scenario-'+('ABC'.indexOf(factory)+1),
      floorAlert:true,analysisVerdictId:item.id,robotId:item.robot_id,alertKey:'analysis:'+analysis.connection.key+':'+item.robot_id,
      kind:'floor_suspected',title:'Suspected floor anomaly'}));
  }
  function bounds(zone){
    if(!zone||!['x1','y1','x2','y2'].every(key=>Number.isFinite(zone[key]))||zone.x1===zone.x2||zone.y1===zone.y2)return null;
    return {x1:Math.min(zone.x1,zone.x2),y1:Math.min(zone.y1,zone.y2),x2:Math.max(zone.x1,zone.x2),y2:Math.max(zone.y1,zone.y2)};
  }
  function center(zone){const box=bounds(zone);return box?{x:(box.x1+box.x2)/2,y:(box.y1+box.y2)/2}:null;}
  function contains(zone,x,y){const box=bounds(zone);return !!box&&x>=box.x1&&x<=box.x2&&y>=box.y1&&y<=box.y2;}
  function factoryOf(pin){return ({'scenario-1':'A','scenario-2':'B','scenario-3':'C'})[pin?.scenarioId];}
  async function clearAttention(pin){
    if(!pin||!Number.isFinite(pin.x)||!Number.isFinite(pin.y))throw Error('The marker does not have valid coordinates.');
    const pendingKey=pin.judgementZoneId||JSON.stringify([pin.source,pin.scenarioId,pin.robotId,pin.x,pin.y]);
    if(pending.has(pendingKey))return pending.get(pendingKey);
    const action=(async()=>{
      busy++;
      try{
        const factory=factoryOf(pin);
        if(!factory)throw Error('This marker does not belong to a factory.');
        const current=await api('GET',null,null,factory);states[factory]=current;
        const conn=current.connection;
        if(!conn||(conn.status!=='active'&&conn.receive_status!=='active'))throw Error('The Judgement AE connection is not ready. Try again once it is connected.');
        if(factoryOf(pin)!==current.factory||(pin.source||'').replace(/\/$/,'')!==conn.profile.url.replace(/\/$/,''))throw Error('This marker belongs to a different factory or CSE.');
        if(pin.connectionKey&&pin.connectionKey!==conn.key)throw Error('The connection changed. Select the current area and try again.');
        const matches=current.zones.filter(zone=>bounds(zone)&&(pin.judgementZoneId?zone.id===pin.judgementZoneId:contains(zone,pin.x,pin.y)))
          .sort((a,b)=>Math.hypot(pin.x-center(a).x,pin.y-center(a).y)-Math.hypot(pin.x-center(b).x,pin.y-center(b).y));
        const zone=matches[0];
        if(!zone)throw Error('The Judgement AE has not reported a matching area for this marker.');
        const result=await api('POST',{factory,zone_id:zone.id,connection_key:conn.key,request_id:requestId(),confirm:true},current.csrf);
        if(result.state==='unknown')throw Error('The CSE did not confirm the clear request. Check the request container before retrying.');
        if(result.state!=='sent')throw Error('The clear request failed. The marker is still on the map.');
        // The backend chooses the center of the stored rectangle, always inside the received bounds.
        const state=states[factory];
        if(state?.connection?.key===conn.key)states[factory]={...state,zones:state.zones.filter(item=>item.id!==zone.id)};
        changed();
        return {state:'sent',message:'The CSE stored the clear request.',cleared_zones:[{...zone,source:conn.profile.url,scenarioId:pin.scenarioId}]};
      }finally{
        busy--;
        await refresh();
      }
    })();
    pending.set(pendingKey,action);
    try{return await action;}finally{pending.delete(pendingKey);}
  }
  function markers(factory){
    const state=states[factory];
    if(!state?.connection||state.factory!==factory)return [];
    return state.zones.filter(zone=>bounds(zone)).map((zone,index)=>({id:-100000-index,...center(zone),x1:zone.x1,y1:zone.y1,x2:zone.x2,y2:zone.y2,
      scenarioId:'scenario-'+('ABC'.indexOf(factory)+1),source:state.connection.profile.url,
      floorAlert:true,judgementZoneId:zone.id,connectionKey:state.connection.key,kind:'attention',robotId:'',title:'Floor anomaly reported'}));
  }
  window.FloorJudgement={clearAttention,markers,analysisMarkers,contains,draw({ctx,canvas,X,Y,factory}){
    const drawn=[],state=states[factory];
    if(state?.factory===factory)for(const zone of state.zones){
      const box=bounds(zone);if(!box)continue;
      const world=[[box.x1,box.y1],[box.x2,box.y1],[box.x2,box.y2],[box.x1,box.y2]];
      const screen=world.map(([x,y])=>[X(x,y),Y(y,x)]);
      ctx.save();ctx.beginPath();screen.forEach(([x,y],i)=>i?ctx.lineTo(x,y):ctx.moveTo(x,y));ctx.closePath();
      // Clip the border and fill to the exact received rectangle: no added buffer.
      ctx.clip();ctx.fillStyle='#d9424238';ctx.strokeStyle='#c43c3c';ctx.lineWidth=2;ctx.fill();ctx.stroke();ctx.restore();
      drawn.push({...zone,world_corners:world,screen_corners:screen});
    }
    canvas.dataset.judgementZones=JSON.stringify(drawn);canvas.dataset.zoneGeometry='rectangle';
  }};
  refresh();refreshAnalysis();setInterval(()=>{if(!document.hidden){refresh();refreshAnalysis();}},1000);
})();
