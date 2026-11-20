(() => {
  'use strict';
  const main = document.querySelector('#settings-main');
  if (!main) return;
  const section = document.createElement('section');
  section.className = 'settings-section';
  section.setAttribute('aria-labelledby', 'factory-c-address-title');
  section.innerHTML = `<div class="section-title"><div><p class="step-label">Factory C</p><h2 id="factory-c-address-title">Factory C connection URL</h2></div></div>
    <p class="hint">C-1 and C-2 status/position queries and error release commands use the same URL.</p>
    <form id="factory-c-address-form"><label>Target<select id="factory-c-address-select" disabled></select></label>
    <p id="factory-c-address-url" class="mono"></p><div class="button-row"><button id="factory-c-address-save" class="primary" type="submit" disabled>Apply selected URL</button><button id="factory-c-address-reload" class="secondary" type="button">Reload saved value</button></div></form>
    <p id="factory-c-address-feedback" class="hint" role="status" aria-live="polite">Loading connection URL...</p>`;
  const connectionCard = main.querySelector('.connection-card');
  if (connectionCard) connectionCard.after(section); else main.prepend(section);
  const select = section.querySelector('select'), button = section.querySelector('button[type="submit"]');
  const reload = section.querySelector('#factory-c-address-reload');
  const feedback = section.querySelector('[role="status"]'), url = section.querySelector('.mono');
  let current = null, busy = false, dirty = false;
  function render() {
    const option = current?.options.find(item => item.id === select.value);
    url.textContent = option?.url || '';
    select.disabled = busy || !current;
    button.disabled = busy || !current || select.value === current.preset;
    reload.disabled = busy;
  }
  async function request(method, body) {
    const response = await fetch('/api/factory-c/address', {method, cache:'no-store',
      headers: method === 'PUT' ? {'Content-Type':'application/json','X-EdgeWay-CSRF':current.csrf} : {},
      ...(body ? {body:JSON.stringify(body)} : {})});
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || 'Could not save the URL.');
    return data;
  }
  function accept(data) {
    current = data;
    select.replaceChildren(...data.options.map(item => new Option(item.label, item.id)));
    select.value = data.preset;
    dirty = false;
  }
  async function load() {
    busy = true; render();
    try { accept(await request('GET')); feedback.textContent = 'This is the saved URL. Select another URL and apply to change it.'; }
    catch (error) { feedback.textContent = error.message; }
    finally { busy = false; render(); }
  }
  select.addEventListener('change', () => { dirty = true; render(); feedback.textContent = 'Not applied yet. Apply to change both the query and command URL.'; });
  reload.addEventListener('click', load);
  section.querySelector('form').addEventListener('submit', async event => {
    event.preventDefault(); if (busy || !current || !dirty) return;
    const preset = select.value;
    busy = true; render();
    try {
      accept(await request('PUT', {preset, expected_revision:current.revision}));
      feedback.textContent = 'URL saved. It applies from the next query. Once new data arrives, select the record for this URL.';
    } catch (error) { feedback.textContent = error.message + ' Reload the saved value to check.'; }
    finally { busy = false; render(); }
  });
  load();
})();
