<template>
  <div class="pb-6">
    <AppHeader />

    <div class="px-4">
      <SamsungCard kicker="Backend">
        <div class="divide-y divide-wings-divider">
          <div class="flex flex-col py-3.5">
            <span class="text-[17px]">Сетевой backend</span>
            <span class="mt-0.5 text-sm text-wings-muted">VK TURN</span>
          </div>
          <OneuiSelect
            label="Под-backend"
            :model-value="subBackend"
            :options="subBackendOptions"
            @update:model-value="setSubBackend"
          />
          <button
            type="button"
            class="flex w-full items-center justify-between py-3.5 text-left"
            @click="openOverlay('vkturn-settings')"
          >
            <span class="flex flex-col">
              <span class="text-[17px]">Настройки VK TURN</span>
              <span class="mt-0.5 text-sm text-wings-muted">Proxy, transport и WireGuard параметры</span>
            </span>
            <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
          </button>
          <button
            type="button"
            class="flex w-full items-center justify-between py-3.5 text-left"
            @click="openOverlay('logs')"
          >
            <span class="flex flex-col">
              <span class="text-[17px]">Журнал</span>
              <span class="mt-0.5 text-sm text-wings-muted">Runtime и proxy события подключения</span>
            </span>
            <ChevronRight :size="20" class="shrink-0 text-wings-muted" />
          </button>
        </div>
      </SamsungCard>

      <SamsungCard kicker="Информация" class="mt-5">
        <button
          type="button"
          class="flex w-full items-center justify-between py-2 text-left"
          @click="openOverlay('about')"
        >
          <span class="flex flex-col">
            <span class="text-[17px]">О приложении</span>
            <span class="mt-0.5 text-sm text-wings-muted">Версия, разработчики, исходный код и лицензии</span>
          </span>
          <span class="flex shrink-0 items-center gap-2">
            <span
              v-if="updateAvailable"
              class="flex h-5 w-5 items-center justify-center rounded-full bg-red-500 text-[11px] font-bold leading-none text-white"
            >
              N
            </span>
            <ChevronRight :size="20" class="text-wings-muted" />
          </span>
        </button>
      </SamsungCard>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue';
import { ChevronRight } from 'lucide-vue-next';
import { ConnectionService, ProfilesService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import AppHeader from '@/components/layout/AppHeader.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import { confirm } from '@/stores/confirm.js';
import { openOverlay } from '@/stores/nav.js';
import { updateAvailable } from '@/stores/update.js';

const subBackendOptions = [
  { value: 'wg', label: 'WireGuard' },
  { value: 'awg', label: 'AmneziaWG' },
];
const subBackend = ref('wg');

onMounted(async () => {
  try {
    const result = await ProfilesService.List();
    subBackend.value = result.subBackend || 'wg';
  } catch {
    // backend not available (pure-vite preview)
  }
});

async function setSubBackend(kind) {
  // Block AmneziaWG (with an install prompt) when its tooling is missing; the select
  // stays on the current value because it is bound to subBackend.
  if (kind === 'awg') {
    try {
      const info = await ConnectionService.AWGAvailability();
      if (!info.available) {
        await confirm({
          title: 'AmneziaWG недоступен',
          message: `AmneziaWG недоступен на этой машине. Установите пакеты:\n\n${info.packages.join('\n')}`,
          confirmText: 'Понятно',
          cancelText: '',
        });
        return;
      }
    } catch {
      // cannot check (pure-vite) -> allow
    }
  }
  subBackend.value = kind;
  try {
    await ProfilesService.SetSubBackend(kind);
  } catch {
    // ignore persist failure
  }
}
</script>
