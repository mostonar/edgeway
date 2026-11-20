"use strict";
(async()=>{
  const message=document.getElementById('camera-message'),container=document.getElementById('camera-frame');
  try{
    const response=await fetch('/api/camera',{cache:'no-store',signal:AbortSignal.timeout(8000)});
    if(!response.ok)throw Error('Could not load camera settings.');
    const camera=await response.json();
    if(!camera.url)throw Error('Save a web viewer URL in Connections first.');
    const url=new URL(camera.url);url.searchParams.set('edgeway_embed','1');
    const frame=document.createElement('iframe');frame.title=camera.name||'Site Camera';
    frame.allow='autoplay; fullscreen';frame.allowFullscreen=true;frame.referrerPolicy='no-referrer';
    frame.setAttribute('sandbox','allow-scripts allow-same-origin allow-forms allow-pointer-lock');
    frame.addEventListener('load',()=>{message.hidden=true;});
    frame.src=url.href;container.append(frame);
  }catch(error){message.hidden=false;message.textContent=error.message;}
})();
