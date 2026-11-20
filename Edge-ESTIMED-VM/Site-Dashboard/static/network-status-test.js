(() => {
  'use strict';
  const samples = {
    stale: {
      tone: 'neutral', glyph: '#position-stale', showMapBadge: false,
      label: 'Stale position', detail: 'Last known position · 24 s ago',
    },
    disconnected: {
      tone: 'disconnected', glyph: '#network-disconnected', showMapBadge: true,
      label: 'Network disconnected', detail: 'Position · Last known',
    },
    recovered: {
      tone: 'recovered', glyph: '#network-recovered', showMapBadge: true,
      label: 'Network recovered', detail: 'Position · Last known until next update',
    },
  };

  const controls = document.getElementById('network-test-controls');
  const mapBadge = document.getElementById('map-network-badge');
  const mapGlyph = document.getElementById('map-network-icon');
  const mapDescription = document.getElementById('test-map-description');
  const statusChip = document.getElementById('test-status-icon');
  const statusGlyph = document.getElementById('test-status-glyph');
  const label = document.getElementById('test-state-label');
  const detail = document.getElementById('test-state-detail');
  const recoveryEvent = document.getElementById('test-recovery-event');
  const recoveryDetails = document.getElementById('test-recovery-details');
  const sampleEvent = {
    robotName: 'C-2',
    recoveredAt: '2026-11-19T14:35:00+09:00',
    outageDurationSeconds: 78,
    lostDataCount: 12,
  };
  const eventTime = new Intl.DateTimeFormat('en-GB', {
    timeStyle: 'medium', timeZone: 'Asia/Seoul',
  });

  function formatDuration(totalSeconds) {
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;
    const parts = [];
    if (hours) parts.push(`${hours} hr`);
    if (minutes) parts.push(`${minutes} min`);
    if (seconds || !parts.length) parts.push(`${seconds} sec`);
    return parts.join(' ');
  }

  function showSampleEvent() {
    if (!recoveryEvent.hidden) return;
    document.getElementById('test-event-robot').textContent = sampleEvent.robotName;
    const recoveredAt = document.getElementById('test-event-time');
    recoveredAt.dateTime = sampleEvent.recoveredAt;
    recoveredAt.textContent = `${eventTime.format(new Date(sampleEvent.recoveredAt))} KST`;
    document.getElementById('test-event-empty').hidden = true;
    recoveryEvent.hidden = false;
  }

  recoveryEvent.addEventListener('click', () => {
    document.getElementById('test-detail-robot').textContent = sampleEvent.robotName;
    const duration = document.getElementById('test-detail-outage-duration');
    duration.dateTime = `PT${sampleEvent.outageDurationSeconds}S`;
    duration.textContent = formatDuration(sampleEvent.outageDurationSeconds);
    document.getElementById('test-detail-lost-count').textContent = String(sampleEvent.lostDataCount);
    recoveryDetails.showModal();
  });

  function renderSample() {
    const state = controls.querySelector('input:checked').value;
    const sample = samples[state];
    mapBadge.setAttribute('visibility', sample.showMapBadge ? 'visible' : 'hidden');
    mapBadge.dataset.tone = sample.tone;
    mapGlyph.setAttribute('href', sample.glyph);
    mapDescription.textContent = `Factory C map, sample robot C-2. ${sample.label}. Last known position.`;
    statusChip.dataset.tone = sample.tone;
    statusGlyph.setAttribute('href', sample.glyph);
    label.dataset.tone = sample.tone;
    label.textContent = sample.label;
    detail.textContent = sample.detail;
    if (state === 'recovered') showSampleEvent();
  }

  controls.addEventListener('change', renderSample);
  renderSample();
})();
