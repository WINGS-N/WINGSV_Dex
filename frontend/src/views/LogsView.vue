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
            <span>Версия: {{ snapshot.version }}</span>
          </div>
          <pre
            ref="logEl"
            class="max-h-[58vh] overflow-auto whitespace-pre-wrap break-words font-mono text-[12px] leading-5 text-wings-text"
            >{{ snapshot.text || 'Пока нет записей.' }}</pre>
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

const channels = [
  { value: 'runtime', label: 'Runtime' },
  { value: 'proxy', label: 'Proxy' },
];

const channel = ref('runtime');
const autoscroll = ref(true);
const snapshot = ref({ channel: 'runtime', lines: [], text: '', version: 0 });
const logEl = ref(null);
let timer = null;
let offLogUpdate = null;
let alive = false;
let requestSeq = 0;
let loading = false;
let loadingId = 0;

const statusText = computed(() => `Канал: ${channel.value === 'runtime' ? 'Runtime' : 'Proxy'}`);

async function loadSnapshot({ notify = false, force = false } = {}) {
  if (loading && !force) return;
  const requestId = ++requestSeq;
  loading = true;
  loadingId = requestId;
  const requestedChannel = channel.value;
  try {
    const nextSnapshot = await LogsService.Snapshot(requestedChannel);
    if (!alive || requestId !== requestSeq || requestedChannel !== channel.value) return;
    snapshot.value = nextSnapshot;
    if (autoscroll.value) {
      await nextTick();
      if (alive && requestId === requestSeq) logEl.value?.scrollTo?.(0, logEl.value.scrollHeight);
    }
  } catch {
    if (notify && alive && requestId === requestSeq) showToast('Журнал недоступен', { type: 'warn' });
  } finally {
    if (loadingId === requestId) loading = false;
  }
}

async function refresh() {
  await loadSnapshot({ notify: true });
}

async function copyText() {
  const requestId = requestSeq;
  try {
    await Clipboard.SetText(snapshot.value.text || '');
    if (alive && requestId === requestSeq) showToast('Журнал скопирован', { type: 'success' });
  } catch {
    if (alive && requestId === requestSeq) showToast('Не удалось скопировать', { type: 'warn' });
  }
}

async function requestClear() {
  if (!alive) return;
  const requestId = ++requestSeq;
  const requestedChannel = channel.value;
  try {
    loading = false;
    loadingId = 0;
    await LogsService.Clear(requestedChannel);
    if (!alive || requestId !== requestSeq || requestedChannel !== channel.value) return;
    showToast('Журнал очищен', { type: 'success' });
    await loadSnapshot({ notify: true, force: true });
  } catch {
    if (alive && requestId === requestSeq) showToast('Не удалось очистить журнал', { type: 'warn' });
  }
}

watch(channel, () => {
  requestSeq++;
  loading = false;
  snapshot.value = { channel: channel.value, lines: [], text: '', version: 0 };
  loadSnapshot({ notify: true });
});

onMounted(async () => {
  alive = true;
  await loadSnapshot({ notify: true });
  offLogUpdate = Events.On('logs:updated', (ev) => {
    if (ev?.data?.channel === channel.value) loadSnapshot({ force: true });
  });
  timer = setInterval(() => loadSnapshot(), 5000);
});

onBeforeUnmount(() => {
  alive = false;
  requestSeq++;
  loading = false;
  loadingId = 0;
  if (timer) clearInterval(timer);
  if (offLogUpdate) offLogUpdate();
});
</script>
