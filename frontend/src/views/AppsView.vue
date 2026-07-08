<template>
  <div class="pb-6">
    <AppHeader />

    <div class="px-4">
      <SamsungCard kicker="Маршрутизация приложений" subtitle="Особое правило VPN для выбранных приложений">
        <div class="routing-mode-picker py-3" role="radiogroup" aria-label="Режим маршрутизации">
          <button
            v-for="opt in modeOptions"
            :key="opt.value"
            type="button"
            class="routing-mode-item"
            :class="{ 'is-active': routing.mode === opt.value }"
            role="radio"
            :aria-checked="routing.mode === opt.value"
            @click="setMode(opt.value)"
          >
            <span class="routing-mode-circle">
              <component :is="opt.icon" class="h-6 w-6" aria-hidden="true" />
            </span>
            <span class="routing-mode-label">{{ opt.label }}</span>
          </button>
        </div>
        <p class="pb-2 text-sm text-wings-muted">{{ modeHint }}</p>
      </SamsungCard>

      <SamsungCard v-if="routing.mode !== 'off'" kicker="Приложения" class="mt-5">
        <div class="py-3">
          <OneuiInput v-model="search" placeholder="Поиск по имени или файлу" :spellcheck="false" />
        </div>
        <div class="pb-3">
          <OneuiRadioGroup v-model="kind" variant="pill" :options="kindOptions" />
        </div>

        <SamsungSectionLoader v-if="!ready" />
        <p v-else-if="!filteredApps.length" class="py-6 text-center text-sm text-wings-muted">Приложения не найдены.</p>
        <TransitionGroup v-else :key="kind + '|' + search" tag="ul" name="applist" class="space-y-1.5 pb-2">
          <li
            v-for="app in filteredApps"
            :key="app.exec"
            class="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-2.5"
            :class="isRouted(app.exec) ? 'bg-wings-accent/10' : 'bg-white/[0.03]'"
            @click="toggle(app.exec, !isRouted(app.exec))"
          >
            <img v-if="app.icon" :src="app.icon" alt="" class="h-9 w-9 shrink-0 rounded-lg object-contain" />
            <div
              v-else
              class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white/[0.06] text-sm font-bold text-wings-mutedStrong"
              aria-hidden="true"
            >
              {{ (app.name || app.exec || '?').slice(0, 1).toUpperCase() }}
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate text-[15px] text-wings-text">{{ app.name }}</p>
              <p class="truncate text-[12px] text-wings-muted">{{ app.exec }}</p>
            </div>
            <SamsungPill v-if="app.system" variant="offline" class="shrink-0">Системное</SamsungPill>
            <OneuiSwitch :model-value="isRouted(app.exec)" class="pointer-events-none shrink-0" />
          </li>
        </TransitionGroup>
      </SamsungCard>

      <p v-if="error" class="mt-4 text-sm text-wings-danger">{{ error }}</p>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { PowerOff, Split, ShieldCheck } from 'lucide-vue-next';
import { AppsService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungPill from '@/components/layout/SamsungPill.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiSwitch from '@/components/controls/OneuiSwitch.vue';
import OneuiRadioGroup from '@/components/controls/OneuiRadioGroup.vue';

const modeOptions = [
  { value: 'off', label: 'Off', icon: PowerOff },
  { value: 'bypass', label: 'Bypass', icon: Split },
  { value: 'whitelist', label: 'Whitelist', icon: ShieldCheck },
];
const modeHints = {
  off: 'Все приложения идут через VPN.',
  bypass: 'Выбранные приложения идут напрямую, мимо VPN; остальные — через VPN.',
  whitelist: 'Через VPN идут только выбранные приложения, остальные — напрямую.',
};

const ready = ref(false);
const error = ref('');
const apps = ref([]);
const search = ref('');
const kind = ref('all');
const routing = reactive({ mode: 'off', bypass: [], whitelist: [] });

const modeHint = computed(() => modeHints[routing.mode] ?? '');

// The list the active mode edits: bypass apps go direct, whitelist apps tunnel.
const activeList = computed(() => (routing.mode === 'whitelist' ? routing.whitelist : routing.bypass));
function isRouted(exec) {
  return activeList.value.includes(exec);
}

const kindOptions = computed(() => [
  { value: 'all', label: 'Все', count: apps.value.length },
  { value: 'user', label: 'Пользовательские', count: apps.value.filter((a) => !a.system).length },
  { value: 'system', label: 'Системные', count: apps.value.filter((a) => a.system).length },
]);

const filteredApps = computed(() => {
  const q = search.value.trim().toLowerCase();
  const routed = activeList.value; // dependency: re-sort when the selection changes
  const list = apps.value.filter((a) => {
    if (kind.value === 'user' && a.system) return false;
    if (kind.value === 'system' && !a.system) return false;
    if (q && !`${a.name} ${a.exec}`.toLowerCase().includes(q)) return false;
    return true;
  });
  // Selected apps float to the top; within each group the backend's alphabetical
  // order is preserved (Array.prototype.sort is stable).
  return list.slice().sort((a, b) => Number(routed.includes(b.exec)) - Number(routed.includes(a.exec)));
});

let saveTimer = null;
function scheduleSave() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(save, 300);
}
async function save() {
  try {
    await AppsService.SetRouting({
      mode: routing.mode,
      bypass: [...routing.bypass],
      whitelist: [...routing.whitelist],
    });
    error.value = '';
  } catch (e) {
    error.value = String(e?.message ?? e ?? 'Не удалось сохранить маршрутизацию');
  }
}

function setMode(m) {
  routing.mode = m;
  scheduleSave();
}
function toggle(exec, on) {
  const list = routing.mode === 'whitelist' ? routing.whitelist : routing.bypass;
  const i = list.indexOf(exec);
  if (on && i < 0) list.push(exec);
  else if (!on && i >= 0) list.splice(i, 1);
  scheduleSave();
}

onMounted(async () => {
  try {
    const r = await AppsService.Routing();
    routing.mode = r?.mode || 'off';
    routing.bypass = Array.isArray(r?.bypass) ? [...r.bypass] : [];
    routing.whitelist = Array.isArray(r?.whitelist) ? [...r.whitelist] : [];
    apps.value = (await AppsService.List()) ?? [];
  } catch (e) {
    error.value = String(e?.message ?? e ?? 'Не удалось загрузить приложения');
  } finally {
    ready.value = true;
  }
});
</script>

<style scoped>
/* FLIP: animate rows sliding to their new position when the selection re-sorts. */
.applist-move {
  transition: transform 0.32s cubic-bezier(0.4, 0, 0.2, 1);
}
.applist-enter-active,
.applist-leave-active {
  transition:
    opacity 0.2s ease,
    transform 0.2s ease;
}
.applist-enter-from,
.applist-leave-to {
  opacity: 0;
}
.applist-leave-active {
  position: absolute;
}
</style>
