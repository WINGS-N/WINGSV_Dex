<template>
  <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <OpenSourceLicensesView v-if="oslOpen" @close="oslOpen = false" />

    <template v-else>
      <header class="flex shrink-0 items-center gap-2 px-3 pb-3 pt-6">
        <button
          type="button"
          class="rounded-full p-1.5 text-wings-mutedStrong hover:text-wings-text"
          aria-label="Назад"
          @click="closeOverlay"
        >
          <ChevronLeft :size="24" />
        </button>
        <h1 class="font-sharp text-[22px] font-bold text-white">О приложении</h1>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
        <!-- App identity -->
        <SamsungCard>
          <div class="flex cursor-pointer select-none items-center gap-4 py-1" @click="tapAppIcon">
            <img src="/appicon.png" alt="WINGS V DeX" class="h-16 w-16 shrink-0 rounded-2xl" />
            <span class="flex flex-col">
              <span class="text-[22px] font-bold text-white">WINGS V DeX</span>
              <span class="mt-0.5 text-[17px] text-wings-muted">Версия {{ version }}</span>
            </span>
          </div>
        </SamsungCard>

        <!-- Update -->
        <SamsungCard kicker="Обновление приложения" class="mt-5">
          <button
            type="button"
            class="flex w-full items-center justify-between gap-3 py-2 text-left disabled:opacity-100"
            :disabled="busy"
            @click="onUpdateClick"
          >
            <span class="flex min-w-0 flex-1 flex-col">
              <span class="text-[17px] text-wings-text">{{ updateTitle }}</span>
              <span class="mt-0.5 text-sm text-wings-muted">{{ updateSummary }}</span>
              <span
                v-if="installing && progress.total > 0"
                class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-white/10"
              >
                <span class="block h-full rounded-full bg-wings-accent transition-all" :style="{ width: `${pct}%` }" />
              </span>
            </span>
            <SamsungSpinner v-if="busy" class="shrink-0" />
            <component v-else :is="isAvailable ? Download : RefreshCw" :size="20" class="shrink-0 text-wings-muted" />
          </button>
        </SamsungCard>

        <!-- Developers -->
        <SamsungCard kicker="Разработчики" class="mt-5">
          <button type="button" class="flex w-full items-center gap-3.5 py-2 text-left" @click="open(WINGS_N_URL)">
            <AvatarCircle username="WINGS-N" initials="WN" color="#2D6BE5" :size="48" />
            <span class="flex min-w-0 flex-1 flex-col">
              <span class="text-[17px] text-wings-text">WINGS-N</span>
              <span class="mt-0.5 text-sm text-wings-muted">Йоу печенье, программное обеспечение</span>
            </span>
            <ChevronRight :size="18" class="shrink-0 text-wings-muted" />
          </button>
        </SamsungCard>

        <!-- Source code and licenses -->
        <SamsungCard kicker="Исходный код и лицензии" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <button
              type="button"
              class="flex w-full items-center justify-between py-3.5 text-left"
              @click="open(SOURCE_URL)"
            >
              <span class="flex flex-col">
                <span class="text-[17px] text-wings-text">Исходный код</span>
                <span class="mt-0.5 text-sm text-wings-muted">github.com/WINGS-N/WINGSV_DeX</span>
              </span>
              <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
            </button>
            <button
              type="button"
              class="flex w-full items-center justify-between py-3.5 text-left"
              @click="oslOpen = true"
            >
              <span class="flex flex-col">
                <span class="text-[17px] text-wings-text">Лицензии свободного ПО</span>
                <span class="mt-0.5 text-sm text-wings-muted">Основные open-source компоненты приложения</span>
              </span>
              <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
            </button>
          </div>
        </SamsungCard>

        <!-- Telegram -->
        <SamsungCard kicker="Telegram" class="mt-5">
          <button
            type="button"
            class="flex w-full items-center justify-between py-2 text-left"
            @click="open(TELEGRAM_URL)"
          >
            <span class="flex flex-col">
              <span class="text-[17px] text-wings-text">Telegram чат WINGS V</span>
              <span class="mt-0.5 text-sm text-wings-muted">Обсуждение, предложения и помощь с настройкой</span>
            </span>
            <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
          </button>
        </SamsungCard>

        <!-- Special thanks -->
        <SamsungCard kicker="Special thanks to" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <button
              v-for="credit in credits"
              :key="credit.title"
              type="button"
              class="flex w-full items-center gap-3.5 py-3.5 text-left"
              @click="open(credit.url)"
            >
              <AvatarCircle
                :src="credit.src"
                :username="credit.username"
                :initials="credit.initials"
                :color="credit.color"
                :contain="credit.contain"
                :size="44"
              />
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="text-[17px] text-wings-text">{{ credit.title }}</span>
                <span class="mt-0.5 text-sm text-wings-muted">{{ credit.summary }}</span>
              </span>
              <ChevronRight :size="18" class="shrink-0 text-wings-muted" />
            </button>
          </div>
        </SamsungCard>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ChevronLeft, ChevronRight, Download, RefreshCw } from 'lucide-vue-next';
import { Browser, Events } from '@wailsio/runtime';
import { AboutService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungSpinner from '@/components/layout/SamsungSpinner.vue';
import AvatarCircle from '@/components/layout/AvatarCircle.vue';
import OpenSourceLicensesView from '@/views/OpenSourceLicensesView.vue';
import { closeOverlay } from '@/stores/nav.js';
import { openOnboarding } from '@/stores/onboarding.js';

const WINGS_N_URL = 'https://github.com/WINGS-N';
const SOURCE_URL = 'https://github.com/WINGS-N/WINGSV_DeX';
const TELEGRAM_URL = 'https://t.me/+KrgCVOtwL980ZDky';
const UPDATE_PROGRESS_EVENT = 'update:progress';
const UPDATE_STATE_EVENT = 'update:state';

const oslOpen = ref(false);
const version = ref('0.1.1');

// Updater state.
const KIND_LABELS = {
  deb: 'установлено через dpkg (.deb)',
  rpm: 'установлено через rpm',
  appimage: 'AppImage',
  'windows-setup': 'установлено через Setup',
  'windows-portable': 'переносимая версия',
  binary: 'бинарный файл',
};
const update = ref(null);
const checking = ref(false);
const installing = ref(false);
const stateMsg = ref('');
const progress = ref({ phase: '', done: 0, total: 0 });

const busy = computed(() => checking.value || installing.value);
const isAvailable = computed(() => update.value?.status === 'available');
const pct = computed(() =>
  progress.value.total > 0 ? Math.round((progress.value.done / progress.value.total) * 100) : 0,
);

const updateTitle = computed(() => {
  if (installing.value) return stateMsg.value || 'Установка обновления...';
  if (checking.value) return 'Проверяем обновления...';
  if (isAvailable.value) return `Обновить до ${update.value.latest}`;
  if (update.value?.status === 'error') return 'Не удалось проверить обновления';
  return 'Обновление не требуется';
});
const updateSummary = computed(() => {
  if (installing.value) {
    const label = progress.value.phase === 'download-vk' ? 'Загрузка компонента' : 'Загрузка';
    if (progress.value.phase.startsWith('download') && progress.value.total > 0) return `${label} ${pct.value}%`;
    return 'Применение обновления...';
  }
  if (checking.value) return 'Подключение и проверка последнего релиза...';
  if (isAvailable.value) {
    const label = KIND_LABELS[update.value.kind] || 'нажмите, чтобы обновить';
    return `${label}. Нажмите, чтобы обновить`;
  }
  if (update.value?.status === 'error') return 'Нажмите, чтобы повторить';
  return `Установлена последняя версия ${version.value}. Нажмите, чтобы проверить ещё раз`;
});

async function checkUpdate() {
  if (busy.value) return;
  checking.value = true;
  try {
    update.value = await AboutService.CheckUpdate();
  } catch {
    update.value = { status: 'error' };
  }
  checking.value = false;
}

function onUpdateClick() {
  if (busy.value) return;
  if (isAvailable.value) {
    // No installable asset for this build -> just open the download page.
    if (!update.value.appAsset) {
      Browser.OpenURL(update.value.pageUrl || SOURCE_URL).catch(() => {});
      return;
    }
    installing.value = true;
    stateMsg.value = 'Установка обновления...';
    progress.value = { phase: '', done: 0, total: 0 };
    AboutService.ApplyUpdate();
    return;
  }
  checkUpdate();
}

const updateOffs = [];

// GitHub avatars load remotely and fall back to the initials disc.
const credits = [
  {
    title: 'Samsung',
    summary: 'создание One UI и дизайн интерфейсов',
    src: '/samsung_black_wtext.jpg',
    initials: 'S',
    color: '#000000',
    url: 'https://www.samsung.com/',
  },
  {
    title: 'tribalfs',
    summary: 'oneui-design, sesl-androidx и SESL 8',
    username: 'tribalfs',
    initials: 'TF',
    color: '#1E8E5A',
    url: 'https://github.com/tribalfs',
  },
  {
    title: 'Yanndroid',
    summary: 'ранние One UI Android-библиотеки и идеи UI архитектуры',
    username: 'Yanndroid',
    initials: 'YN',
    color: '#F18A27',
    url: 'https://github.com/Yanndroid',
  },
  {
    title: 'salvogiangri',
    summary: 'One UI модификации и совместные SESL наработки',
    username: 'salvogiangri',
    initials: 'SG',
    color: '#9A5C2F',
    url: 'https://github.com/salvogiangri',
  },
  {
    title: 'zx2c4',
    summary: 'WireGuard и tunnel backend',
    username: 'zx2c4',
    initials: 'ZX',
    color: '#51657A',
    url: 'https://github.com/zx2c4',
  },
  {
    title: 'cacggghp',
    summary: 'автор vk-turn-proxy',
    username: 'cacggghp',
    initials: 'CC',
    color: '#685ACF',
    url: 'https://github.com/cacggghp',
  },
  {
    title: 'Moroka8',
    summary: 'идеи и наработки по VK captcha bypass для vk-turn-proxy',
    username: 'Moroka8',
    initials: 'M8',
    color: '#8E5A2B',
    url: 'https://github.com/Moroka8',
  },
  {
    title: 'samosvalishe',
    summary: 'обход фильтрации трафика VK через подражание SRTP',
    username: 'samosvalishe',
    initials: 'SV',
    color: '#7B3F8F',
    url: 'https://github.com/samosvalishe',
  },
  {
    title: 'Amnezia VPN',
    summary: 'AmneziaWG backend',
    username: 'amnezia-vpn',
    initials: 'AW',
    color: '#1B8B73',
    url: 'https://github.com/amnezia-vpn',
  },
];

onMounted(async () => {
  try {
    version.value = await AboutService.Version();
  } catch {
    // backend not available (pure-vite preview) -> keep the fallback
  }
  updateOffs.push(
    Events.On(UPDATE_PROGRESS_EVENT, (ev) => {
      if (ev?.data) {
        progress.value = ev.data;
        installing.value = true;
      }
    }),
  );
  updateOffs.push(
    Events.On(UPDATE_STATE_EVENT, (ev) => {
      const s = ev?.data?.state;
      if (s === 'installing') {
        installing.value = true;
        stateMsg.value = 'Установка обновления...';
      } else if (s === 'restarting') {
        stateMsg.value = 'Перезапуск...';
      } else if (s === 'error') {
        installing.value = false;
        update.value = { status: 'error' };
      } else if (s === 'uptodate') {
        installing.value = false;
        checkUpdate();
      }
    }),
  );
  checkUpdate();
});

onBeforeUnmount(() => updateOffs.forEach((off) => off && off()));

function open(url) {
  Browser.OpenURL(url).catch(() => {});
}

// Easter egg: 5 taps within 2s replay the SUW intro with the Over the Horizon track.
let tapCount = 0;
let tapStartedAt = 0;
function tapAppIcon() {
  const now = Date.now();
  if (now - tapStartedAt > 2000) tapCount = 0;
  if (tapCount === 0) tapStartedAt = now;
  tapCount += 1;
  if (tapCount >= 5) {
    tapCount = 0;
    closeOverlay();
    openOnboarding({ music: true });
  }
}
</script>
