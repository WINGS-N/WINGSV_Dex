<template>
  <div ref="rootEl" class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <header class="flex shrink-0 items-center gap-2 px-3 pb-3 pt-6">
      <button
        type="button"
        class="rounded-full p-1.5 text-wings-mutedStrong hover:text-wings-text"
        aria-label="Назад"
        @click="back"
      >
        <ChevronLeft :size="24" />
      </button>
      <h1 class="font-sharp text-[22px] font-bold text-white">Автопоиск</h1>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
      <!-- Settings -->
      <template v-if="step === 'settings'">
        <SamsungCard kicker="Параметры проверки">
          <div class="divide-y divide-wings-divider">
            <OneuiInput label="Сколько искать" type="number" v-model="settings.targetCount" />
            <OneuiInput label="Таймаут TCPing, мс" type="number" v-model="settings.tcpingTimeoutMs" />
            <OneuiInput label="Размер тестового файла, MB" type="number" v-model="settings.downloadSizeMb" />
            <OneuiInput label="Таймаут скачивания, с" type="number" v-model="settings.downloadTimeoutSeconds" />
            <OneuiInput label="Количество прогонов" type="number" v-model="settings.downloadAttempts" />
            <SwitchRow
              title="Встроенная подписка"
              subtitle="Искать в списке Universal"
              v-model="settings.useBuiltInSubscription"
            />
          </div>
        </SamsungCard>
      </template>

      <!-- Mode -->
      <template v-else-if="step === 'mode'">
        <p class="mb-6 mt-6 text-center text-[1.05rem] text-wings-muted">Выберите, как проверить доступные профили</p>
      </template>

      <!-- Whitelist wait -->
      <template v-else-if="state.phase === 'whitelist_wait'">
        <div class="flex flex-col items-center gap-4 py-6 text-center">
          <Wifi :size="64" class="text-wings-accent" />
          <p class="text-[17px] font-bold text-white">Подключите сеть с белым списком</p>
          <p class="text-sm text-wings-muted">
            Сначала подключитесь к сети с белым списком. Затем мы обновим подписку и проверим профили на ней (если
            обновление не удастся - используем локальные профили).
          </p>
        </div>
      </template>

      <!-- Apply -->
      <template v-else-if="state.phase === 'awaiting_apply'">
        <div class="flex flex-col items-center gap-3 py-2 text-center">
          <Wifi :size="52" class="text-wings-accent" />
          <p class="text-[17px] font-bold text-white">Найден стабильный профиль: {{ state.message }}</p>
          <p class="text-sm text-wings-muted">Применить найденную конфигурацию?</p>
        </div>
        <ProgressBar class="mt-2" :label="`Найдено: ${state.found} из ${state.target}`" />
        <ProfileChain :rows="chain" class="mt-3" />
      </template>

      <!-- Failed -->
      <template v-else-if="state.phase === 'failed'">
        <div class="flex flex-col items-center gap-3 py-10 text-center">
          <p class="text-[17px] font-bold text-white">Автопоиск не удался</p>
          <p class="text-sm text-wings-muted">{{ state.message }}</p>
        </div>
      </template>

      <!-- Running -->
      <template v-else>
        <div class="flex flex-col items-center gap-2 py-2 text-center">
          <Wifi :size="52" class="text-white/85" />
          <p class="text-[19px] font-bold text-white">{{ phaseTitle }}</p>
          <p class="text-sm text-wings-muted">{{ phaseSubtitle }}</p>
        </div>
        <ProgressBar
          class="mt-3"
          :value="state.total ? state.completed / state.total : 0"
          :label="`${state.completed} из ${state.total} · Найдено: ${state.found} из ${state.target}`"
        />
        <ProfileChain :rows="chain" class="mt-3" />
      </template>
    </div>

    <!-- Fixed bottom action bar -->
    <div v-if="footer" class="shrink-0 border-t border-wings-divider px-4 py-3">
      <div class="flex flex-col gap-2">
        <template v-if="step === 'settings'">
          <SamsungButton variant="primary" block @click="goMode">Далее</SamsungButton>
        </template>
        <template v-else-if="step === 'mode'">
          <SamsungButton variant="primary" block @click="startRun('standard')">Стандарт</SamsungButton>
          <SamsungButton variant="secondary" block @click="startRun('whitelist')">Сеть с белым списком</SamsungButton>
        </template>
        <template v-else-if="state.phase === 'whitelist_wait'">
          <SamsungButton variant="primary" block @click="continueRun">Я подключил сеть</SamsungButton>
          <SamsungButton variant="secondary" block @click="back">Отмена</SamsungButton>
        </template>
        <template v-else-if="state.phase === 'awaiting_apply'">
          <SamsungButton variant="primary" block @click="apply(true)">Применить</SamsungButton>
          <SamsungButton variant="secondary" block @click="apply(false)">Не применять</SamsungButton>
        </template>
        <template v-else-if="state.phase === 'failed'">
          <SamsungButton variant="secondary" block @click="restart">Заново</SamsungButton>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { ChevronLeft, Wifi } from 'lucide-vue-next';
import { Events } from '@wailsio/runtime';
import { AutoSearchService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SwitchRow from '@/components/layout/SwitchRow.vue';
import ProfileChain from '@/components/autosearch/ProfileChain.vue';
import ProgressBar from '@/components/autosearch/ProgressBar.vue';
import { closeOverlay } from '@/stores/nav.js';
import { usePinnedScroll } from '@/composables/usePinnedScroll.js';

const rootEl = usePinnedScroll();

const step = ref('settings'); // settings | mode | run
const settings = reactive({
  targetCount: 5,
  tcpingTimeoutMs: 1000,
  downloadSizeMb: 5,
  downloadTimeoutSeconds: 20,
  downloadAttempts: 2,
  useBuiltInSubscription: true,
});
const state = reactive({ phase: '', completed: 0, total: 0, found: 0, target: 5, message: '' });
const chain = ref([]);

const INT_FIELDS = ['targetCount', 'tcpingTimeoutMs', 'downloadSizeMb', 'downloadTimeoutSeconds', 'downloadAttempts'];

const phaseTitle = computed(() => (state.phase === 'download' ? 'Проверка трафика' : 'Идёт TCPing'));
const phaseSubtitle = computed(() =>
  state.phase === 'download' ? 'Проверяем профили скачиванием тест-файла...' : 'Проверяем TCP доступность профилей',
);
// The action bar shows on every step except the pure "running" phases (tcping/download/prepare).
const footer = computed(
  () =>
    step.value === 'settings' ||
    step.value === 'mode' ||
    ['whitelist_wait', 'awaiting_apply', 'failed'].includes(state.phase),
);

let offState = null;
let offProfile = null;
const removeTimers = new Map();

onMounted(async () => {
  try {
    Object.assign(settings, await AutoSearchService.Settings());
  } catch {
    // backend not available (pure-vite preview)
  }
  offState = Events.On('autosearch:state', (ev) => {
    if (ev?.data) Object.assign(state, ev.data);
  });
  offProfile = Events.On('autosearch:profile', (ev) => {
    if (ev?.data) upsertRow(ev.data);
  });
});

// Keep one row per profile id; failed rows fade out after 5s, like the app.
function upsertRow(row) {
  const i = chain.value.findIndex((r) => r.id === row.id);
  if (i >= 0) chain.value[i] = { ...chain.value[i], ...row };
  else chain.value.push({ ...row });
  chain.value = chain.value.slice(-50);
  if (row.status === 'err') scheduleRemove(row.id);
  else if (removeTimers.has(row.id)) {
    clearTimeout(removeTimers.get(row.id));
    removeTimers.delete(row.id);
  }
}
function scheduleRemove(id) {
  if (removeTimers.has(id)) return;
  removeTimers.set(
    id,
    setTimeout(() => {
      const i = chain.value.findIndex((r) => r.id === id);
      if (i >= 0) chain.value[i].fading = true;
      setTimeout(() => {
        chain.value = chain.value.filter((r) => r.id !== id);
        removeTimers.delete(id);
      }, 240);
    }, 5000),
  );
}

async function goMode() {
  try {
    const payload = { ...settings };
    INT_FIELDS.forEach((f) => (payload[f] = Number(settings[f]) || 0));
    Object.assign(settings, await AutoSearchService.SetSettings(payload));
  } catch {
    // ignore
  }
  step.value = 'mode';
}

function startRun(mode) {
  chain.value = [];
  state.phase = 'prepare';
  step.value = 'run';
  AutoSearchService.Start(mode);
}

function continueRun() {
  chain.value = [];
  state.phase = 'prepare';
  AutoSearchService.Continue();
}

async function apply(doApply) {
  await AutoSearchService.Apply(doApply);
  closeOverlay();
}

function restart() {
  state.phase = '';
  chain.value = [];
  step.value = 'settings';
}

function back() {
  AutoSearchService.Cancel();
  closeOverlay();
}

onBeforeUnmount(() => {
  if (offState) offState();
  if (offProfile) offProfile();
  removeTimers.forEach((t) => clearTimeout(t));
  AutoSearchService.Cancel();
});
</script>
