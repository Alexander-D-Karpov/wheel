'use strict';

const wheelId = document.body.dataset.wheelId;
const minSeconds = parseInt(document.body.dataset.minSeconds, 10) || 3;
const maxSeconds = parseInt(document.body.dataset.maxSeconds, 10) || 300;
const maxEntries = parseInt(document.body.dataset.maxEntries, 10) || 500;

const view = new WheelView(document.getElementById('canvas'));
const entriesNode = document.getElementById('entries');
const entryCountNode = document.getElementById('entry-count');
const selectedCountNode = document.getElementById('selected-count');
const titleField = document.getElementById('title');
const secondsRange = document.getElementById('spin-seconds');
const secondsNumber = document.getElementById('spin-seconds-value');
const startButton = document.getElementById('start');

let wheel = null;
const selected = new Set();

// The slider stays coarse; the number field covers the full allowed range.
secondsRange.min = minSeconds;
secondsRange.max = Math.min(60, maxSeconds);

function selectedEntries() {
  return wheel.entries.filter(function (entry) {
    return selected.has(entry.id);
  });
}

// Swatch nodes are kept by entry id so a colour change never has to rebuild the
// list, which would take the focus out from under whoever is editing it.
const swatches = new Map();

// refresh redraws the preview and recolours the list. A colour belongs to a
// position on the wheel, so the list uses each entry's position among the
// selected ones to stay in step with the drawing.
function refresh() {
  const chosen = selectedEntries();
  view.setEntries(chosen);

  const colors = new Map();
  chosen.forEach(function (entry, index) {
    colors.set(entry.id, sliceColor(index, chosen.length));
  });

  entryCountNode.textContent = wheel.entries.length;
  selectedCountNode.textContent = chosen.length;
  startButton.disabled = chosen.length < 2;

  swatches.forEach(function (node, id) {
    node.style.background = colors.get(id) || '';
  });
}

// render rebuilds the list; use it only when entries are added or removed.
function render() {
  swatches.clear();
  clear(entriesNode);
  wheel.entries.forEach(function (entry) {
    entriesNode.appendChild(entryRow(entry));
  });
  refresh();
}

function entryRow(entry) {
  const swatch = el('span', { class: 'swatch' });
  swatches.set(entry.id, swatch);

  const tick = el('input', { type: 'checkbox', 'aria-label': 'Include ' + entry.label });
  tick.checked = selected.has(entry.id);
  tick.addEventListener('change', function () {
    if (tick.checked) selected.add(entry.id);
    else selected.delete(entry.id);
    refresh();
  });

  const label = el('input', { type: 'text', maxlength: '120', 'aria-label': 'Entry name' });
  label.value = entry.label;
  label.addEventListener('change', function () {
    save(label, { label: label.value }, entry, function (updated) {
      entry.label = updated.label;
      label.value = updated.label;
    });
  });

  const weight = el('input', {
    type: 'number', class: 'narrow', min: '0.01', step: '0.01', 'aria-label': 'Weight'
  });
  weight.value = formatWeight(entry.weight);
  weight.addEventListener('change', function () {
    const parsed = parseFloat(weight.value);
    if (!isFinite(parsed)) {
      weight.value = formatWeight(entry.weight);
      return;
    }
    save(weight, { weight: parsed }, entry, function (updated) {
      entry.weight = updated.weight;
      weight.value = formatWeight(updated.weight);
    });
  });

  const remove = el('button', {
    class: 'icon', type: 'button', text: 'x', title: 'Remove entry',
    'aria-label': 'Remove ' + entry.label,
    onclick: async function () {
      try {
        await api('DELETE', '/api/entries/' + entry.id);
        wheel.entries = wheel.entries.filter(function (other) {
          return other.id !== entry.id;
        });
        selected.delete(entry.id);
        render();
      } catch (err) {
        reportError(err);
      }
    }
  });

  return el('div', { class: 'entry' }, [swatch, tick, label, weight, remove]);
}

async function save(field, patch, entry, apply) {
  field.disabled = true;
  try {
    apply(await api('PATCH', '/api/entries/' + entry.id, patch));
    notice('saved');
    refresh();
  } catch (err) {
    reportError(err);
  } finally {
    field.disabled = false;
  }
}

function renderSettings() {
  titleField.value = wheel.title;
  secondsRange.value = Math.min(wheel.spin_seconds, Number(secondsRange.max));
  secondsNumber.value = wheel.spin_seconds;
  document.querySelectorAll('input[name=mode]').forEach(function (radio) {
    radio.checked = radio.value === wheel.mode;
  });
}

const saveSettings = debounce(async function (patch) {
  try {
    const updated = await api('PATCH', '/api/wheels/' + wheelId, patch);
    wheel.title = updated.title;
    wheel.mode = updated.mode;
    wheel.spin_seconds = updated.spin_seconds;
    notice('saved');
  } catch (err) {
    reportError(err);
  }
}, 400);

titleField.addEventListener('input', function () {
  saveSettings({ title: titleField.value });
});

function applySeconds(value) {
  let seconds = parseInt(value, 10);
  if (!isFinite(seconds)) return;
  seconds = Math.min(maxSeconds, Math.max(minSeconds, seconds));
  secondsNumber.value = seconds;
  secondsRange.value = Math.min(seconds, Number(secondsRange.max));
  saveSettings({ spin_seconds: seconds });
}

secondsRange.addEventListener('input', function () {
  applySeconds(secondsRange.value);
});
secondsNumber.addEventListener('change', function () {
  applySeconds(secondsNumber.value);
});

document.querySelectorAll('input[name=mode]').forEach(function (radio) {
  radio.addEventListener('change', function () {
    if (radio.checked) saveSettings({ mode: radio.value });
  });
});

const labelInput = document.getElementById('entry-label');
const weightInput = document.getElementById('entry-weight');

async function addEntry() {
  const label = labelInput.value.trim();
  if (!label) {
    labelInput.focus();
    return;
  }
  if (wheel.entries.length >= maxEntries) {
    reportError(new Error('A wheel holds at most ' + maxEntries + ' entries.'));
    return;
  }

  const weight = parseFloat(weightInput.value);
  try {
    const entry = await api('POST', '/api/wheels/' + wheelId + '/entries', {
      label: label,
      weight: isFinite(weight) ? weight : 1
    });
    wheel.entries.push(entry);
    selected.add(entry.id);
    labelInput.value = '';
    weightInput.value = '1';
    labelInput.focus();
    render();
  } catch (err) {
    reportError(err);
  }
}

document.getElementById('add-entry').addEventListener('click', addEntry);
labelInput.addEventListener('keydown', function (event) {
  if (event.key === 'Enter') addEntry();
});

document.getElementById('add-bulk').addEventListener('click', async function () {
  const box = document.getElementById('bulk');
  if (!box.value.trim()) return;
  try {
    const added = await api('POST', '/api/wheels/' + wheelId + '/entries/bulk', { text: box.value });
    added.forEach(function (entry) {
      wheel.entries.push(entry);
      selected.add(entry.id);
    });
    box.value = '';
    notice('added ' + added.length);
    render();
  } catch (err) {
    reportError(err);
  }
});

document.getElementById('select-all').addEventListener('click', function () {
  wheel.entries.forEach(function (entry) {
    selected.add(entry.id);
  });
  setAllTicks(true);
});

document.getElementById('select-none').addEventListener('click', function () {
  selected.clear();
  setAllTicks(false);
});

function setAllTicks(checked) {
  entriesNode.querySelectorAll('input[type=checkbox]').forEach(function (box) {
    box.checked = checked;
  });
  refresh();
}

document.getElementById('delete').addEventListener('click', async function () {
  if (!window.confirm('Delete this wheel and all of its entries?')) return;
  try {
    await api('DELETE', '/api/wheels/' + wheelId);
    window.location.href = '/';
  } catch (err) {
    reportError(err);
  }
});

startButton.addEventListener('click', async function () {
  const ids = selectedEntries().map(function (entry) {
    return entry.id;
  });
  if (ids.length < 2) return;

  const mode = document.querySelector('input[name=mode]:checked');
  startButton.disabled = true;
  try {
    const play = await api('POST', '/api/wheels/' + wheelId + '/plays', {
      entry_ids: ids,
      mode: mode ? mode.value : wheel.mode,
      spin_seconds: parseInt(secondsNumber.value, 10)
    });
    window.location.href = play.path;
  } catch (err) {
    startButton.disabled = false;
    reportError(err);
  }
});

api('GET', '/api/wheels/' + wheelId).then(function (loaded) {
  wheel = loaded;
  wheel.entries.forEach(function (entry) {
    selected.add(entry.id);
  });
  renderSettings();
  render();
}).catch(function (err) {
  document.querySelector('main').replaceChildren(
    el('section', { class: 'panel' }, [
      el('h1', { text: 'This wheel is not available' }),
      el('p', { class: 'muted', text: err.message }),
      el('p', {}, [el('a', { href: '/', text: 'Back to your wheels' })])
    ])
  );
});
