<template>
  <canvas ref="cv" class="pointer-events-none" :style="{ width: size + 'px', height: size + 'px' }" />
</template>

<script setup>
// A traffic-speed-driven glow around the power button. Intensity = 0.22 +
// 0.78*sqrt(min(1, speed/8MB/s)); an outer halo, three slowly orbiting soft blobs (5.6s
// period) and a pulsing ring, all eased. The palette runs green (inbound) -> blue
// (outbound): the three blobs take green, a teal midpoint and blue, and the rings are
// stroked with a green-to-blue gradient.
import { onMounted, onUnmounted, ref, watch } from 'vue';

const props = defineProps({
  connected: { type: Boolean, default: false },
  bytesPerSecond: { type: Number, default: 0 },
  size: { type: Number, default: 248 },
  colorA: { type: String, default: '#16b877' }, // green - inbound
  colorB: { type: String, default: '#2f7ff0' }, // blue - outbound
});

const MAX_SPEED = 8 * 1024 * 1024;
const DURATION = 5600;
const TWO_PI = Math.PI * 2;

const cv = ref(null);
let ctx = null;
let raf = 0;
let startTs = 0;
let phase = 0;
let displayed = 0;
let target = 0;
let rgbA = { r: 22, g: 184, b: 119 };
let rgbB = { r: 47, g: 127, b: 240 };

function parseColor(hex, fallback) {
  const m = /^#?([0-9a-f]{6})$/i.exec((hex || '').trim());
  if (!m) return fallback;
  const n = parseInt(m[1], 16);
  return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
}

function mix(t) {
  return {
    r: rgbA.r + (rgbB.r - rgbA.r) * t,
    g: rgbA.g + (rgbB.g - rgbA.g) * t,
    b: rgbA.b + (rgbB.b - rgbA.b) * t,
  };
}

function setupCanvas() {
  const c = cv.value;
  const dpr = window.devicePixelRatio || 1;
  c.width = Math.round(props.size * dpr);
  c.height = Math.round(props.size * dpr);
  ctx = c.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

function computeTarget() {
  const speed = Math.max(0, props.bytesPerSecond);
  const factor = speed <= 0 ? 0 : Math.sqrt(Math.min(1, speed / MAX_SPEED));
  target = props.connected ? 0.22 + 0.78 * factor : 0;
}

function rgba(c, a) {
  return `rgba(${c.r | 0},${c.g | 0},${c.b | 0},${Math.max(0, Math.min(255, a)) / 255})`;
}

function softCircle(x, y, radius, alpha, color) {
  for (let i = 5; i >= 1; i--) {
    const layer = i / 5;
    ctx.fillStyle = rgba(color, (alpha * (1 - layer * 0.12)) / i);
    ctx.beginPath();
    ctx.arc(x, y, radius * layer, 0, TWO_PI);
    ctx.fill();
  }
}

function orbitBlob(cx, cy, orbitRadius, orbitPhase, vScale, radius, alpha, color) {
  const angle = orbitPhase * TWO_PI;
  const x = cx + Math.cos(angle) * orbitRadius;
  const y = cy + Math.sin(angle) * orbitRadius * vScale;
  softCircle(x, y, radius, alpha, color);
}

// A green->blue gradient across the ring, at the given per-layer alpha.
function ringGradient(cx, cy, radius, alpha) {
  const g = ctx.createLinearGradient(cx - radius, cy - radius, cx + radius, cy + radius);
  g.addColorStop(0, rgba(rgbA, alpha));
  g.addColorStop(1, rgba(rgbB, alpha));
  return g;
}

function outerRing(cx, cy, radius, intensity) {
  for (let i = 4; i >= 1; i--) {
    const layer = i / 4;
    ctx.lineWidth = 7 + 10 * layer;
    ctx.strokeStyle = ringGradient(cx, cy, radius + layer, (36 * intensity) / i);
    ctx.beginPath();
    ctx.arc(cx, cy, radius + layer, 0, TWO_PI);
    ctx.stroke();
  }
}

function draw() {
  const s = props.size;
  ctx.clearRect(0, 0, s, s);
  if (!props.connected || displayed <= 0.01) return;
  const intensity = displayed;
  const cx = s / 2;
  const cy = s / 2;
  const buttonRadius = Math.min(90, s * 0.42);
  const haloRadius = buttonRadius + (2 + 5 * intensity);
  const orbitRadius = buttonRadius + (2 + 4 * intensity);

  outerRing(cx, cy, haloRadius, intensity);
  orbitBlob(cx, cy, orbitRadius, phase, 0.94, 16 + 14 * intensity, 78 * intensity, rgbA);
  orbitBlob(cx, cy, orbitRadius * 0.99, phase + 0.37, 0.92, 14 + 10 * intensity, 54 * intensity, mix(0.5));
  orbitBlob(cx, cy, orbitRadius * 0.98, phase + 0.68, 0.96, 11 + 8 * intensity, 38 * intensity, rgbB);

  const pulse = 0.5 + 0.5 * Math.sin(phase * TWO_PI);
  ctx.lineWidth = 1.5 + 3 * displayed;
  ctx.strokeStyle = ringGradient(cx, cy, haloRadius + (1 + 4 * pulse), 70 * intensity);
  ctx.beginPath();
  ctx.arc(cx, cy, haloRadius + (1 + 4 * pulse), 0, TWO_PI);
  ctx.stroke();
}

function frame(now) {
  if (!startTs) startTs = now;
  phase = ((now - startTs) % DURATION) / DURATION;
  displayed += (target - displayed) * 0.075;
  if (Math.abs(target - displayed) < 0.005) displayed = target;
  draw();
  raf = requestAnimationFrame(frame);
}

function startLoop() {
  if (raf) return;
  startTs = 0;
  raf = requestAnimationFrame(frame);
}

function stopLoop() {
  if (raf) cancelAnimationFrame(raf);
  raf = 0;
}

watch(
  () => [props.connected, props.bytesPerSecond],
  () => {
    computeTarget();
    if (props.connected) {
      startLoop();
    } else {
      displayed = 0;
      if (ctx) ctx.clearRect(0, 0, props.size, props.size);
      stopLoop();
    }
  },
);

onMounted(() => {
  rgbA = parseColor(props.colorA, { r: 22, g: 184, b: 119 });
  rgbB = parseColor(props.colorB, { r: 47, g: 127, b: 240 });
  setupCanvas();
  computeTarget();
  if (props.connected) startLoop();
});

onUnmounted(stopLoop);
</script>
