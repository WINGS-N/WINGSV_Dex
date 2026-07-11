<template>
  <div class="flex h-full flex-col px-6 pb-8 pt-5">
    <div class="shrink-0 text-center">
      <div
        class="mb-2 inline-flex items-center rounded-full bg-white/15 px-4 py-1.5 text-[14px] font-semibold text-white"
      >
        {{ badge }}
      </div>
      <h1
        class="font-sharp text-[2.1rem] font-bold leading-tight text-white [text-shadow:0_2px_16px_rgba(8,29,64,0.25)]"
      >
        {{ title }}
      </h1>
      <p class="mt-2 text-[1.05rem] font-medium leading-snug text-white/90">{{ subtitle }}</p>
    </div>

    <div class="mt-4 min-h-0 flex-1 overflow-y-auto">
      <div class="flex justify-center py-2">
        <Wifi :size="56" class="text-white/85 [filter:drop-shadow(0_2px_14px_rgba(8,29,64,0.25))]" />
      </div>
      <ProgressBar
        v-if="showProgress"
        glass
        :value="state.total ? state.completed / state.total : undefined"
        :label="`${state.completed} из ${state.total} · Найдено: ${state.found} из ${state.target}`"
      />
      <ProfileChain :rows="chain" glass class="mt-3" />
    </div>

    <div class="shrink-0 pt-3">
      <template v-if="state.phase === 'whitelist_wait'">
        <SuwButton block @click="continueRun">Я подключил сеть с белым списком</SuwButton>
        <SuwButton block class="mt-2" @click="$emit('back')">Отмена</SuwButton>
      </template>
      <template v-else-if="state.phase === 'awaiting_apply'">
        <SuwButton block @click="apply(true)">Применить</SuwButton>
        <SuwButton block class="mt-2" @click="apply(false)">Не применять</SuwButton>
      </template>
      <template v-else-if="state.phase === 'failed'">
        <SuwButton block @click="$emit('finish')">Продолжить</SuwButton>
      </template>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Wifi } from 'lucide-vue-next';
import { Events } from '@wailsio/runtime';
import { AutoSearchService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import SuwButton from '@/components/onboarding/SuwButton.vue';
import ProfileChain from '@/components/autosearch/ProfileChain.vue';
import ProgressBar from '@/components/autosearch/ProgressBar.vue';

const props = defineProps({ mode: { type: String, default: 'standard' } });
const emit = defineEmits(['finish', 'back']);

const state = reactive({ phase: 'prepare', completed: 0, total: 0, found: 0, target: 5, message: '' });
const chain = ref([]);
const removeTimers = new Map();

const badge = computed(() =>
  state.phase === 'awaiting_apply' || state.phase === 'whitelist_wait' ? 'Ожидается действие' : 'Автопоиск выполняется',
);
const title = computed(() => {
  switch (state.phase) {
    case 'whitelist_wait':
      return 'Подключите сеть с белым списком';
    case 'download':
      return 'Проверка трафика';
    case 'awaiting_apply':
      return 'Ожидание действия';
    case 'failed':
      return 'Не найдено';
    default:
      return 'Идёт TCPing';
  }
});
const subtitle = computed(() => {
  switch (state.phase) {
    case 'whitelist_wait':
      return 'Сначала подключитесь к сети с белым списком - на ней обновим подписку и проверим профили.';
    case 'download':
      return 'Проверяем профили скачиванием тест-файла...';
    case 'awaiting_apply':
      return `Найден стабильный профиль: ${state.message}. Применить найденную конфигурацию?`;
    case 'failed':
      return state.message;
    default:
      return 'Проверяем TCP доступность профилей';
  }
});
const showProgress = computed(() => ['tcping', 'download', 'awaiting_apply'].includes(state.phase));

let offState = null;
let offProfile = null;

onMounted(() => {
  offState = Events.On('autosearch:state', (ev) => ev?.data && Object.assign(state, ev.data));
  offProfile = Events.On('autosearch:profile', (ev) => ev?.data && upsertRow(ev.data));
  AutoSearchService.Start(props.mode);
});

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

function continueRun() {
  chain.value = [];
  state.phase = 'prepare';
  AutoSearchService.Continue();
}

async function apply(doApply) {
  await AutoSearchService.Apply(doApply);
  emit('finish');
}

onBeforeUnmount(() => {
  if (offState) offState();
  if (offProfile) offProfile();
  removeTimers.forEach((t) => clearTimeout(t));
  AutoSearchService.Cancel();
});
</script>
