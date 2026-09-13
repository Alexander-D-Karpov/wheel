'use strict';

const listNode = document.getElementById('wheel-list');
const titleNode = document.getElementById('new-title');
const createButton = document.getElementById('create');

function describe(wheel) {
  const mode = wheel.mode === 'exclusion' ? 'exclusion' : 'selection';
  const plural = wheel.entry_count === 1 ? 'entry' : 'entries';
  return wheel.entry_count + ' ' + plural + ', ' + mode + ', ' + wheel.spin_seconds + 's spin';
}

async function load() {
  const wheels = await api('GET', '/api/wheels');
  clear(listNode);

  if (!wheels.length) {
    listNode.appendChild(el('p', { class: 'muted', text: 'No wheels yet. Create one above.' }));
    return;
  }
  wheels.forEach(function (wheel) {
    listNode.appendChild(el('a', { class: 'tile', href: '/wheel/' + wheel.id }, [
      el('strong', { text: wheel.title }),
      el('span', { class: 'muted', text: describe(wheel) })
    ]));
  });
}

async function create() {
  createButton.disabled = true;
  try {
    const wheel = await api('POST', '/api/wheels', { title: titleNode.value });
    window.location.href = '/wheel/' + wheel.id;
  } catch (err) {
    createButton.disabled = false;
    reportError(err);
  }
}

createButton.addEventListener('click', create);
titleNode.addEventListener('keydown', function (event) {
  if (event.key === 'Enter') create();
});

load().catch(reportError);
