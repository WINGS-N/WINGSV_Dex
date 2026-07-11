<template>
  <div class="pb-6">
    <AppHeader>
      <template #action>
        <button
          type="button"
          class="rounded-full border border-wings-divider px-3 py-1.5 text-[13px] text-wings-text transition-colors hover:border-wings-accent hover:text-wings-accent"
          @click="closeOverlay"
        >
          Закрыть
        </button>
      </template>
    </AppHeader>

    <div class="px-4">
      <SamsungCard kicker="Logs">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-wings-divider pb-4">
          <div>
            <h2 class="text-[22px] font-bold leading-tight text-wings-text">Просмотр журнала</h2>
            <p class="mt-1 text-sm text-wings-muted">Runtime и proxy события обновляются во время подключения.</p>
          </div>

          <div class="flex items-center gap-2 rounded-full border border-wings-divider bg-wings-surface p-1">
            <button
              v-for="option in channels"
              :key="option.value"
              type="button"
              class="rounded-full px-3 py-1.5 text-[13px] transition-colors"
              :class="
                channel === option.value ? 'bg-wings-accent text-white' : 'text-wings-muted hover:text-wings-text'
              "
              @click="channel = option.value"
            >
              {{ option.label }}
            </button>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap items-center gap-2">
          <SamsungButton variant="secondary" @click="refresh">Обновить</SamsungButton>
          <SamsungButton variant="secondary" @click="copyText">Копировать</SamsungButton>
          <SamsungButton variant="danger" @click="requestClear">Очистить</SamsungButton>

          <label class="ml-auto inline-flex items-center gap-2 text-[13px] text-wings-muted">
            <input v-model="autoscroll" type="checkbox" class="h-4 w-4 accent-wings-accent" />
            Автопрокрутка
          </label>
        </div>

        <div class="mt-4 rounded-[20px] border border-wings-divider bg-[#0c0c0c] p-4">
          <div class="mb-3 flex items-center justify-between gap-3 text-[12px] text-wings-muted">
            <span>{{ statusText }}</span>
            <span>Строк: {{ lines.length }}</span>
          </div>
          <pre
            ref="logEl"
            class="max-h-[58vh] overflow-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-5 text-wings-text"
            >{{ displayText || 'Пока нет записей.' }}</pre>
        </div>
      </SamsungCard>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Clipboard, Events } from '@wailsio/runtime';
import { LogsService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import { closeOverlay } from '@/stores/nav.js';
import { showToast } from '@/stores/toast.js';

// Keep the on-screen buffer bounded like the on-disk store; drop the oldest in chunks so
// trimming does not run on every appended line.
const MAX_LINES = 4000;
const TRIM_CHUNK = 500;

const channels = [
  { value: 'runtime', label: 'Runtime' },
  { value: 'proxy', label: 'Proxy' },
];

const channel = ref('runtime');
const autoscroll = ref(true);
const lines = ref([]);
const logEl = ref(null);
let alive = false;
let requestSeq = 0;

const displayText = computed(() => lines.value.join('\n'));
const statusText = computed(() => `Канал: ${channel.value === 'runtime' ? 'Runtime' : 'Proxy'}`);

async function scrollToEnd() {
  if (!autoscroll.value) return;
  await nextTick();
  if (alive) logEl.value?.scrollTo?.(0, logEl.value.scrollHeight);
}

async function loadSnapshot({ notify = false } = {}) {
  const requestId = ++requestSeq;
  const requested = channel.value;
  try {
    const snap = await LogsService.Snapshot(requested);
    if (!alive || requestId !== requestSeq || requested !== channel.value) return;
    lines.value = snap.lines || [];
    await scrollToEnd();
  } catch {
    if (notify && alive && requestId === requestSeq) showToast('Журнал недоступен', { type: 'warn' });
  }
}

// Live push: append each new line for the visible channel without re-fetching the file.
function onLine(ev) {
  const d = ev?.data;
  if (!d || d.channel !== channel.value) return;
  lines.value.push(d.line);
  if (lines.value.length > MAX_LINES) lines.value.splice(0, lines.value.length - (MAX_LINES - TRIM_CHUNK));
  scrollToEnd();
}

async function refresh() {
  await loadSnapshot({ notify: true });
}

async function copyText() {
  try {
    await Clipboard.SetText(displayText.value);
    showToast('Журнал скопирован', { type: 'success' });
  } catch {
    showToast('Не удалось скопировать', { type: 'warn' });
  }
}

async function requestClear() {
  const requested = channel.value;
  try {
    await LogsService.Clear(requested);
    if (requested === channel.value) lines.value = [];
    showToast('Журнал очищен', { type: 'success' });
  } catch {
    showToast('Не удалось очистить журнал', { type: 'warn' });
  }
}

watch(channel, () => {
  lines.value = [];
  loadSnapshot({ notify: true });
});

let offLine = null;
onMounted(async () => {
  alive = true;
  await loadSnapshot({ notify: true });
  offLine = Events.On('logs:line', onLine);
});

onBeforeUnmount(() => {
  alive = false;
  requestSeq++;
  if (offLine) offLine();
});
</script>
