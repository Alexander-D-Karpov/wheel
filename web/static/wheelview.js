'use strict';

/* Canvas rendering of the wheel.

   The animation is driven entirely by server time: given the starting angle,
   the total travel, the start timestamp and the duration, every browser
   computes the same angle for the same instant. Nothing here decides an
   outcome, and a tab that was asleep simply resumes at the right position. */

const TAU = Math.PI * 2;
const MAX_WHEEL_PX = 520;
const MIN_WHEEL_PX = 220;

function easeOutQuart(t) {
  return 1 - Math.pow(1 - t, 4);
}

function themeColor(name, fallback) {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name);
  return value ? value.trim() : fallback;
}

class WheelView {
  constructor(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.entries = [];
    this.rotation = 0;
    this.winnerId = null;
    this.animation = null;
    this.size = MIN_WHEEL_PX;

    this.tick = this.tick.bind(this);

    const refresh = () => {
      this.measure();
      this.draw();
    };
    if (typeof ResizeObserver === 'function') {
      new ResizeObserver(refresh).observe(canvas.parentElement);
    }
    window.addEventListener('resize', refresh);
    // A theme switch changes the frame and label colours, not the geometry.
    const scheme = window.matchMedia('(prefers-color-scheme: dark)');
    if (scheme.addEventListener) scheme.addEventListener('change', () => this.draw());

    this.measure();
  }

  measure() {
    const available = this.canvas.parentElement.clientWidth || MIN_WHEEL_PX;
    const byHeight = Math.max(MIN_WHEEL_PX, window.innerHeight * 0.62);
    const size = Math.round(Math.max(MIN_WHEEL_PX, Math.min(available, byHeight, MAX_WHEEL_PX)));
    if (size === this.size && this.canvas.width) return;

    const ratio = window.devicePixelRatio || 1;
    this.size = size;
    this.canvas.style.width = size + 'px';
    this.canvas.style.height = size + 'px';
    this.canvas.width = Math.round(size * ratio);
    this.canvas.height = Math.round(size * ratio);
    this.ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  }

  setEntries(entries) {
    this.entries = (entries || []).slice();
    this.draw();
  }

  setRotation(rotation) {
    this.animation = null;
    this.rotation = rotation || 0;
    this.draw();
  }

  setWinner(entryId) {
    this.winnerId = entryId || null;
    this.draw();
  }

  get spinning() {
    return this.animation !== null;
  }

  // spin replays a server-issued spin. clock.now() must return server time.
  spin(options) {
    this.winnerId = null;
    this.animation = {
      from: options.from,
      delta: options.delta,
      startedAt: options.startedAt,
      durationMs: options.durationMs > 0 ? options.durationMs : 1,
      clock: options.clock,
      onDone: options.onDone || null
    };
    requestAnimationFrame(this.tick);
  }

  cancel() {
    this.animation = null;
  }

  tick() {
    const a = this.animation;
    if (!a) return;

    let t = (a.clock.now() - a.startedAt) / a.durationMs;
    if (t < 0) t = 0;
    if (t > 1) t = 1;

    this.rotation = a.from + easeOutQuart(t) * a.delta;
    this.draw();

    if (t < 1) {
      requestAnimationFrame(this.tick);
      return;
    }
    this.animation = null;
    if (a.onDone) a.onDone();
  }

  draw() {
    const ctx = this.ctx;
    const size = this.size;
    const center = size / 2;
    const radius = center - 8;

    ctx.clearRect(0, 0, size, size);

    const frame = themeColor('--border-strong', '#888');
    const surface = themeColor('--surface', '#fff');
    const muted = themeColor('--muted', '#777');

    if (!this.entries.length) {
      ctx.beginPath();
      ctx.arc(center, center, radius, 0, TAU);
      ctx.fillStyle = themeColor('--surface-sunken', '#eee');
      ctx.fill();
      ctx.lineWidth = 2;
      ctx.strokeStyle = frame;
      ctx.stroke();

      ctx.fillStyle = muted;
      ctx.font = '15px system-ui, sans-serif';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText('no entries', center, center);
      return;
    }

    const count = this.entries.length;
    let total = 0;
    for (const entry of this.entries) total += entry.weight > 0 ? entry.weight : 0.01;

    ctx.save();
    ctx.translate(center, center);
    ctx.rotate(this.rotation);

    let accumulated = 0;
    this.entries.forEach((entry, index) => {
      const weight = entry.weight > 0 ? entry.weight : 0.01;
      const start = (accumulated / total) * TAU;
      const end = ((accumulated + weight) / total) * TAU;
      accumulated += weight;

      ctx.beginPath();
      if (count === 1) {
        // A lone entry fills the disc; a wedge path would leave a seam.
        ctx.arc(0, 0, radius, 0, TAU);
      } else {
        ctx.moveTo(0, 0);
        ctx.arc(0, 0, radius, start, end);
        ctx.closePath();
      }
      ctx.fillStyle = sliceColor(index, count);
      ctx.fill();

      // A hairline keeps neighbouring slices separate even when the wheel is small.
      if (count > 1) {
        ctx.lineWidth = 1;
        ctx.strokeStyle = surface;
        ctx.stroke();
      }

      const span = end - start;
      if (span > 0.055) {
        const middle = start + span / 2;
        this.drawLabel(entry.label, middle, radius, span, sliceInk(index, count), this.rotation + middle);
      }
    });

    if (this.winnerId) {
      this.outlineWinner(radius, total);
    }
    ctx.restore();

    // Rim and hub, drawn after the slices so nothing overlaps them.
    ctx.beginPath();
    ctx.arc(center, center, radius, 0, TAU);
    ctx.lineWidth = 2;
    ctx.strokeStyle = frame;
    ctx.stroke();

    const hub = Math.max(14, radius * 0.11);
    ctx.beginPath();
    ctx.arc(center, center, hub, 0, TAU);
    ctx.fillStyle = surface;
    ctx.fill();
    ctx.lineWidth = 2;
    ctx.strokeStyle = frame;
    ctx.stroke();
  }

  // drawLabel writes one slice label along its radius. On the left of the wheel
  // that direction points backwards, so the text is turned over to stay
  // readable; absoluteAngle is what decides, which means labels stay upright
  // wherever the wheel comes to rest.
  drawLabel(label, angle, radius, span, ink, absoluteAngle) {
    const ctx = this.ctx;
    const facing = ((absoluteAngle % TAU) + TAU) % TAU;
    const upsideDown = facing > Math.PI / 2 && facing < (3 * Math.PI) / 2;

    ctx.save();
    ctx.rotate(angle);
    if (upsideDown) ctx.rotate(Math.PI);
    ctx.textAlign = upsideDown ? 'left' : 'right';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = ink;

    const fontSize = Math.max(10, Math.min(17, radius * span * 0.62, 17));
    ctx.font = '600 ' + fontSize.toFixed(1) + 'px system-ui, sans-serif';

    const maxWidth = radius - Math.max(20, radius * 0.16);
    let text = label;
    if (ctx.measureText(text).width > maxWidth) {
      while (text.length > 1 && ctx.measureText(text + '...').width > maxWidth) {
        text = text.slice(0, -1);
      }
      text += '...';
    }
    ctx.fillText(text, upsideDown ? -(radius - 10) : radius - 10, 0);
    ctx.restore();
  }

  outlineWinner(radius, total) {
    const ctx = this.ctx;
    let accumulated = 0;
    for (const entry of this.entries) {
      const weight = entry.weight > 0 ? entry.weight : 0.01;
      const start = (accumulated / total) * TAU;
      const end = ((accumulated + weight) / total) * TAU;
      accumulated += weight;
      if (entry.id !== this.winnerId) continue;

      ctx.beginPath();
      if (this.entries.length === 1) {
        ctx.arc(0, 0, radius - 1, 0, TAU);
      } else {
        ctx.moveTo(0, 0);
        ctx.arc(0, 0, radius - 1, start, end);
        ctx.closePath();
      }
      ctx.lineWidth = 3;
      ctx.strokeStyle = themeColor('--text', '#000');
      ctx.stroke();
      return;
    }
  }
}
