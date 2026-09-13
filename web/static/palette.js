'use strict';

/* Slice colours. Neighbouring slices must be easy to tell apart, and the same
   entry must get the same colour in the wheel and in the list beside it, so the
   colour is a pure function of the entry's position. */

const GOLDEN_ANGLE = 137.508;

// Three (saturation, lightness) pairs, cycled so that even a repeated hue lands
// on a different shade.
const SHADES = [
  { s: 63, l: 55 },
  { s: 76, l: 43 },
  { s: 52, l: 67 }
];

function sliceShade(index, count) {
  let cycle = index % SHADES.length;
  // The last slice sits next to the first one on the wheel, so nudge it onto a
  // different shade when the cycle would otherwise repeat.
  if (count > 1 && index === count - 1 && cycle === 0) cycle = 1;
  return SHADES[cycle];
}

function sliceColor(index, count) {
  const shade = sliceShade(index, count);
  const hue = (index * GOLDEN_ANGLE + 24) % 360;
  return 'hsl(' + hue.toFixed(1) + ' ' + shade.s + '% ' + shade.l + '%)';
}

// sliceInk returns a text colour that stays readable on top of sliceColor.
function sliceInk(index, count) {
  const shade = sliceShade(index, count);
  const hue = (index * GOLDEN_ANGLE + 24) % 360;
  return relativeLuminance(hue, shade.s / 100, shade.l / 100) > 0.38 ? '#101318' : '#f7f9fc';
}

function relativeLuminance(h, s, l) {
  const rgb = hslToRgb(h, s, l);
  const channel = function (v) {
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(rgb[0]) + 0.7152 * channel(rgb[1]) + 0.0722 * channel(rgb[2]);
}

function hslToRgb(h, s, l) {
  const c = (1 - Math.abs(2 * l - 1)) * s;
  const hp = (h % 360) / 60;
  const x = c * (1 - Math.abs((hp % 2) - 1));
  let rgb;
  if (hp < 1) rgb = [c, x, 0];
  else if (hp < 2) rgb = [x, c, 0];
  else if (hp < 3) rgb = [0, c, x];
  else if (hp < 4) rgb = [0, x, c];
  else if (hp < 5) rgb = [x, 0, c];
  else rgb = [c, 0, x];
  const m = l - c / 2;
  return [rgb[0] + m, rgb[1] + m, rgb[2] + m];
}
