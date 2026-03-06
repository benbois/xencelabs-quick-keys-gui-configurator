// Wails bindings - imported from generated module
import { LoadConfig as LoadConfigAPI, SaveConfig as SaveConfigAPI, BackupConfig as BackupConfigAPI, GetConfigPath, GetAvailableKeys, GetBattery, GetDeviceState, ReadConfigFile, WriteConfigFile, ValidateConfigYAML, Quit, SaveGUIState } from './wailsjs/go/main/App.js';
import { EventsOn } from './wailsjs/runtime/runtime.js';
import { WindowHide, WindowGetSize } from './wailsjs/runtime/runtime.js';

let config = null;
let baselineJSON = '';
let keysPanelSourceField = null;
let lastBatteryPercent = -1;

const BATTERY_SUFFIX_RE = /\s*▋▋▋\s*\d+%\s*$/;
function stripBatterySuffix(s) {
  if (!s || typeof s !== 'string') return '';
  return s.replace(BATTERY_SUFFIX_RE, '').trim();
}
function addBatterySuffix(name, pct) {
  const n = (name || '').trim();
  return n ? `${n} ▋▋▋ ${pct}%` : '';
}

async function loadConfig() {
  try {
    config = await LoadConfigAPI();
    if (config) {
      if (!config.device && config.Device) config.device = config.Device;
      if (!config.layers && config.Layers) config.layers = config.Layers;
      baselineJSON = JSON.stringify(config);
      renderGeneral();
      renderLayers();
    }
  } catch (err) {
    console.error('Load config failed:', err);
    alert('Failed to load config: ' + err);
  }
}

// Normalize config: Go JSON uses lowercase keys (device, brightness, etc.)
function device(config) {
  return config?.device ?? config?.Device ?? {};
}
function layers(config) {
  return config?.layers ?? config?.Layers ?? [];
}

function renderGeneral() {
  if (!config) return;
  const d = device(config);
  const brightnessValues = ['off', 'low', 'medium', 'full'];
  const brightnessVal = d.brightness ?? d.Brightness ?? 'medium';
  const brightnessIdx = brightnessValues.indexOf(brightnessVal);
  const brightnessEl = document.getElementById('brightness');
  brightnessEl.value = brightnessIdx >= 0 ? brightnessIdx : 2;
  document.getElementById('brightnessLabel').textContent = brightnessValues[brightnessEl.value];
  const orientationValues = [0, 90, 180, 270];
  const orient = parseInt(d.orientation ?? d.Orientation ?? 0, 10);
  const orientIdx = orientationValues.includes(orient) ? orientationValues.indexOf(orient) : 0;
  const orientEl = document.getElementById('orientation');
  orientEl.value = orientIdx;
  document.getElementById('orientationLabel').textContent = orientationValues[orientIdx] + '°';
  const wheelSpeedValues = ['slowest', 'slower', 'normal', 'faster', 'fastest'];
  const wheelSpeedVal = d.wheel_speed ?? d.WheelSpeed ?? 'normal';
  const wheelSpeedIdx = wheelSpeedValues.indexOf(wheelSpeedVal);
  const wheelSpeedEl = document.getElementById('wheelSpeed');
  wheelSpeedEl.value = wheelSpeedIdx >= 0 ? wheelSpeedIdx : 2;
  document.getElementById('wheelSpeedLabel').textContent = wheelSpeedValues[wheelSpeedEl.value];
  const overlayVal = parseInt(d.overlay_duration ?? d.OverlayDuration ?? 2, 10);
  const overlayEl = document.getElementById('overlayDur');
  overlayEl.value = Math.max(1, Math.min(5, overlayVal));
  document.getElementById('overlayDurLabel').textContent = overlayEl.value + ' s';

  const sleepVal = parseInt(d.sleep_timeout ?? d.SleepTimeout ?? 30, 10);
  const sleepSnap = Math.max(2, Math.min(30, Math.round(sleepVal / 2) * 2));
  const sleepEl = document.getElementById('sleepTimeout');
  sleepEl.value = sleepSnap;
  document.getElementById('sleepTimeoutLabel').textContent = sleepEl.value + ' min';

  const btn8Double = d.button_8_double ?? d.Button8Double ?? [];
  const batteryOnDbl = Array.isArray(btn8Double) && btn8Double.some(k => (k || '').includes('BATTERY'));
  document.getElementById('batteryOnDoubleClick').checked = batteryOnDbl;

  const dblVal = parseInt(d.double_click_ms ?? d.DoubleClickMs ?? 500, 10);
  const dblEl = document.getElementById('doubleClickMs');
  dblEl.value = [300, 400, 500].includes(dblVal) ? dblVal : 500;
  document.getElementById('doubleClickMsLabel').textContent = dblEl.value + ' ms';
  document.getElementById('keyboardLayout').value = d.keyboard_layout ?? d.KeyboardLayout ?? 'qwerty';
  document.getElementById('showBattery').checked = (d.show_battery_in_layer_name ?? d.ShowBatteryInLayerName) !== false;

  updateDoubleClickDurationState();

  const initialSelect = document.getElementById('initialLayer');
  initialSelect.innerHTML = '';
  const layersList = layers(config);
  layersList.forEach((l, i) => {
    const opt = document.createElement('option');
    opt.value = i;
    opt.textContent = `${i} - ${l.name ?? l.Name ?? 'Set ' + (i + 1)}`;
    initialSelect.appendChild(opt);
  });
  const initIdx = parseInt(d.initial_layer ?? d.InitialLayer ?? '', 10);
  if (!isNaN(initIdx) && initIdx >= 0 && initIdx < layersList.length) {
    initialSelect.value = initIdx;
  } else {
    initialSelect.value = 0;
  }
  updateOrientationImg();
}

const MAX_SETS = 5;

function createEmptyLayer() {
  return {
    name: '',
    color: { R: 192, G: 192, B: 192 },
    wheel_speed: 'normal',
    wheel: { left: [], right: [] },
    buttons: {}
  };
}

function renderLayers() {
  if (!config) return;
  const layersList = layers(config);
  if (!layersList.length) return;
  const headers = document.getElementById('tabHeaders');
  const content = document.getElementById('tabContent');
  headers.innerHTML = '';
  content.innerHTML = '';

  layersList.forEach((layer, idx) => {
    const btn = document.createElement('button');
    btn.className = 'tab-btn' + (idx === 0 ? ' active' : '');
    btn.textContent = layer.name ?? layer.Name ?? `Set ${idx + 1}`;
    btn.dataset.index = idx;
    btn.dataset.color = rgbString(layer.color ?? layer.Color);
    btn.spellcheck = false;
    btn.onclick = () => switchTab(idx);
    headers.appendChild(btn);

    const panel = document.createElement('div');
    panel.className = 'tab-panel' + (idx === 0 ? ' active' : '');
    panel.dataset.index = idx;
    panel.dataset.color = rgbString(layer.color ?? layer.Color);
    panel.innerHTML = buildLayerPanel(layer, idx, layersList.length);
    content.appendChild(panel);
  });

  if (layersList.length < MAX_SETS) {
    const addBtn = document.createElement('button');
    addBtn.className = 'tab-btn tab-btn-add';
    addBtn.textContent = '+';
    addBtn.title = 'Add new set';
    addBtn.spellcheck = false;
    addBtn.onclick = () => addNewLayer();
    headers.appendChild(addBtn);
  }
  const activeBtn = headers.querySelector('.tab-btn.active');
  const tabContent = document.getElementById('tabContent');
  if (activeBtn?.dataset.color) {
    const color = activeBtn.dataset.color;
    activeBtn.style.borderColor = color;
    if (tabContent) tabContent.style.borderTopColor = color;
  }
  updateDeviceNameBatteryIndicators();
}

function addNewLayer() {
  const layersList = config.layers ?? config.Layers;
  if (!layersList || layersList.length >= MAX_SETS) return;
  const newLayer = createEmptyLayer();
  layersList.push(newLayer);
  if (!config.layers) config.layers = layersList;
  renderGeneral();
  renderLayers();
  switchTab(layersList.length - 1);
}

function buildLayerPanel(layer, idx, layersCount) {
  const buttons = layer.buttons ?? layer.Buttons ?? {};
  const c = layer.color ?? layer.Color;
  const w = layer.wheel ?? layer.Wheel;
  const wheelSpeed = layer.wheel_speed ?? layer.WheelSpeed ?? 'normal';
  const deleteBtnHtml = layersCount > 1 ? `<button type="button" class="delete-set-btn" data-index="${idx}">Delete this Set</button>` : '';
  const getBtn = (b) => buttons[String(b)] || {};
  const getLabel = (b) => escapeHtml(getBtn(b).label ?? getBtn(b).Label ?? '');
  const getKeys = (b) => escapeHtml((getBtn(b).keys ?? getBtn(b).Keys ?? []).join(', '));
  const showBattery = document.getElementById('showBattery')?.checked ?? false;
  const pct = lastBatteryPercent >= 0 ? lastBatteryPercent : 50;
  const nameVal = (layer.name ?? layer.Name ?? '').trim();
  const displayVal = showBattery ? addBatterySuffix(nameVal, pct) : nameVal;
  let html = `
    <div class="layer-section">
      <input type="color" id="layerColor_${idx}" value="${rgbToHex(c)}" class="layer-color-hidden" aria-hidden="true">
      <div class="form-row wheel-speed-delete-row">
        <label>Wheel speed</label>
        <div class="value-slider">
          <div class="range-container">
            <div class="range">
              <div class="track"></div>
              <input type="range" id="layerWheelSpd_${idx}" class="slider" min="0" max="4" step="1" value="${['slowest','slower','normal','faster','fastest'].indexOf(wheelSpeed) >= 0 ? ['slowest','slower','normal','faster','fastest'].indexOf(wheelSpeed) : 2}">
              <div class="ticks"><span></span><span></span><span></span><span></span><span></span></div>
            </div>
          </div>
          <span id="layerWheelSpdLabel_${idx}">${wheelSpeed}</span>
        </div>
        ${deleteBtnHtml}
      </div>
    </div>
    <div class="device-wrapper">
      <img src="xencelabs-quick-keys_arrow.png" alt="Quick Keys" class="device-img" width="1000" height="398">
      <div class="device-overlay">
        <div class="device-ring-overlay" id="layerRing_${idx}" style="--ring-fill: ${rgbString(c)}">
          <svg viewBox="0 0 177.44724 70.55555" xmlns="http://www.w3.org/2000/svg" class="device-ring-svg">
            <path fill="var(--ring-fill)" d="m 149.58984,13.632813 c -11.93191,0 -21.625,9.693087 -21.625,21.624999 0,11.931913 9.69309,21.626954 21.625,21.626954 11.93192,0 21.62696,-9.695041 21.62696,-21.626954 0,-11.931912 -9.69504,-21.624999 -21.62696,-21.624999 z m 0,1.976562 c 10.86375,0 19.65039,8.784695 19.65039,19.648437 0,10.863743 -8.78664,19.650391 -19.65039,19.650391 -10.86374,0 -19.64843,-8.786648 -19.64843,-19.650391 0,-10.863742 8.78469,-19.648437 19.64843,-19.648437 z"/>
          </svg>
        </div>
        <div class="device-name-wrap" id="layerNameWrap_${idx}">
          <input type="text" id="layerName_${idx}" value="${escapeHtml(displayVal)}" placeholder="Name" class="device-field device-name" title="Set name">
        </div>
        <input type="text" id="layerWheelLeft_${idx}" value="${escapeHtml((w?.left ?? w?.Left ?? []).join(', '))}" placeholder="L" class="device-field device-wheel-l" title="Wheel Left">
        <input type="text" id="layerWheelRight_${idx}" value="${escapeHtml((w?.right ?? w?.Right ?? []).join(', '))}" placeholder="R" class="device-field device-wheel-r" title="Wheel Right">
  `;
  for (let b = 0; b < 8; b++) {
    const keysStr = getKeys(b) || '—';
    html += `<input type="text" id="btnKeys_${idx}_${b}" value="${keysStr}" placeholder="Key" class="device-field device-btn-${b}" title="${escapeHtml(keysStr)}">`;
    html += `<input type="text" id="btnLabel_${idx}_${b}" value="${getLabel(b)}" placeholder="Label" class="device-field device-label-${b}" title="Button ${b} Label">`;
  }
  html += `<input type="text" id="btnKeys_${idx}_8" value="${getKeys(8) || '—'}" placeholder="Layer" class="device-field device-btn-8" title="${escapeHtml(getKeys(8) || '—')}">`;
  html += `<input type="text" id="btnKeys_${idx}_9" value="${getKeys(9) || '—'}" placeholder="Key" class="device-field device-btn-9" title="${escapeHtml(getKeys(9) || '—')}">`;
  html += `<input type="text" id="btnLabel_${idx}_8" value="${getLabel(8)}" style="display:none">`;
  html += `<input type="text" id="btnLabel_${idx}_9" value="${getLabel(9)}" style="display:none">`;
  html += `
      </div>
    </div>
    <p class="device-ring-hint">Click the color ring to pick LED color</p>
    <p class="device-ring-hint">Click any button to assign its key</p>
  `;
  return html;
}

function switchTab(idx) {
  document.querySelectorAll('.tab-btn').forEach(b => {
    b.classList.remove('active');
    b.style.borderColor = '';
  });
  document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
  const activeBtn = document.querySelector(`.tab-btn[data-index="${idx}"]`);
  const tabContent = document.getElementById('tabContent');
  if (activeBtn) {
    activeBtn.classList.add('active');
    const color = activeBtn.dataset.color || '#0078d4';
    activeBtn.style.borderColor = color;
    if (tabContent) tabContent.style.borderTopColor = color;
  }
  const activePanel = document.querySelector(`.tab-panel[data-index="${idx}"]`);
  if (activePanel) activePanel.classList.add('active');
}

function collectGeneral() {
  if (!config) return;
  if (!config.device) config.device = config.Device ?? {};
  const d = config.device;
  const brightnessValues = ['off', 'low', 'medium', 'full'];
  const bIdx = parseInt(document.getElementById('brightness').value, 10);
  d.brightness = brightnessValues[Number.isInteger(bIdx) && bIdx >= 0 && bIdx <= 3 ? bIdx : 2];
  const wheelSpeedValues = ['slowest', 'slower', 'normal', 'faster', 'fastest'];
  const wIdx = parseInt(document.getElementById('wheelSpeed').value, 10);
  d.wheel_speed = wheelSpeedValues[Number.isInteger(wIdx) && wIdx >= 0 && wIdx <= 4 ? wIdx : 2];
  const orientationValues = [0, 90, 180, 270];
  const orientIdx = parseInt(document.getElementById('orientation').value, 10);
  d.orientation = orientationValues[Number.isInteger(orientIdx) && orientIdx >= 0 && orientIdx <= 3 ? orientIdx : 0];
  d.overlay_duration = parseInt(document.getElementById('overlayDur').value, 10) || 2;
  d.sleep_timeout = parseInt(document.getElementById('sleepTimeout').value, 10) || 30;
  d.initial_layer = document.getElementById('initialLayer').value;
  d.double_click_ms = parseInt(document.getElementById('doubleClickMs').value, 10) || 500;
  d.keyboard_layout = document.getElementById('keyboardLayout').value;
  d.show_battery_in_layer_name = document.getElementById('showBattery').checked;

  const batteryOnDbl = document.getElementById('batteryOnDoubleClick').checked;
  d.button_8_double = batteryOnDbl ? ['INTERNAL_BATTERY_OVERLAY'] : [];
}

function collectLayers() {
  if (!config) return;
  const layersList = config.layers ?? config.Layers;
  if (!layersList) return;
  if (!config.layers) config.layers = layersList;
  layersList.forEach((layer, idx) => {
    const nameEl = document.getElementById(`layerName_${idx}`);
    if (nameEl) layer.name = stripBatterySuffix(nameEl.value);
    const colorEl = document.getElementById(`layerColor_${idx}`);
    if (colorEl) {
      const hex = colorEl.value;
      layer.color = layer.color || layer.Color || {};
      layer.color.R = parseInt(hex.slice(1, 3), 16);
      layer.color.G = parseInt(hex.slice(3, 5), 16);
      layer.color.B = parseInt(hex.slice(5, 7), 16);
    }
    const wheelSpdEl = document.getElementById(`layerWheelSpd_${idx}`);
    if (wheelSpdEl) {
      const wsIdx = parseInt(wheelSpdEl.value, 10);
      layer.wheel_speed = ['slowest', 'slower', 'normal', 'faster', 'fastest'][Number.isInteger(wsIdx) && wsIdx >= 0 && wsIdx <= 4 ? wsIdx : 2];
    }
    const wheelLeftEl = document.getElementById(`layerWheelLeft_${idx}`);
    if (wheelLeftEl) {
      layer.wheel = layer.wheel || layer.Wheel || {};
      layer.wheel.left = parseKeys(wheelLeftEl?.value);
    }
    const wheelRightEl = document.getElementById(`layerWheelRight_${idx}`);
    if (wheelRightEl) {
      layer.wheel = layer.wheel || layer.Wheel || {};
      layer.wheel.right = parseKeys(wheelRightEl?.value);
    }

    layer.buttons = layer.buttons || layer.Buttons || {};
    for (let b = 0; b < 10; b++) {
      const label = document.getElementById(`btnLabel_${idx}_${b}`)?.value.trim();
      const keys = parseKeys(document.getElementById(`btnKeys_${idx}_${b}`)?.value);
      if (label || keys?.length) {
        layer.buttons[String(b)] = { Label: label, Keys: keys || [] };
      }
    }
  });
}

function updateDoubleClickDurationState() {
  const checked = document.getElementById('batteryOnDoubleClick')?.checked ?? false;
  const row = document.getElementById('doubleClickDurationRow');
  const slider = document.getElementById('doubleClickMs');
  if (!row || !slider) return;
  row.classList.toggle('disabled', !checked);
  slider.disabled = !checked;
}

function parseKeys(s) {
  if (!s || !s.trim()) return [];
  return s.split(',').map(k => k.trim()).filter(Boolean);
}

function rgbToHex(c) {
  if (!c) return '#c0c0c0';
  const r = (c.R ?? c.r ?? 0).toString(16).padStart(2, '0');
  const g = (c.G ?? c.g ?? 0).toString(16).padStart(2, '0');
  const b = (c.B ?? c.b ?? 0).toString(16).padStart(2, '0');
  return '#' + r + g + b;
}

function rgbString(c) {
  if (!c) return 'rgb(192, 192, 192)';
  const r = c.r ?? c.R ?? 192;
  const g = c.g ?? c.G ?? 192;
  const b = c.b ?? c.B ?? 192;
  return `rgb(${r}, ${g}, ${b})`;
}

function escapeHtml(s) {
  if (!s) return '';
  const div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

function hasUnsavedChanges() {
  collectGeneral();
  collectLayers();
  return JSON.stringify(config) !== baselineJSON;
}

async function save(silent = false) {
  collectGeneral();
  collectLayers();
  const layersList = config.layers ?? config.Layers ?? [];
  const validLayers = layersList.filter(l => (l.name ?? l.Name ?? '').trim() !== '');
  if (validLayers.length === 0) {
    alert('At least one set must have a name.');
    return;
  }
  const oldToNew = [];
  let ni = 0;
  for (let i = 0; i < layersList.length; i++) {
    oldToNew[i] = (layersList[i].name ?? layersList[i].Name ?? '').trim() !== '' ? ni++ : -1;
  }
  const initVal = document.getElementById('initialLayer')?.value ?? '0';
  const initIdx = parseInt(initVal, 10);
  let newInit = '0';
  if (!isNaN(initIdx) && initIdx >= 0 && initIdx < oldToNew.length && oldToNew[initIdx] >= 0) {
    newInit = String(oldToNew[initIdx]);
  }
  const configToSave = { ...config, layers: validLayers };
  const dev = configToSave.device ?? configToSave.Device ?? {};
  dev.initial_layer = dev.InitialLayer = newInit;
  configToSave.device = configToSave.Device = dev;
  try {
    await BackupConfigAPI();
    await SaveConfigAPI(configToSave);
    config.layers = validLayers;
    if (config.Layers) config.Layers = validLayers;
    (config.device ?? config.Device ?? {}).initial_layer = newInit;
    if (config.Device) config.Device.InitialLayer = newInit;
    baselineJSON = JSON.stringify(config);
    renderGeneral();
    renderLayers();
    if (!silent) showModal('Configuration saved!', false, () => {}, 3000);
  } catch (err) {
    alert('Save failed: ' + err);
  }
}

let showModalAutoCloseTimer = null;
function showModal(message, isConfirm, onConfirm, autoCloseMs) {
  if (showModalAutoCloseTimer) {
    clearTimeout(showModalAutoCloseTimer);
    showModalAutoCloseTimer = null;
  }
  const overlay = document.getElementById('modal-overlay');
  const msg = document.getElementById('modal-message');
  const cancelBtn = document.getElementById('modal-cancel');
  const saveExitBtn = document.getElementById('modal-save-exit');
  const confirmBtn = document.getElementById('modal-confirm');
  msg.textContent = message;
  overlay.classList.remove('hidden');
  cancelBtn.style.display = isConfirm ? 'inline-block' : 'none';
  saveExitBtn.style.display = 'none';
  confirmBtn.textContent = isConfirm ? 'Exit' : 'OK';
  const done = () => {
    overlay.classList.add('hidden');
    if (showModalAutoCloseTimer) {
      clearTimeout(showModalAutoCloseTimer);
      showModalAutoCloseTimer = null;
    }
  };
  cancelBtn.onclick = () => { done(); };
  confirmBtn.onclick = () => { onConfirm(); done(); };
  if (autoCloseMs > 0) {
    showModalAutoCloseTimer = setTimeout(() => { done(); }, autoCloseMs);
  }
}

function showDeleteConfirmModal(message, onYes) {
  const overlay = document.getElementById('modal-overlay');
  const msg = document.getElementById('modal-message');
  const cancelBtn = document.getElementById('modal-cancel');
  const saveExitBtn = document.getElementById('modal-save-exit');
  const confirmBtn = document.getElementById('modal-confirm');
  msg.textContent = message;
  overlay.classList.remove('hidden');
  cancelBtn.style.display = 'inline-block';
  saveExitBtn.style.display = 'none';
  cancelBtn.textContent = 'Cancel';
  confirmBtn.textContent = 'Yes';
  const done = () => overlay.classList.add('hidden');
  cancelBtn.onclick = () => { done(); };
  confirmBtn.onclick = () => { onYes(); done(); };
}

function showExitModal(onSaveAndExit, onExitWithoutSaving) {
  const overlay = document.getElementById('modal-overlay');
  const msg = document.getElementById('modal-message');
  const cancelBtn = document.getElementById('modal-cancel');
  const saveExitBtn = document.getElementById('modal-save-exit');
  const confirmBtn = document.getElementById('modal-confirm');
  msg.textContent = 'Unsaved changes';
  overlay.classList.remove('hidden');
  cancelBtn.style.display = 'inline-block';
  saveExitBtn.style.display = 'inline-block';
  saveExitBtn.textContent = 'Save & Exit';
  confirmBtn.textContent = 'Exit anyway';
  const done = () => overlay.classList.add('hidden');
  cancelBtn.onclick = () => { done(); };
  saveExitBtn.onclick = () => { onSaveAndExit(); done(); };
  confirmBtn.onclick = () => { onExitWithoutSaving(); done(); };
}

function updateOrientationImg() {
  const img = document.getElementById('orientationImg');
  if (!img) return;
  const orientationValues = [0, 90, 180, 270];
  const orientIdx = parseInt(document.getElementById('orientation')?.value || '0', 10);
  const deg = orientationValues[Number.isInteger(orientIdx) && orientIdx >= 0 && orientIdx <= 3 ? orientIdx : 0];
  img.style.transform = `rotate(${deg}deg)`;
}

function switchMainTab(tab) {
  document.querySelectorAll('.main-tab-btn').forEach(b => b.classList.remove('active'));
  document.querySelectorAll('.main-tab-panel').forEach(p => p.classList.remove('active'));
  document.querySelector(`.main-tab-btn[data-main-tab="${tab}"]`)?.classList.add('active');
  document.getElementById(`mainTab${tab === 'general' ? 'General' : 'Sets'}`)?.classList.add('active');
}

document.addEventListener('keydown', (e) => {
    const configOverlay = document.getElementById('config-editor-overlay');
    const errPopup = document.getElementById('config-editor-error-popup');
    const modalOverlay = document.getElementById('modal-overlay');
    const keysPanel = document.getElementById('keys-panel');

    if (e.ctrlKey || e.metaKey) {
      if (e.key === 's') {
        e.preventDefault();
        if (configOverlay && !configOverlay.classList.contains('hidden') && (!errPopup || errPopup.classList.contains('hidden'))) {
          document.getElementById('config-editor-save')?.click();
        } else {
          document.getElementById('saveBtn')?.click();
        }
        return;
      }
      if (e.key === 'x') {
        e.preventDefault();
        document.getElementById('quitBtn')?.click();
        return;
      }
    }

    if (e.key !== 'Escape') return;
    if (errPopup && !errPopup.classList.contains('hidden')) {
      document.getElementById('config-editor-error-close')?.click();
      e.preventDefault();
    } else if (configOverlay && !configOverlay.classList.contains('hidden')) {
      document.getElementById('config-editor-cancel')?.click();
      e.preventDefault();
    } else if (modalOverlay && !modalOverlay.classList.contains('hidden')) {
      const cancelBtn = document.getElementById('modal-cancel');
      if (cancelBtn && cancelBtn.style.display !== 'none') {
        cancelBtn.click();
      } else {
        document.getElementById('modal-confirm')?.click();
      }
      e.preventDefault();
    } else if (keysPanel && !keysPanel.classList.contains('hidden')) {
      document.getElementById('keysModalClose')?.click();
      e.preventDefault();
    }
  });

  function init() {
  document.querySelectorAll('.main-tab-btn').forEach(btn => {
    btn.onclick = () => switchMainTab(btn.dataset.mainTab);
  });
  document.getElementById('orientation').addEventListener('input', (e) => {
    const orientationValues = [0, 90, 180, 270];
    const idx = parseInt(e.target.value, 10);
    document.getElementById('orientationLabel').textContent = (orientationValues[idx] ?? 0) + '°';
    updateOrientationImg();
  });
  document.getElementById('saveBtn').onclick = () => save();
  function updateConfigEditorLines(content) {
    const lines = (content || '').split('\n');
    const count = Math.max(lines.length, 1);
    const linesEl = document.getElementById('config-editor-lines');
    const stripesEl = document.getElementById('config-editor-stripes');
    const areaEl = document.getElementById('config-editor-content-area');
    linesEl.innerHTML = lines.map((_, i) => `<div class="config-editor-line">${i + 1}</div>`).join('');
    stripesEl.innerHTML = Array.from({ length: count }, (_, i) =>
      `<div class="config-editor-stripe${i % 2 === 1 ? ' even' : ''}" data-line="${i + 1}"></div>`
    ).join('');
    areaEl.style.minHeight = count * 20 + 24 + 'px';
  }

  function showConfigEditorError(lineNum) {
    const popup = document.getElementById('config-editor-error-popup');
    const lineEl = document.getElementById('config-editor-error-line');
    lineEl.textContent = lineNum > 0 ? `Please check line ${lineNum}` : 'Please check your configuration';
    popup.classList.remove('hidden');
  }
  document.getElementById('config-editor-error-close').onclick = () => {
    document.getElementById('config-editor-error-popup').classList.add('hidden');
  };
  function highlightConfigEditorErrorLine(lineNum) {
    const stripesEl = document.getElementById('config-editor-stripes');
    const scrollEl = document.querySelector('.config-editor-scroll');
    stripesEl?.querySelectorAll('.config-editor-stripe.error').forEach(s => s.classList.remove('error'));
    const line = parseInt(lineNum, 10) || 0;
    if (line > 0 && stripesEl) {
      const stripe = stripesEl.querySelector(`.config-editor-stripe[data-line="${line}"]`);
      if (stripe) {
        stripe.classList.add('error');
        stripe.scrollIntoView({ block: 'center', behavior: 'smooth' });
      } else if (scrollEl) {
        scrollEl.scrollTop = Math.max(0, (line - 1) * 20 - 50);
      }
    }
  }
  document.getElementById('openEditor').onclick = async () => {
    try {
      const content = await ReadConfigFile();
      const overlay = document.getElementById('config-editor-overlay');
      const textarea = document.getElementById('config-editor-text');
      textarea.value = content;
      updateConfigEditorLines(content);
      document.getElementById('config-editor-error-popup').classList.add('hidden');
      overlay.classList.remove('hidden');
    } catch (err) {
      alert('Failed to load config: ' + err);
    }
  };
  document.getElementById('config-editor-text').addEventListener('input', () => {
    if (!document.getElementById('config-editor-overlay').classList.contains('hidden')) {
      updateConfigEditorLines(document.getElementById('config-editor-text').value);
    }
  });
  document.getElementById('config-editor-cancel').onclick = () => {
    document.getElementById('config-editor-overlay').classList.add('hidden');
    document.getElementById('config-editor-error-popup').classList.add('hidden');
  };
  document.getElementById('config-editor-save').onclick = async () => {
    const textarea = document.getElementById('config-editor-text');
    const content = textarea.value;
    const result = await ValidateConfigYAML(content);
    if (result?.error) {
      showConfigEditorError(result.line ?? 0);
      highlightConfigEditorErrorLine(result.line ?? 0);
      return;
    }
    try {
      await WriteConfigFile(content);
      document.getElementById('config-editor-overlay').classList.add('hidden');
      await loadConfig();
      showModal('Configuration saved!', false, () => {}, 3000);
    } catch (err) {
      alert('Failed to save config: ' + err);
    }
  };
  document.getElementById('quitBtn').onclick = async () => {
    const doHide = async () => {
      try {
        const [w, h] = await WindowGetSize();
        if (w > 0 && h > 0) await SaveGUIState(w, h);
      } catch (_) {}
      WindowHide();
    };
    if (hasUnsavedChanges()) {
      showExitModal(
        async () => { await save(true); await doHide(); },
        doHide
      );
    } else {
      await doHide();
    }
  };

  // Slider label sync (chaque slider gère uniquement son propre label)
  document.getElementById('brightness').addEventListener('input', (e) => {
    document.getElementById('brightnessLabel').textContent = ['off', 'low', 'medium', 'full'][parseInt(e.target.value, 10)] || 'medium';
  });
  document.getElementById('wheelSpeed').addEventListener('input', (e) => {
    document.getElementById('wheelSpeedLabel').textContent = ['slowest', 'slower', 'normal', 'faster', 'fastest'][parseInt(e.target.value, 10)] || 'normal';
  });
  document.getElementById('overlayDur').addEventListener('input', (e) => {
    document.getElementById('overlayDurLabel').textContent = e.target.value + ' s';
  });
  document.getElementById('sleepTimeout').addEventListener('input', (e) => {
    document.getElementById('sleepTimeoutLabel').textContent = e.target.value + ' min';
  });
  document.getElementById('doubleClickMs').addEventListener('input', (e) => {
    document.getElementById('doubleClickMsLabel').textContent = e.target.value + ' ms';
  });
  document.getElementById('batteryOnDoubleClick')?.addEventListener('change', () => {
    updateDoubleClickDurationState();
  });
  document.addEventListener('click', (e) => {
    const path = e.target;
    if (path?.tagName === 'path' && path.closest('.device-ring-overlay')) {
      const overlay = path.closest('.device-ring-overlay');
      const idx = overlay.id?.replace('layerRing_', '');
      const colorInput = document.getElementById(`layerColor_${idx}`);
      if (colorInput) colorInput.click();
      return;
    }
    const btn = e.target?.closest('.delete-set-btn');
    if (btn) {
      const idx = parseInt(btn.dataset.index, 10);
      const nameEl = document.getElementById(`layerName_${idx}`);
      const name = stripBatterySuffix(nameEl?.value || '') || `Set ${idx + 1}`;
      const layersList = config?.layers ?? config?.Layers ?? [];
      if (layersList.length <= 1) {
        alert('You must keep at least one set.');
        return;
      }
      showDeleteConfirmModal(`Really delete the set\n\n${name}`, async () => {
        layersList.splice(idx, 1);
        if (!config.layers) config.layers = layersList;
        if (config.Layers) config.Layers = layersList;
        renderGeneral();
        renderLayers();
        switchTab(Math.min(idx, layersList.length - 1));
        await save(true);
      });
    }
  });

  document.addEventListener('input', (e) => {
    const id = e.target?.id;
    if (id?.startsWith('layerWheelSpd_') && !id?.includes('Label')) {
      const idx = id.replace('layerWheelSpd_', '');
      const labelEl = document.getElementById(`layerWheelSpdLabel_${idx}`);
      if (labelEl) labelEl.textContent = ['slowest', 'slower', 'normal', 'faster', 'fastest'][parseInt(e.target.value, 10)] || 'normal';
    }
    if (id?.startsWith('layerName_')) {
      const idx = id.replace('layerName_', '');
      const tabBtn = document.querySelector(`.tab-btn[data-index="${idx}"]`);
      if (tabBtn) tabBtn.textContent = stripBatterySuffix(e.target.value) || `Set ${parseInt(idx, 10) + 1}`;
    }
    if (id?.match(/^btnKeys_\d+_\d+$/)) {
      e.target.title = (e.target.value || '').trim() || '—';
    }
  });

  // Color picker sync (ring SVG + tab border)
  document.addEventListener('input', (e) => {
    if (e.target?.id?.startsWith('layerColor_')) {
      const idx = e.target.id.replace('layerColor_', '');
      const hex = e.target.value;
      const r = parseInt(hex.slice(1, 3), 16);
      const g = parseInt(hex.slice(3, 5), 16);
      const b = parseInt(hex.slice(5, 7), 16);
      const rgb = `rgb(${r}, ${g}, ${b})`;
      const ringEl = document.getElementById(`layerRing_${idx}`);
      if (ringEl) ringEl.style.setProperty('--ring-fill', rgb);
      const tabBtn = document.querySelector(`.tab-btn[data-index="${idx}"]`);
      if (tabBtn) {
        tabBtn.dataset.color = rgb;
        if (tabBtn.classList.contains('active')) tabBtn.style.borderColor = rgb;
      }
      const tabPanel = document.querySelector(`.tab-panel[data-index="${idx}"]`);
      if (tabPanel) {
        tabPanel.dataset.color = rgb;
      }
      const tabContent = document.getElementById('tabContent');
      if (tabContent && tabBtn?.classList.contains('active')) {
        tabContent.style.borderTopColor = rgb;
      }
    }
  });

  // Mémoriser la taille à la fermeture et lors du redimensionnement
  let resizeDebounce = null;
  window.addEventListener('resize', () => {
    clearTimeout(resizeDebounce);
    resizeDebounce = setTimeout(async () => {
      try {
        const [w, h] = await WindowGetSize();
        if (w > 0 && h > 0) await SaveGUIState(w, h);
      } catch (_) {}
    }, 500);
  });

  document.getElementById('keysModalClose').onclick = () => {
    document.getElementById('keys-panel').classList.add('hidden');
  };

  const keysCustomInput = document.getElementById('keysCustomInput');
  const keysClearBtn = document.getElementById('keysClearBtn');
  const keysCodeWrap = document.querySelector('.keys-code-input-wrap');

  function updateKeysClearButton() {
    const v = (keysCustomInput?.value || '').trim();
    keysCodeWrap?.classList.toggle('has-value', !!v);
  }

  keysCustomInput?.addEventListener('input', updateKeysClearButton);
  keysCustomInput?.addEventListener('change', updateKeysClearButton);

  keysClearBtn?.addEventListener('click', () => {
    if (keysCustomInput) {
      keysCustomInput.value = '';
      updateKeysClearButton();
      keysCustomInput.focus();
    }
  });

  document.getElementById('keysUseCustomBtn').onclick = () => {
    const input = document.getElementById('keysCustomInput');
    const code = (input?.value || '').trim();
    if (code) {
      insertKeyIntoSourceField(code);
      if (input) input.value = '';
      updateKeysClearButton();
    }
  };

  document.addEventListener('focusin', (e) => {
    const id = e.target?.id;
    if (id && (id.startsWith('btnKeys_') || id.startsWith('layerWheelLeft_') || id.startsWith('layerWheelRight_'))) {
      keysPanelSourceField = e.target;
      showKeysModal();
    }
    if (id?.startsWith('layerName_')) {
      const input = e.target;
      if (document.getElementById('showBattery')?.checked && BATTERY_SUFFIX_RE.test(input.value)) {
        input.value = stripBatterySuffix(input.value);
      }
    }
  });

  document.addEventListener('focusout', (e) => {
    const id = e.target?.id;
    if (id?.startsWith('layerName_') && document.getElementById('showBattery')?.checked) {
      const input = e.target;
      const pct = lastBatteryPercent >= 0 ? lastBatteryPercent : 50;
      const name = (input.value || '').trim();
      if (name) input.value = addBatterySuffix(name, pct);
    }
  });

  document.getElementById('showBattery')?.addEventListener('change', updateDeviceNameBatteryIndicators);

  loadConfig();
  updateBatteryDisplay();
  setTimeout(updateBatteryDisplay, 500);
  setInterval(updateBatteryDisplay, 2000);
  if (typeof EventsOn === 'function') {
    EventsOn('deviceStateChanged', (plugged) => {
      updateDeviceStateDisplay(plugged === true);
      updateBatteryDisplay();
    });
  }
}

async function showKeysModal() {
  const panel = document.getElementById('keys-panel');
  const table = document.getElementById('keysTable');
  const input = document.getElementById('keysCustomInput');
  if (!panel || !table) return;
  if (input) {
    input.value = (keysPanelSourceField?.value || '').trim();
    document.querySelector('.keys-code-input-wrap')?.classList.toggle('has-value', !!input.value.trim());
  }
  try {
    const keys = await GetAvailableKeys();
    const cols = 4;
    let html = '<thead><tr>' + '<th>Key</th>'.repeat(cols) + '</tr></thead><tbody>';
    for (let i = 0; i < keys.length; i += cols) {
      html += '<tr>';
      for (let c = 0; c < cols; c++) {
        const k = keys[i + c];
        html += k ? `<td data-key="${(k || '').replace(/"/g, '&quot;')}">${escapeHtml(k)}</td>` : '<td></td>';
      }
      html += '</tr>';
    }
    html += '</tbody>';
    table.innerHTML = html;
    table.querySelectorAll('td[data-key]').forEach(td => {
      td.onclick = () => appendKeyToCustomField(td.dataset.key);
    });
    panel.classList.remove('hidden');
  } catch (err) {
    console.error('GetAvailableKeys failed:', err);
    table.innerHTML = '<tbody><tr><td>Failed to load keys</td></tr></tbody>';
    panel.classList.remove('hidden');
  }
}

function appendKeyToCustomField(key) {
  const input = document.getElementById('keysCustomInput');
  if (!input) return;
  const cur = (input.value || '').trim();
  input.value = cur ? cur + ', ' + key : key;
  input.dispatchEvent(new Event('input'));
}

function insertKeyIntoSourceField(key) {
  const el = keysPanelSourceField;
  if (!el || !el.matches('input[type="text"]')) return;
  el.value = key;
  el.title = key || '—';
  document.getElementById('keys-panel').classList.add('hidden');
}

function deviceStateIconSvg(plugged) {
  const color = plugged ? '#4caf50' : '#999';
  if (plugged) {
    return `<svg viewBox="0 0 16 16" width="14" height="14" class="device-state-svg"><rect x="5" y="2" width="6" height="8" rx="1" fill="${color}"/><rect x="5" y="10" width="2" height="4" fill="${color}"/><rect x="9" y="10" width="2" height="4" fill="${color}"/></svg>`;
  }
  return `<svg viewBox="0 0 16 16" width="14" height="14" class="device-state-svg"><rect x="5" y="2" width="6" height="8" rx="1" fill="${color}"/><rect x="5" y="10" width="2" height="4" fill="${color}"/><rect x="9" y="10" width="2" height="4" fill="${color}"/><line x1="2" y1="14" x2="14" y2="2" stroke="${color}" stroke-width="1.5" stroke-linecap="round"/></svg>`;
}

function updateDeviceStateDisplay(plugged) {
  const iconEl = document.getElementById('deviceStateIcon');
  const textEl = document.getElementById('deviceStateText');
  if (!iconEl || !textEl) return;
  const state = plugged ? 'Plugged' : 'Unplugged';
  iconEl.innerHTML = deviceStateIconSvg(plugged);
  textEl.textContent = state;
}

async function updateBatteryDisplay() {
  const iconEl = document.getElementById('batteryIcon');
  const pctEl = document.getElementById('batteryPct');
  const stateEl = document.getElementById('batteryState');
  if (!iconEl || !pctEl || !stateEl) return;
  try {
    const res = await GetBattery();
    const percent = res?.percent ?? res?.Percent ?? -1;
    lastBatteryPercent = percent;
    const charging = res?.charging ?? res?.Charging ?? false;
    pctEl.textContent = percent >= 0 ? `${percent}%` : '—';
    const state = percent >= 0 ? (charging ? 'Charging' : 'Discharging') : '';
    stateEl.textContent = state || '';
    stateEl.title = percent >= 0 ? (charging ? `${percent}% (charging)` : `${percent}%`) : 'Battery unknown';
    iconEl.innerHTML = batteryIconSvg(percent, charging);
    updateDeviceNameBatteryIndicators();
  } catch (err) {
    console.error('GetBattery failed:', err);
    lastBatteryPercent = -1;
    pctEl.textContent = '—';
    stateEl.textContent = '';
    iconEl.innerHTML = batteryIconSvg(-1, false);
    updateDeviceNameBatteryIndicators();
  }
  try {
    const deviceStateIconEl = document.getElementById('deviceStateIcon');
    const deviceStateTextEl = document.getElementById('deviceStateText');
    if (deviceStateIconEl && deviceStateTextEl && typeof GetDeviceState === 'function') {
      const deviceState = await GetDeviceState();
      const plugged = deviceState === 'Plugged';
      updateDeviceStateDisplay(plugged);
    }
  } catch (_) {
    const textEl = document.getElementById('deviceStateText');
    const iconEl2 = document.getElementById('deviceStateIcon');
    if (textEl) textEl.textContent = '—';
    if (iconEl2) iconEl2.innerHTML = '';
  }
}

function updateDeviceNameBatteryIndicators() {
  const showBattery = document.getElementById('showBattery')?.checked ?? false;
  const pct = lastBatteryPercent >= 0 ? lastBatteryPercent : 50;
  document.querySelectorAll('.device-name-wrap input.device-name').forEach(input => {
    if (document.activeElement === input) return;
    const name = stripBatterySuffix(input.value);
    input.value = showBattery ? addBatterySuffix(name, pct) : name;
    input.title = showBattery ? `${name} ▋▋▋ ${pct}%` : (name || 'Set name');
  });
}

function batteryIconSvg(percent, charging) {
  const pct = Math.max(0, Math.min(100, percent));
  const fill = percent < 0 ? 0 : pct / 100;
  let color = '#999';
  if (percent >= 0) {
    if (charging) color = '#4caf50';
    else if (percent <= 20) color = '#f44336';
    else if (percent <= 40) color = '#ff9800';
    else if (percent <= 60) color = '#ffc107';
    else color = '#4caf50';
  }
  const w = 22;
  const h = 11;
  const pad = 1.5;
  const innerW = Math.max(0, (w - pad * 2) * fill);
  const tipX = w - 1;
  const tipW = 1.5;
  const tipH = 4;
  return `<svg viewBox="0 0 24 12" width="22" height="12" class="battery-svg">
    <rect x="0.5" y="0.5" width="${w}" height="${h}" rx="1.5" fill="none" stroke="${color}" stroke-width="1"/>
    <rect x="${tipX}" y="${(h - tipH) / 2}" width="${tipW}" height="${tipH}" rx="0.5" fill="${color}"/>
    ${innerW > 0 ? `<rect x="${pad}" y="${pad}" width="${innerW}" height="${h - pad * 2}" rx="0.5" fill="${color}"/>` : ''}
    ${charging ? `<path d="M11 1 L13 5 L11 5 L12 9 L9 4 L11 4 Z" fill="${color}"/>` : ''}
  </svg>`;
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
