(() => {
  'use strict';
  const main = document.querySelector('#settings-main');
  if (!main) return;
  const section = document.createElement('section');
  section.className = 'settings-section';
  section.setAttribute('aria-labelledby', 'factory-c-address-title');
  section.innerHTML = `<div class="section-title"><div><p class="step-label">Factory C</p><h2 id="factory-c-address-title">공장 C 연결 주소</h2></div></div>
    <p class="hint">C-1·C-2의 상태·좌표 조회와 오류 해제 명령에 같은 주소를 사용합니다.</p>
    <form id="factory-c-address-form"><label>연결 대상<select id="factory-c-address-select" disabled></select></label>
    <p id="factory-c-address-url" class="mono"></p><div class="button-row"><button id="factory-c-address-save" class="primary" type="submit" disabled>선택한 주소 적용</button><button id="factory-c-address-reload" class="secondary" type="button">저장값 다시 보기</button></div></form>
    <p id="factory-c-address-feedback" class="hint" role="status" aria-live="polite">연결 주소를 불러오는 중입니다.</p>`;
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
    if (!response.ok) throw new Error(data.error || '주소를 저장하지 못했습니다.');
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
    try { accept(await request('GET')); feedback.textContent = '현재 저장된 주소입니다. 다른 주소를 선택한 뒤 적용하세요.'; }
    catch (error) { feedback.textContent = error.message; }
    finally { busy = false; render(); }
  }
  select.addEventListener('change', () => { dirty = true; render(); feedback.textContent = '아직 적용하지 않았습니다. 조회·명령 주소를 함께 바꾸려면 적용하세요.'; });
  reload.addEventListener('click', load);
  section.querySelector('form').addEventListener('submit', async event => {
    event.preventDefault(); if (busy || !current || !dirty) return;
    const preset = select.value;
    busy = true; render();
    try {
      accept(await request('PUT', {preset, expected_revision:current.revision}));
      feedback.textContent = '주소를 저장했습니다. 다음 조회부터 적용됩니다. 새 데이터 수신 후 해당 주소의 기록을 선택하세요.';
    } catch (error) { feedback.textContent = error.message + ' 저장값을 다시 불러와 확인하세요.'; }
    finally { busy = false; render(); }
  });
  load();
})();
