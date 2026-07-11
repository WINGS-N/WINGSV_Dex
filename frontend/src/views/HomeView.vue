<template>
  <div class="pb-6">
    <AppHeader />

    <div class="px-4">
      <SamsungCard class="flex flex-col items-center pb-6 pt-7 text-center">
        <span :class="pillClasses">{{ pillText }}</span>

        <!-- 248 slot holds the traffic glow; the 180 button is centred, ~34px pill gap -->
        <div class="relative flex h-[248px] w-[248px] items-center justify-center">
          <PowerGlow
            :connected="isOn"
            :bytes-per-second="glowBps"
            :size="248"
            :color-a="FLOW_DOWN"
            :color-b="FLOW_UP"
            class="absolute inset-0"
          />
          <button
            type="button"
            class="relative flex h-[180px] w-[180px] items-center justify-center rounded-full border transition-colors disabled:opacity-60"
            :class="
              isOn
                ? 'power-on border-transparent text-white'
                : 'border-white/[0.06] bg-wings-surface text-wings-mutedStrong hover:text-wings-text'
            "
            :disabled="busy"
            :aria-label="isOn ? 'Отключиться' : 'Подключиться'"
            @click="toggle"
          >
            <svg viewBox="0 0 24 24" fill="currentColor" class="relative z-[2] h-[96px] w-[96px]">
              <path
                d="M13,3h-2v10h2zM17.83,5.17l-1.42,1.42A6.97,6.97 0,0 1,19 12c0,3.87 -3.13,7 -7,7s-7,-3.13 -7,-7c0,-2.09 0.91,-4 2.41,-5.41L6,5.17A8.96,8.96 0,0 0,3 12c0,4.97 4.03,9 9,9s9,-4.03 9,-9c0,-2.76 -1.24,-5.23 -3.17,-6.83z"
              />
            </svg>
          </button>
        </div>

        <div class="mt-4">
          <p class="text-[17px] text-wings-mutedStrong">{{ hintText }}</p>
          <p class="mt-2.5 text-sm text-wings-muted">{{ endpointText }}</p>
          <p v-if="state.error" class="mt-1 text-sm text-wings-danger">{{ state.error }}</p>
        </div>

        <div class="mt-[22px] flex w-full flex-col gap-3">
          <button
            type="button"
            class="flex h-14 w-full items-center gap-3 rounded-2xl bg-wings-surface px-4"
            :disabled="importing"
            @click="pasteImport"
          >
            <ClipboardPaste :size="20" class="shrink-0 text-wings-accent" />
            <span class="flex-1 text-center text-[15px] font-medium">Добавить из буфера</span>
            <span class="w-5 shrink-0" aria-hidden="true"></span>
          </button>
          <button
            type="button"
            class="flex h-14 w-full items-center gap-3 rounded-2xl bg-wings-surface px-4"
            @click="copyConfig"
          >
            <Copy :size="20" class="shrink-0 text-wings-accent" />
            <span class="flex-1 text-center text-[15px] font-medium">{{
              copied ? 'Скопировано' : 'Скопировать конфигурацию'
            }}</span>
            <span class="w-5 shrink-0" aria-hidden="true"></span>
          </button>
        </div>
      </SamsungCard>

      <div class="mt-3 grid grid-cols-2 gap-3">
        <StatCard title="Downlink" :value="down" :icon="ArrowDown" :icon-color="FLOW_DOWN" />
        <StatCard title="Uplink" :value="up" :icon="ArrowUp" :icon-color="FLOW_UP" />
        <StatCard title="Принято" :value="rx" :icon="ArrowDown" :icon-color="FLOW_DOWN" />
        <StatCard title="Передано" :value="tx" :icon="ArrowUp" :icon-color="FLOW_UP" />
      </div>

      <div class="mt-3">
        <div class="surface-card !p-4">
          <div class="flex items-center justify-between text-wings-muted">
            <span class="text-[13px]">Текущий IP</span>
            <button
              type="button"
              aria-label="Обновить IP"
              class="hover:text-wings-text disabled:opacity-50"
              :disabled="refreshingIP"
              @click="refreshIP"
            >
              <RefreshCw :size="16" :class="{ 'animate-spin': refreshingIP }" />
            </button>
          </div>
          <p class="mt-1.5 text-[19px] font-semibold">{{ ip }}</p>
        </div>
      </div>

      <div class="mt-3 grid grid-cols-2 gap-3">
        <StatCard title="Страна" :value="country" :icon="Globe" />
        <StatCard title="Провайдер" :value="provider" :icon="Radio" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { ArrowDown, ArrowUp, ClipboardPaste, Copy, Globe, Radio, RefreshCw } from 'lucide-vue-next';
import { Clipboard, Events } from '@wailsio/runtime';
import { ConnectionService, ProfilesService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import PowerGlow from '@/components/layout/PowerGlow.vue';
import StatCard from '@/components/layout/StatCard.vue';

const CONNECTION_STATE_EVENT = 'connection:state';
const TRAFFIC_STATS_EVENT = 'connection:stats';
const IP_INFO_EVENT = 'connection:ipinfo';

// Directional flow colours: green = inbound (downlink / received), blue = outbound
// (uplink / sent). Shared by the stat-card arrows and the power-button traffic glow.
const FLOW_DOWN = '#16b877';
const FLOW_UP = '#2f7ff0';

const state = reactive({
  status: 'disconnected',
  streams: 0,
  threads: 0,
  stage: '',
  endpoint: '',
  title: '',
  error: '',
});
const activeEndpoint = ref('');
const importing = ref(false);
const copied = ref(false);
const refreshingIP = ref(false);
const offs = [];

const traffic = reactive({ rxBytes: 0, txBytes: 0, rxRate: 0, txRate: 0 });
const ipinfo = reactive({ ip: '', country: '', countryCode: '', provider: '' });

// Turn an ISO 3166-1 alpha-2 code into its flag emoji (regional indicators).
function flagEmoji(code) {
  const cc = (code || '').trim().toUpperCase();
  if (!/^[A-Z]{2}$/.test(cc)) return '';
  return String.fromCodePoint(...[...cc].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

function formatBytes(n) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = Number(n) || 0;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`;
}

const down = computed(() => `${formatBytes(traffic.rxRate)}/s`);
const up = computed(() => `${formatBytes(traffic.txRate)}/s`);
const rx = computed(() => formatBytes(traffic.rxBytes));
const tx = computed(() => formatBytes(traffic.txBytes));
const ip = computed(() => ipinfo.ip || '—');
const country = computed(() => {
  const label = ipinfo.country || ipinfo.countryCode || '';
  if (!label) return '—';
  const flag = flagEmoji(ipinfo.countryCode);
  return flag ? `${flag} ${label}` : label;
});
const provider = computed(() => ipinfo.provider || '—');

function assign(next) {
  if (!next) return;
  state.status = next.status ?? 'disconnected';
  state.streams = next.streams ?? 0;
  state.threads = next.threads ?? 0;
  state.stage = next.stage ?? '';
  state.endpoint = next.endpoint ?? '';
  state.title = next.title ?? '';
  state.error = next.error ?? '';
}

const isOn = computed(() => state.status === 'connected');
const busy = computed(() => state.status === 'stopping');
// Sum of up/down rates drives the power-button glow intensity.
const glowBps = computed(() => (traffic.rxRate || 0) + (traffic.txRate || 0));

// Home pill: bold, 14/8 padding, radius 28, with a solid per-state fill - green
// #16965A/white when connected, amber #F2A93B/near-black while connecting or
// stopping, neutral surface/muted when off.
const pillClasses = computed(() => {
  const base = 'inline-flex items-center rounded-[28px] px-3.5 py-2 text-[15px] font-bold';
  switch (state.status) {
    case 'connected':
      return `${base} bg-[#16965A] text-white`;
    case 'connecting':
    case 'stopping':
      return `${base} bg-[#F2A93B] text-[#10131A]`;
    default:
      return `${base} bg-wings-surface text-wings-mutedStrong`;
  }
});

// Connecting sub-stage token -> short label, connect_stage_*.
function stageLabel(stage) {
  switch (stage) {
    case 'captcha':
      return 'captcha';
    case 'auth':
      return 'TURN auth';
    case 'turn':
      return 'TURN';
    default:
      return '';
  }
}

const pillText = computed(() => {
  const n = state.streams;
  const m = state.threads;
  switch (state.status) {
    case 'connecting': {
      // Show the sub-stage until streams start filling, then the N/M counter.
      const label = stageLabel(state.stage);
      if (label) return `Подключение (${label})…`;
      return n > 0 && m > 0 ? `Подключение (${n}/${m})…` : 'Подключение…';
    }
    case 'connected':
      return n > 0 && m > 0 && n < m ? `Подключено (${n}/${m})` : 'Подключено';
    case 'stopping':
      return 'Останавливаем соединение…';
    default:
      return 'Отключено';
  }
});

const hintText = computed(() => {
  switch (state.status) {
    case 'connecting':
      return 'Запускаем сервисы…';
    case 'connected':
      return 'Нажмите, чтобы выключить';
    case 'stopping':
      return 'Останавливаем сервисы…';
    default:
      return 'Нажмите, чтобы включить';
  }
});

const endpointText = computed(() => state.endpoint || activeEndpoint.value || 'нет активного профиля');

async function toggle() {
  if (busy.value) return;
  try {
    if (state.status === 'disconnected') {
      assign(await ConnectionService.Connect());
    } else {
      assign(await ConnectionService.Disconnect());
    }
  } catch (e) {
    state.error = String(e?.message ?? e ?? 'Ошибка подключения');
  }
}

async function pasteImport() {
  importing.value = true;
  state.error = '';
  try {
    const text = await Clipboard.Text();
    if (!text) return;
    await ProfilesService.SmartImport(text.trim());
    await loadActiveEndpoint();
  } catch (e) {
    state.error = String(e?.message ?? e ?? 'Не удалось импортировать из буфера');
  } finally {
    importing.value = false;
  }
}

async function copyConfig() {
  state.error = '';
  try {
    const link = await ProfilesService.ExportActive();
    await Clipboard.SetText(link);
    copied.value = true;
    setTimeout(() => (copied.value = false), 1500);
  } catch (e) {
    state.error = String(e?.message ?? e ?? 'Нет активного профиля для экспорта');
  }
}

async function loadActiveEndpoint() {
  try {
    const result = await ProfilesService.List();
    if ((result.networkBackend || 'vk_turn') === 'xray') {
      const active = (result.xrayProfiles ?? []).find((p) => p.id === result.xrayActiveId);
      activeEndpoint.value = active ? (active.port ? `${active.address}:${active.port}` : active.address) : '';
    } else {
      const active = (result.profiles ?? []).find((p) => p.id === result.activeId);
      activeEndpoint.value = active ? active.vkTurnEndpoint : '';
    }
  } catch {
    // backend not available (pure-vite preview)
  }
}

function applyStats(next) {
  if (!next) return;
  traffic.rxBytes = next.rxBytes ?? 0;
  traffic.txBytes = next.txBytes ?? 0;
  traffic.rxRate = next.rxRate ?? 0;
  traffic.txRate = next.txRate ?? 0;
}

function applyIPInfo(next) {
  ipinfo.ip = next?.ip ?? '';
  ipinfo.country = next?.country ?? '';
  ipinfo.countryCode = next?.countryCode ?? '';
  ipinfo.provider = next?.provider ?? '';
}

async function refreshIP() {
  if (refreshingIP.value) return;
  refreshingIP.value = true;
  try {
    applyIPInfo(await ConnectionService.RefreshIPInfo());
  } catch {
    // lookup failed (offline / not connected)
  } finally {
    refreshingIP.value = false;
  }
}

onMounted(async () => {
  offs.push(Events.On(CONNECTION_STATE_EVENT, (ev) => assign(ev?.data)));
  offs.push(Events.On(TRAFFIC_STATS_EVENT, (ev) => applyStats(ev?.data)));
  offs.push(Events.On(IP_INFO_EVENT, (ev) => applyIPInfo(ev?.data)));
  try {
    assign(await ConnectionService.State());
    applyIPInfo(await ConnectionService.IPInfo());
  } catch {
    // backend not available
  }
  loadActiveEndpoint();
  // Auto-refresh the exit IP / geo whenever the Home tab opens.
  refreshIP();
});

onUnmounted(() => {
  offs.forEach((off) => typeof off === 'function' && off());
});
</script>

<style scoped>
/* Connected power button: green-to-blue fill + a thin conic gradient rim (our 180px
   size unchanged). The animated traffic glow around it lives in PowerGlow. */
.power-on {
  background: radial-gradient(circle at 50% 50%, #1b7fe0 0%, #1866d6 56%, #1259d1 100%);
}
/* Green tint is not pinned to one spot: two soft green pools sit near the rim and slowly
   orbit the center, so the green drifts around the edges over a blue base. Clipped to the
   circle by the round pseudo-element's own border-radius. */
.power-on::after {
  content: '';
  position: absolute;
  inset: 0;
  z-index: 0;
  border-radius: 9999px;
  background:
    radial-gradient(38% 38% at 74% 30%, rgba(31, 191, 122, 0.95) 0%, rgba(31, 191, 122, 0) 70%),
    radial-gradient(34% 34% at 28% 72%, rgba(24, 200, 128, 0.62) 0%, rgba(24, 200, 128, 0) 72%);
  animation: power-green-float 9s linear infinite;
  pointer-events: none;
}
@media (prefers-reduced-motion: reduce) {
  .power-on::after {
    animation: none;
  }
}
@keyframes power-green-float {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
.power-on::before {
  content: '';
  position: absolute;
  inset: -2px;
  z-index: 1;
  border-radius: 9999px;
  padding: 2px;
  background: conic-gradient(from -90deg, #16b877, #45a6ff, #2f7ff0, #16b877);
  -webkit-mask:
    linear-gradient(#000 0 0) content-box,
    linear-gradient(#000 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  pointer-events: none;
}
</style>
