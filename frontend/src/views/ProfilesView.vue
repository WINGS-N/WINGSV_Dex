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

        <p v-else-if="items.length === 0" class="py-8 text-center text-sm text-wings-muted">Профилей пока нет</p>

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
          </div>

          <SamsungCard class="!p-0">
            <div class="divide-y divide-wings-divider">
              <div v-for="p in filteredItems" :key="p.id" class="flex items-center gap-3 px-4 py-3.5">
                <button type="button" class="min-w-0 flex-1 text-left" @click="activate(p.id)">
                  <div class="flex items-center gap-2">
                    <Check v-if="p.id === currentActiveId" :size="16" class="shrink-0 text-emerald-400" />
                    <span class="truncate text-[17px]">{{ p.title }}</span>
                  </div>
                  <span class="mt-0.5 block truncate text-sm text-wings-muted">{{ p.subtitle }}</span>
                </button>
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
            </div>
          </SamsungCard>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import { Bookmark, Check, ChevronRight, ClipboardPaste, Star, Trash2 } from 'lucide-vue-next';
import { Clipboard } from '@wailsio/runtime';
import { ConnectionService, ProfilesService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import { confirm } from '@/stores/confirm.js';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';

const profiles = ref([]);
const activeId = ref('');
const xrayProfiles = ref([]);
const xrayActiveId = ref('');
const networkBackend = ref('vk_turn');
const loading = ref(true);
const busy = ref(false);
const error = ref('');
const showImport = ref(false);
const linkText = ref('');
const activeFilter = ref('all'); // all | favorites
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
}

// Map each backend's stored profiles to a common list shape the template renders.
const items = computed(() => {
  if (isXray.value) {
    return xrayProfiles.value.map((p) => ({
      id: p.id,
      title: p.title,
      subtitle: p.port ? `${p.address}:${p.port}` : p.address,
      favorite: p.favorite,
    }));
  }
  return profiles.value
    .filter((p) => (p.transportKind || 'wg') === subBackend.value)
    .map((p) => ({
      id: p.id,
      title: p.title,
      subtitle: `${p.vkTurnEndpoint} (${transportLabel(p.transportKind)})`,
      favorite: p.favorite,
    }));
});

const hasFavorites = computed(() => items.value.some((p) => p.favorite));
const filteredItems = computed(() =>
  activeFilter.value === 'favorites' ? items.value.filter((p) => p.favorite) : items.value,
);

// Fall back to "all" when the favorites filter is active and no favorites remain.
watch(hasFavorites, (has) => {
  if (!has && activeFilter.value === 'favorites') activeFilter.value = 'all';
});

function chipClass(id) {
  return activeFilter.value === id
    ? 'border-transparent bg-wings-accent text-white'
    : 'border-wings-divider bg-wings-surface text-wings-muted hover:text-wings-text';
}

async function setNetworkBackend(kind) {
  apply(await ProfilesService.SetNetworkBackend(kind));
  activeFilter.value = 'all';
}

// Returns true when the mode may be entered. AmneziaWG is blocked (with an install
// prompt) when its tooling is missing, so the app never enters an unusable mode.
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
    return true; // cannot check (pure-vite) -> allow
  }
}

async function setSubBackend(kind) {
  if (!(await ensureSubBackendAllowed(kind))) return; // select reverts to current
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

async function importLink(link) {
  const value = (link ?? '').trim();
  if (!value) return;
  busy.value = true;
  error.value = '';
  try {
    // One smart import understands wingsv:// (VK TURN or Xray, single or many), vless://
    // and other share links, and http(s) subscription URLs - switching backend as needed.
    apply(await ProfilesService.SmartImport(value));
    // For a VK TURN import, follow the imported profile's transport (unless AmneziaWG
    // tooling is missing, which prompts to install and stays put).
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
  const item = items.value.find((p) => p.id === id);
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

onMounted(refresh);
</script>
