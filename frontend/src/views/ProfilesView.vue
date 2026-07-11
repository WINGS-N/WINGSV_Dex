<template>
  <div class="pb-6">
    <AppHeader />

    <div class="px-4">
      <SamsungCard kicker="Действия">
        <div class="divide-y divide-wings-divider">
          <OneuiSelect
            label="Сетевой backend"
            :model-value="networkBackend"
            :options="networkBackendOptions"
            @update:model-value="setNetworkBackend"
          />
          <OneuiSelect
            v-if="!isXray"
            label="Под-backend"
            :model-value="subBackend"
            :options="subBackendOptions"
            @update:model-value="setSubBackend"
          />

          <template v-if="isXray">
            <button
              type="button"
              class="flex w-full items-center justify-between py-3.5 text-left"
              @click="openOverlay('subscriptions')"
            >
              <span class="flex flex-col">
                <span class="text-[17px]">Подписки</span>
                <span class="mt-0.5 text-sm text-wings-muted">Управление URL подписок и их обновлением</span>
              </span>
              <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
            </button>
            <button
              type="button"
              class="flex w-full items-center justify-between py-3.5 text-left"
              :disabled="refreshingSubs"
              @click="refreshSubs"
            >
              <span class="flex flex-col">
                <span class="text-[17px]">Обновить подписки</span>
                <span class="mt-0.5 text-sm text-wings-muted">Последнее обновление: {{ lastUpdatedText }}</span>
              </span>
              <SamsungSpinner v-if="refreshingSubs" class="shrink-0" />
              <RefreshCw v-else :size="20" class="shrink-0 text-wings-muted" />
            </button>
            <button
              type="button"
              class="flex w-full items-center justify-between py-3.5 text-left"
              @click="showTest = !showTest"
            >
              <span class="flex flex-col">
                <span class="text-[17px]">Тест подключения</span>
                <span class="mt-0.5 text-sm text-wings-muted">Задержка для всех профилей текущего фильтра</span>
              </span>
              <ChevronRight
                :size="20"
                class="text-wings-muted transition-transform"
                :class="{ 'rotate-90': showTest }"
              />
            </button>
            <div v-if="showTest" class="flex gap-2 py-3">
              <SamsungButton variant="secondary" :busy="testing" @click="runTest('tcping')">TCPing</SamsungButton>
              <SamsungButton variant="secondary" :busy="testing" @click="runTest('real')">Real Delay</SamsungButton>
            </div>
          </template>

          <button
            type="button"
            class="flex w-full items-center justify-between py-3.5 text-left"
            @click="showImport = !showImport"
          >
            <span class="flex flex-col">
              <span class="text-[17px]">Добавить профиль</span>
              <span class="mt-0.5 text-sm text-wings-muted">Импортировать конфиг из буфера обмена</span>
            </span>
            <ChevronRight
              :size="20"
              class="text-wings-muted transition-transform"
              :class="{ 'rotate-90': showImport }"
            />
          </button>
        </div>

        <div v-if="showImport" class="mt-3 flex flex-col gap-2">
          <textarea
            v-model="linkText"
            rows="3"
            placeholder="wingsv:// / vless:// / https://подписка"
            class="w-full resize-none rounded-xl border border-wings-divider bg-wings-input px-3 py-2 font-mono text-sm text-wings-text outline-none placeholder:text-wings-muted focus:border-wings-inputLine"
          ></textarea>
          <div class="flex gap-2">
            <SamsungButton variant="secondary" :busy="busy" @click="pasteAndImport">
              <template #icon><ClipboardPaste :size="18" /></template>
              Из буфера
            </SamsungButton>
            <SamsungButton variant="primary" :busy="busy" :disabled="!linkText.trim()" @click="importLink(linkText)">
              Импортировать
            </SamsungButton>
          </div>
          <p v-if="error" class="text-sm text-wings-danger">{{ error }}</p>
        </div>
      </SamsungCard>

      <div class="mt-6">
        <div class="mb-2 px-1 text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker">Профили</div>

        <SamsungSectionLoader v-if="loading" />

        <p v-else-if="allItems.length === 0" class="py-8 text-center text-sm text-wings-muted">Профилей пока нет</p>

        <template v-else>
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="inline-flex h-9 items-center rounded-full border px-4 text-[15px] transition-colors"
              :class="chipClass('all')"
              @click="activeFilter = 'all'"
            >
              Все
            </button>
            <button
              v-if="hasFavorites"
              type="button"
              aria-label="Избранное"
              class="inline-flex h-9 items-center rounded-full border px-3 transition-colors"
              :class="chipClass('favorites')"
              @click="activeFilter = 'favorites'"
            >
              <Bookmark :size="18" :class="{ 'fill-current': activeFilter === 'favorites' }" />
            </button>
            <button
              v-for="chip in subscriptionChips"
              :key="chip.id"
              type="button"
              class="inline-flex h-9 items-center rounded-full border px-4 text-[15px] transition-colors"
              :class="chipClass(chip.id)"
              @click="activeFilter = chip.id"
            >
              {{ chip.title }}
            </button>
          </div>

          <SamsungCard class="!p-0">
            <div class="divide-y divide-wings-divider">
              <template v-for="group in groups" :key="group.key">
                <div v-if="group.title" class="bg-wings-surface px-4 py-2 text-[13px] font-bold text-wings-text">
                  {{ group.title }}
                </div>
                <div v-for="p in group.items" :key="p.id" class="flex items-center gap-3 px-4 py-3.5">
                  <button type="button" class="min-w-0 flex-1 text-left" @click="activate(p.id)">
                    <div class="flex items-center gap-2">
                      <Check v-if="p.id === currentActiveId" :size="16" class="shrink-0 text-emerald-400" />
                      <span class="truncate text-[17px]">{{ p.title }}</span>
                    </div>
                    <span class="mt-0.5 block truncate text-sm text-wings-muted">{{ p.subtitle }}</span>
                  </button>
                  <SamsungSpinner v-if="pending[p.id]" class="shrink-0" />
                  <span
                    v-else-if="p.ping !== undefined"
                    class="shrink-0 rounded-full px-2.5 py-1 text-[13px] font-semibold"
                    :class="pingClass(p.ping)"
                  >
                    {{ p.ping < 0 ? '—' : `${p.ping} ms` }}
                  </span>
                  <button type="button" aria-label="В избранное" class="shrink-0 p-1" @click="toggleFavorite(p.id)">
                    <Star :size="20" :class="p.favorite ? 'fill-wings-accent text-wings-accent' : 'text-wings-muted'" />
                  </button>
                  <button
                    type="button"
                    aria-label="Удалить"
                    class="shrink-0 p-1 text-wings-muted hover:text-wings-danger"
                    @click="remove(p.id)"
                  >
                    <Trash2 :size="18" />
                  </button>
                </div>
              </template>
            </div>
          </SamsungCard>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { Bookmark, Check, ChevronRight, ClipboardPaste, RefreshCw, Star, Trash2 } from 'lucide-vue-next';
import { Clipboard, Events } from '@wailsio/runtime';
import {
  ConnectionService,
  ProfilesService,
  SubscriptionService,
  XrayTestService,
} from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import { confirm } from '@/stores/confirm.js';
import { openOverlay } from '@/stores/nav.js';
import { showToast } from '@/stores/toast.js';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import SamsungSpinner from '@/components/layout/SamsungSpinner.vue';

const profiles = ref([]);
const activeId = ref('');
const xrayProfiles = ref([]);
const xrayActiveId = ref('');
const subscriptions = ref([]);
const networkBackend = ref('vk_turn');
const loading = ref(true);
const busy = ref(false);
const error = ref('');
const showImport = ref(false);
const showTest = ref(false);
const testing = ref(false);
const refreshingSubs = ref(false);
const linkText = ref('');
const activeFilter = ref('all'); // all | favorites | <subscriptionId>
const pings = reactive({}); // profileId -> delayMs (from the last test)
const pending = reactive({}); // profileId -> true while its test is in flight
const networkBackendOptions = [
  { value: 'vk_turn', label: 'VK TURN' },
  { value: 'xray', label: 'Xray' },
];
const subBackendOptions = [
  { value: 'wg', label: 'WireGuard' },
  { value: 'awg', label: 'AmneziaWG' },
];
const subBackend = ref('wg');

const isXray = computed(() => networkBackend.value === 'xray');
const currentActiveId = computed(() => (isXray.value ? xrayActiveId.value : activeId.value));

function apply(result) {
  profiles.value = result.profiles ?? [];
  activeId.value = result.activeId ?? '';
  subBackend.value = result.subBackend || 'wg';
  networkBackend.value = result.networkBackend || 'vk_turn';
  xrayProfiles.value = result.xrayProfiles ?? [];
  xrayActiveId.value = result.xrayActiveId ?? '';
  subscriptions.value = result.subscriptions ?? [];
}

// All items for the current backend, mapped to a common list shape (with the last ping).
const allItems = computed(() => {
  if (isXray.value) {
    return xrayProfiles.value.map((p) => ({
      id: p.id,
      title: p.title,
      subtitle: p.port ? `${p.address}:${p.port}` : p.address,
      favorite: p.favorite,
      subscriptionId: p.subscriptionId || '',
      ping: pings[p.id],
    }));
  }
  return profiles.value
    .filter((p) => (p.transportKind || 'wg') === subBackend.value)
    .map((p) => ({
      id: p.id,
      title: p.title,
      subtitle: `${p.vkTurnEndpoint} (${transportLabel(p.transportKind)})`,
      favorite: p.favorite,
      subscriptionId: '',
      ping: undefined,
    }));
});

const hasFavorites = computed(() => allItems.value.some((p) => p.favorite));

// One chip per subscription that actually has nodes, plus a "Свои" chip for manual nodes.
const subscriptionChips = computed(() => {
  if (!isXray.value) return [];
  const present = new Set(allItems.value.map((p) => p.subscriptionId));
  const chips = subscriptions.value
    .filter((s) => present.has(s.id))
    .map((s) => ({ id: s.id, title: s.title || s.url }));
  if (present.has('')) chips.push({ id: 'manual', title: 'Свои' });
  return chips;
});

// Filter the items by the active chip.
const filteredItems = computed(() => {
  if (activeFilter.value === 'all') return allItems.value;
  if (activeFilter.value === 'favorites') return allItems.value.filter((p) => p.favorite);
  if (activeFilter.value === 'manual') return allItems.value.filter((p) => !p.subscriptionId);
  return allItems.value.filter((p) => p.subscriptionId === activeFilter.value);
});

// Best-to-worst by ping once a test has finished (tested first, failures and untested last).
// Held stable while the test is still streaming so rows do not jump around under the loader.
function sortByPing(items) {
  if (!hasPings.value || testing.value) return items;
  return [...items].sort((a, b) => rank(a.ping) - rank(b.ping));
}
function rank(ping) {
  if (ping === undefined) return Number.MAX_SAFE_INTEGER;
  if (ping < 0) return Number.MAX_SAFE_INTEGER - 1;
  return ping;
}
const hasPings = computed(() => Object.keys(pings).length > 0);

// Under "Все" the xray list is grouped by subscription; otherwise it is a single group.
const groups = computed(() => {
  if (!isXray.value || activeFilter.value !== 'all') {
    return [{ key: 'flat', title: '', items: sortByPing(filteredItems.value) }];
  }
  const out = [];
  for (const sub of subscriptions.value) {
    const items = allItems.value.filter((p) => p.subscriptionId === sub.id);
    if (items.length) out.push({ key: sub.id, title: sub.title || sub.url, items: sortByPing(items) });
  }
  const manual = allItems.value.filter((p) => !p.subscriptionId);
  if (manual.length) out.push({ key: 'manual', title: 'Свои профили', items: sortByPing(manual) });
  return out;
});

const lastUpdatedText = computed(() => {
  const times = subscriptions.value.map((s) => s.lastUpdatedAt || 0).filter(Boolean);
  if (!times.length) return 'никогда';
  return new Date(Math.max(...times) * 1000).toLocaleString();
});

// Fall back to "Все" when the active chip no longer has any items.
watch([hasFavorites, subscriptionChips], () => {
  if (activeFilter.value === 'favorites' && !hasFavorites.value) activeFilter.value = 'all';
  if (
    activeFilter.value !== 'all' &&
    activeFilter.value !== 'favorites' &&
    !subscriptionChips.value.some((c) => c.id === activeFilter.value)
  ) {
    activeFilter.value = 'all';
  }
});

function chipClass(id) {
  return activeFilter.value === id
    ? 'border-transparent bg-wings-accent text-white'
    : 'border-wings-divider bg-wings-surface text-wings-muted hover:text-wings-text';
}

function pingClass(ping) {
  if (ping < 0) return 'bg-red-500/20 text-red-400';
  if (ping < 150) return 'bg-emerald-500/20 text-emerald-400';
  if (ping < 400) return 'bg-amber-500/20 text-amber-400';
  return 'bg-red-500/20 text-red-400';
}

async function setNetworkBackend(kind) {
  apply(await ProfilesService.SetNetworkBackend(kind));
  activeFilter.value = 'all';
}

async function ensureSubBackendAllowed(kind) {
  if (kind !== 'awg') return true;
  try {
    const info = await ConnectionService.AWGAvailability();
    if (info.available) return true;
    await confirm({
      title: 'AmneziaWG недоступен',
      message: `AmneziaWG недоступен на этой машине. Установите пакеты:\n\n${info.packages.join('\n')}`,
      confirmText: 'Понятно',
      cancelText: '',
    });
    return false;
  } catch {
    return true;
  }
}

async function setSubBackend(kind) {
  if (!(await ensureSubBackendAllowed(kind))) return;
  apply(await ProfilesService.SetSubBackend(kind));
}

function transportLabel(kind) {
  return kind === 'awg' ? 'AWG' : 'WG';
}

async function refresh() {
  try {
    apply(await ProfilesService.List());
  } finally {
    loading.value = false;
  }
}

async function refreshSubs() {
  refreshingSubs.value = true;
  try {
    const res = await SubscriptionService.RefreshAll();
    apply(await ProfilesService.List());
    showToast(`Обновлено узлов: ${res.updated}`, { type: 'success' });
  } catch {
    showToast('Не удалось обновить подписки', { type: 'warn' });
  } finally {
    refreshingSubs.value = false;
  }
}

// Kick off the test; results stream back per node so a Samsung loader runs on each row
// until its badge lands, then the list sorts best-to-worst once the run finishes.
async function runTest(kind) {
  try {
    const ids = await XrayTestService.Start(kind);
    Object.keys(pending).forEach((k) => delete pending[k]);
    (ids ?? []).forEach((id) => {
      pending[id] = true;
    });
    testing.value = (ids ?? []).length > 0;
    showTest.value = false;
  } catch {
    showToast('Тест не удался', { type: 'warn' });
  }
}

async function importLink(link) {
  const value = (link ?? '').trim();
  if (!value) return;
  busy.value = true;
  error.value = '';
  try {
    apply(await ProfilesService.SmartImport(value));
    if (!isXray.value) {
      const active = profiles.value.find((p) => p.id === activeId.value);
      const kind = active?.transportKind || 'wg';
      if (kind !== subBackend.value && (await ensureSubBackendAllowed(kind))) {
        apply(await ProfilesService.SetSubBackend(kind));
      }
    }
    linkText.value = '';
    showImport.value = false;
  } catch (e) {
    error.value = String(e?.message ?? e ?? 'Не удалось импортировать ссылку');
  } finally {
    busy.value = false;
  }
}

async function pasteAndImport() {
  try {
    const text = await Clipboard.Text();
    if (text) {
      linkText.value = text.trim();
      await importLink(text);
    }
  } catch {
    error.value = 'Нет доступа к буферу обмена, вставьте ссылку вручную';
  }
}

async function activate(id) {
  apply(isXray.value ? await ProfilesService.XrayActivate(id) : await ProfilesService.Activate(id));
}

async function toggleFavorite(id) {
  apply(isXray.value ? await ProfilesService.XrayToggleFavorite(id) : await ProfilesService.ToggleFavorite(id));
}

async function remove(id) {
  const item = allItems.value.find((p) => p.id === id);
  const ok = await confirm({
    title: 'Удалить профиль',
    message: item ? `Удалить профиль «${item.title}»?` : 'Удалить профиль?',
    confirmText: 'Удалить',
    cancelText: 'Отмена',
    danger: true,
  });
  if (!ok) return;
  apply(isXray.value ? await ProfilesService.XrayRemove(id) : await ProfilesService.Remove(id));
}

let offResult = null;
let offDone = null;
onMounted(async () => {
  await refresh();
  offResult = Events.On('xraytest:result', (ev) => {
    const d = ev?.data;
    if (!d) return;
    pings[d.id] = d.delayMs;
    delete pending[d.id];
  });
  offDone = Events.On('xraytest:done', () => {
    testing.value = false;
    Object.keys(pending).forEach((k) => delete pending[k]);
  });
});
onBeforeUnmount(() => {
  if (offResult) offResult();
  if (offDone) offDone();
});
</script>
