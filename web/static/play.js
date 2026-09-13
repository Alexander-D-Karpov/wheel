'use strict';

const slug = document.body.dataset.slug;
const baseUrl = document.body.dataset.baseUrl || window.location.origin;

const view = new WheelView(document.getElementById('canvas'));
const bannerNode = document.getElementById('banner');
const entriesNode = document.getElementById('entries');
const historyNode = document.getElementById('history');
const connectionNode = document.getElementById('connection');
const viewersNode = document.getElementById('viewers');
const ownerControls = document.getElementById('owner-controls');
const spinButton = document.getElementById('spin');
const resetButton = document.getElementById('reset');

let state = null;
let isOwner = false;
let animatingSpinId = null;
let pendingResult = null;

/* --- rendering ---------------------------------------------------------- */

function setBanner(text, kind) {
  if (!text) {
    bannerNode.textContent = '';
    bannerNode.className = 'banner is-hidden';
    return;
  }
  bannerNode.textContent = text;
  bannerNode.className = 'banner is-' + kind;
}

function activeEntries() {
  return state.entries.filter(function (entry) {
    return entry.active;
  });
}

function colorMap() {
  const active = activeEntries();
  const colors = new Map();
  active.forEach(function (entry, index) {
    colors.set(entry.id, sliceColor(index, active.length));
  });
  return colors;
}

function renderDetails() {
  document.getElementById('play-title').textContent = state.title;
  document.getElementById('fact-mode').textContent = state.mode === 'exclusion'
    ? 'Exclusion, the picked entry drops out'
    : 'Selection, nothing is removed';
  document.getElementById('fact-seconds').textContent = state.spin_seconds + ' seconds';
  document.getElementById('fact-round').textContent = state.round;

  const active = activeEntries();
  const colors = colorMap();
  document.getElementById('active-count').textContent = active.length;
  document.getElementById('total-count').textContent = state.entries.length;

  clear(entriesNode);
  state.entries.forEach(function (entry) {
    const swatch = el('span', { class: 'swatch' });
    const color = colors.get(entry.id);
    if (color) swatch.style.background = color;

    const classes = ['entry'];
    if (!entry.active) classes.push('is-out');
    if (state.winner && state.winner.id === entry.id && state.status === 'finished') {
      classes.push('is-winner');
    }

    entriesNode.appendChild(el('div', { class: classes.join(' ') }, [
      swatch,
      el('span', { class: 'entry-name', text: entry.label }),
      el('span', { class: 'entry-weight', text: 'x' + formatWeight(entry.weight) })
    ]));
  });

  clear(historyNode);
  if (!state.history.length) {
    historyNode.appendChild(el('li', { class: 'muted no-marker', text: 'No spins yet.' }));
  }
  const verb = state.mode === 'exclusion' ? 'out' : 'picked';
  state.history.forEach(function (item) {
    historyNode.appendChild(el('li', {}, [
      el('span', { text: item.winner_label }),
      el('span', { class: 'muted', text: ' — ' + verb + ', ' + formatClock(item.at) })
    ]));
  });

  renderControls();
}

function renderControls() {
  ownerControls.classList.toggle('is-hidden', !isOwner);
  if (!isOwner) return;

  spinButton.disabled = state.status !== 'idle';
  spinButton.textContent = state.status === 'spinning'
    ? 'Spinning'
    : (state.status === 'finished' ? 'Finished' : 'Spin');
  resetButton.disabled = state.status === 'spinning';
}

function restingBanner() {
  if (state.status === 'finished' && state.winner) {
    setBanner('Winner: ' + state.winner.label, 'win');
    view.setWinner(state.winner.id);
    return;
  }
  if (state.status === 'idle' && state.winner) {
    // Outside exclusion mode the last pick is the winner; inside it, the last
    // pick is the entry that just went out.
    if (state.mode === 'exclusion') {
      setBanner(state.winner.label + ' is out.', 'out');
      view.setWinner(null);
    } else {
      setBanner('Winner: ' + state.winner.label, 'win');
      view.setWinner(state.winner.id);
    }
    return;
  }
  setBanner('', '');
  view.setWinner(null);
}

/* --- state transitions -------------------------------------------------- */

function adopt(next) {
  // Broadcast payloads carry no owner flag, so the one from the authenticated
  // request is kept for the life of the page.
  if (next.is_owner) isOwner = true;
  next.is_owner = isOwner;
  state = next;
  serverClock.observe(next.server_now);
}

function applyState(next) {
  adopt(next);
  renderDetails();

  if (state.status === 'spinning' && state.spin) {
    startSpin(state.spin);
    return;
  }

  animatingSpinId = null;
  view.cancel();
  view.setEntries(activeEntries());
  view.setRotation(state.rotation);
  restingBanner();
}

function startSpin(spin) {
  // Reconnecting mid-spin must not restart the animation from the beginning.
  if (animatingSpinId === spin.id && view.spinning) return;

  animatingSpinId = spin.id;
  pendingResult = null;
  view.setWinner(null);
  view.setEntries(spin.layout);
  setBanner('Spinning', 'running');

  view.spin({
    from: spin.from,
    delta: spin.delta,
    startedAt: spin.started_at,
    durationMs: spin.duration_ms,
    clock: serverClock,
    onDone: function () {
      animatingSpinId = null;
      if (pendingResult) {
        const result = pendingResult;
        pendingResult = null;
        applyResult(result);
      }
    }
  });
}

function applyResult(message) {
  adopt(message.state);
  renderDetails();

  view.setEntries(activeEntries());
  view.setRotation(state.rotation);

  if (message.finished && state.winner) {
    setBanner(message.picked_label + ' is out. ' + state.winner.label + ' wins.', 'win');
    view.setWinner(state.winner.id);
  } else if (message.eliminated) {
    setBanner(message.picked_label + ' is out.', 'out');
    view.setWinner(null);
  } else {
    setBanner('Winner: ' + message.picked_label, 'win');
    view.setWinner(message.picked_id);
  }
}

/* --- live stream -------------------------------------------------------- */

function connect() {
  const source = new EventSource('/api/plays/' + encodeURIComponent(slug) + '/events');

  source.addEventListener('open', function () {
    connectionNode.textContent = 'live';
    connectionNode.className = 'notice is-live';
  });

  source.addEventListener('error', function () {
    connectionNode.textContent = 'reconnecting';
    connectionNode.className = 'notice is-lost';
  });

  source.addEventListener('state', function (event) {
    applyState(JSON.parse(event.data).state);
  });

  source.addEventListener('spin', function (event) {
    const message = JSON.parse(event.data);
    serverClock.observe(message.server_now);
    state.status = 'spinning';
    state.round = message.round;
    renderDetails();
    startSpin(message.spin);
  });

  source.addEventListener('result', function (event) {
    const message = JSON.parse(event.data);
    if (view.spinning) pendingResult = message;
    else applyResult(message);
  });

  source.addEventListener('viewers', function (event) {
    viewersNode.textContent = 'Viewers: ' + JSON.parse(event.data).count;
  });

  source.addEventListener('ping', function (event) {
    serverClock.observe(JSON.parse(event.data).server_now);
  });
}

/* --- owner actions ------------------------------------------------------ */

spinButton.addEventListener('click', async function () {
  spinButton.disabled = true;
  try {
    // The animation is started by the broadcast, so the owner and the audience
    // work from exactly the same message.
    await api('POST', '/api/plays/' + encodeURIComponent(slug) + '/spin');
  } catch (err) {
    renderControls();
    reportError(err);
  }
});

resetButton.addEventListener('click', async function () {
  if (!window.confirm('Put every entry back and clear the history?')) return;
  resetButton.disabled = true;
  try {
    applyState(await api('POST', '/api/plays/' + encodeURIComponent(slug) + '/reset'));
  } catch (err) {
    reportError(err);
  } finally {
    renderControls();
  }
});

/* --- share link --------------------------------------------------------- */

const shareField = document.getElementById('share');
shareField.value = baseUrl.replace(/\/+$/, '') + '/p/' + slug;

document.getElementById('copy').addEventListener('click', async function () {
  const button = document.getElementById('copy');
  try {
    await navigator.clipboard.writeText(shareField.value);
  } catch (err) {
    shareField.focus();
    shareField.select();
    return;
  }
  button.textContent = 'Copied';
  setTimeout(function () {
    button.textContent = 'Copy';
  }, 1500);
});

/* --- start -------------------------------------------------------------- */

api('GET', '/api/plays/' + encodeURIComponent(slug)).then(function (initial) {
  applyState(initial);
  // Subscribe first so the page goes live straight away; the clock refines
  // itself in the background and is only needed once a spin starts.
  connect();
  serverClock.start();
}).catch(function (err) {
  document.querySelector('main').replaceChildren(
    el('section', { class: 'panel' }, [
      el('h1', { text: 'This play session is not available' }),
      el('p', { class: 'muted', text: err.message }),
      el('p', {}, [el('a', { href: '/', text: 'Back to your wheels' })])
    ])
  );
});
