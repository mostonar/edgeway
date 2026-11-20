"use strict";
// Demo command UI: explicit confirmation; publication never means fault cleared.
(()=>{
  const $=id=>document.getElementById(id);
  let selected=null,generation=0,snapshot=null,busy=false,selectedInspection=false;
  const make=(tag,text)=>{const e=document.createElement(tag);e.textContent=text;return e;};
  for(const id of ['operator-name','operator-note'])$(id).closest('label').hidden=true;
  const preview=make('pre','');preview.style.cssText='white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;max-height:160px;overflow:auto';
  $('operator-help').after(preview);
  async function get(id){
    const r=await fetch(`/api/events/${id}/demo-command`,{cache:'no-store',signal:AbortSignal.timeout(12000)});
    const data=await r.json();if(!r.ok)throw Error(data.error||'Could not load command settings.');return data;
  }
  function render(data){
    snapshot=data;$('operator-panel').hidden=false;$('operator-form').hidden=false;
    $('operator-event-id').textContent=data.robot_id+' · Clear fault request';
    $('operator-status').textContent=data.status;
    $('operator-help').textContent=data.reason||'Sends a request only. Robot completion is not confirmed.';
    preview.textContent='Target: '+(data.url||'Unavailable')+'\n'+JSON.stringify(data.body,null,2)+(data.tls_verified?'':'\nDemo connection: certificate verification disabled.');
    const buttons=$('operator-buttons');buttons.replaceChildren();
    const button=make('button','Send clear_fault');button.type='button';button.className='primary';button.disabled=busy||!data.can_send;
    button.onclick=submit;buttons.append(button);
    $('operator-history-count').textContent=String(data.history.length);
    $('operator-history').replaceChildren(...data.history.map(row=>{
      const li=make('li','');li.append(make('strong',row.state==='sent'?'Request sent':row.state),make('p',row.message),make('small',row.created_at));return li;
    }));
  }
  async function refresh(){
    const id=selected,version=generation;if(id===null||!selectedInspection)return;
    try{const data=await get(id);if(version===generation)render(data);}
    catch(error){if(version===generation){$('operator-panel').hidden=false;$('operator-form').hidden=true;$('operator-status').textContent='Unavailable';$('operator-feedback').textContent=error.message;}}
  }
  async function submit(){
    if(busy||!snapshot?.can_send)return;
    const data=snapshot,id=selected,version=generation;
    if(!window.confirm(`Send clear_fault to ${data.robot_id}?\n${data.url}\n\nThis sends a request; it does not confirm that the fault is cleared.`))return;
    busy=true;render(data);$('operator-feedback').textContent='Sending request…';
    try{
      const bytes=new Uint8Array(16);crypto.getRandomValues(bytes);
      const request_id=Array.from(bytes,b=>b.toString(16).padStart(2,'0')).join('');
      const r=await fetch(`/api/events/${id}/demo-command`,{method:'POST',headers:{'Content-Type':'application/json','X-EdgeWay-CSRF':data.csrf},
        body:JSON.stringify({request_id,robot_id:data.robot_id,expected_url:data.url,command:'clear_fault',confirm:true}),signal:AbortSignal.timeout(15000)});
      const result=await r.json();if(!r.ok)throw Error(result.error||'Could not send request.');
      if(version===generation)$('operator-feedback').textContent=result.message;
    }catch(error){if(version===generation)$('operator-feedback').textContent=error.name==='TimeoutError'?'Transmission unconfirmed. Check the command CNT before resending.':error.message;}
    finally{busy=false;await refresh();}
  }
  window.addEventListener('edgeway:selection',({detail})=>{
    generation++;selected=detail.event?.id??null;snapshot=null;
    $('operator-panel').hidden=selected===null;$('operator-form').hidden=true;
    $('operator-event-id').textContent='Clear fault request';$('operator-status').textContent='Loading';
    $('operator-help').textContent='';$('operator-feedback').textContent='';preview.textContent='';
    $('operator-history').replaceChildren();$('operator-history-count').textContent='0';
    let payload=detail.event?.content;try{if(typeof payload==='string')payload=JSON.parse(payload);}catch{payload=null;}
    const reported=payload?.state??detail.event?.display?.reported_state;
    selectedInspection=RobotState.primary(reported)==='inspection_wait';
    if(!selectedInspection){$('operator-panel').hidden=true;return;}
    refresh();
  });
})();
