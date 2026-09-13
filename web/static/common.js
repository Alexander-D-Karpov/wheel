'use strict';

/* Shared helpers: the JSON client, small DOM builders and the clock the whole
   synchronisation scheme rests on. */

async function api(method, url, body) {
  const options = {
    method: method,
    credentials: 'same-origin',
    headers: { Accept: 'application/json' }
  };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  let response;
  try {
    response = await fetch(url, options);
  } catch (err) {
    throw new Error('The server is not reachable.');
  }

  if (response.status === 204) return null;

  const text = await response.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch (err) {
      data = null;
    }
  }
  if (!response.ok) {
    throw new Error((data && data.error) || ('Request failed with status ' + response.status));
  }
  return data;
}

function el(tag, props, children) {
  const node = document.createElement(tag);
  if (props) {
    Object.keys(props).forEach(function (key) {
      const value = props[key];
      if (key === 'class') node.className = value;
      else if (key === 'text') node.textContent = value;
      else if (key === 'html') throw new Error('raw html is not allowed here');
      else if (key.slice(0, 2) === 'on') node.addEventListener(key.slice(2), value);
      else if (value === true) node.setAttribute(key, '');
      else if (value !== false && value != null) node.setAttribute(key, value);
    });
  }
  (children || []).forEach(function (child) {
    if (child) node.appendChild(child);
  });
  return node;
}

function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

function debounce(fn, ms) {
  let timer = null;
  return function () {
    const args = arguments;
    const self = this;
    clearTimeout(timer);
    timer = setTimeout(function () {
      fn.apply(self, args);
    }, ms);
  };
}

let noticeTimer = null;

function notice(message, kind) {
  const node = document.getElementById('notice');
  if (!node) return;
  node.textContent = message;
  node.className = 'notice' + (kind ? ' is-' + kind : '');
  clearTimeout(noticeTimer);
  if (message) {
    noticeTimer = setTimeout(function () {
      node.textContent = '';
      node.className = 'notice';
    }, 2500);
  }
}

function reportError(err) {
  const message = (err && err.message) || String(err);
  notice(message, 'lost');
  window.alert(message);
}

function formatClock(ms) {
  const d = new Date(ms);
  const pad = function (n) { return n < 10 ? '0' + n : String(n); };
  return pad(d.getHours()) + ':' + pad(d.getMinutes()) + ':' + pad(d.getSeconds());
}

function formatWeight(weight) {
  return Number(weight.toFixed(2)).toString();
}

/* serverClock keeps this browser's idea of the server's clock. Every spin is
   described in server time, so an accurate offset is what makes two people on
   different machines see the wheel in the same position. */
const serverClock = {
  offset: 0,
  bestRtt: Infinity,
  started: false,

  now: function () {
    return Date.now() + this.offset;
  },

  // observe folds in a timestamp that arrived with a message. It is only
  // trusted when nothing better is known, since one-way delay is unknown.
  observe: function (serverNow) {
    if (typeof serverNow !== 'number' || !isFinite(serverNow)) return;
    if (this.bestRtt !== Infinity) return;
    this.offset = serverNow - Date.now();
  },

  sample: async function () {
    const sent = Date.now();
    let data;
    try {
      data = await api('GET', '/api/time');
    } catch (err) {
      return;
    }
    const received = Date.now();
    const rtt = received - sent;
    if (!data || typeof data.now !== 'number') return;

    // The server read its clock somewhere in the middle of the round trip.
    if (rtt <= this.bestRtt) {
      this.bestRtt = rtt;
      this.offset = data.now - (sent + rtt / 2);
    }
  },

  start: async function () {
    if (this.started) return;
    this.started = true;
    for (let i = 0; i < 3; i++) await this.sample();
    const self = this;
    setInterval(function () {
      // Let the best-round-trip record fade so a temporary network hiccup does
      // not freeze the estimate forever.
      self.bestRtt = self.bestRtt * 1.3 + 5;
      self.sample();
    }, 60000);
  }
};
