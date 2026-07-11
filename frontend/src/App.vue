<template>
  <div ref="rootEl" class="flex h-screen flex-col overflow-clip bg-wings-page font-samsung text-wings-text">
    <VkTurnSettingsView v-if="overlay === 'vkturn-settings'" />
    <LogsView v-else-if="overlay === 'logs'" />
    <AboutView v-else-if="overlay === 'about'" />

    <template v-else>
      <main class="min-h-0 flex-1 overflow-y-auto">
        <component :is="activeView" />
      </main>

      <nav class="flex shrink-0 items-stretch border-t border-wings-divider bg-wings-page px-1 pb-1.5 pt-2">
        <button
          v-for="tab in tabs"
          :key="tab.id"
          type="button"
          class="flex flex-1 flex-col items-center gap-1 rounded-xl py-1.5 transition-colors"
          :class="tab.id === active ? 'text-wings-accent' : 'text-wings-muted'"
          @click="active = tab.id"
        >
          <span class="relative">
            <component :is="tab.icon" :size="22" :stroke-width="tab.id === active ? 2.4 : 1.9" />
            <span
              v-if="tab.id === 'settings' && updateAvailable"
              class="absolute -right-1.5 -top-0.5 h-2.5 w-2.5 rounded-full border-2 border-wings-page bg-red-500"
            />
          </span>
          <span class="text-[11px] font-medium leading-none">{{ tab.label }}</span>
        </button>
      </nav>
    </template>

    <FirstLaunchView v-if="showOnboarding" />

    <ConfirmDialog />
    <ToastHost />
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { House, User, LayoutGrid, Settings } from 'lucide-vue-next';
import { Events } from '@wailsio/runtime';
import HomeView from '@/views/HomeView.vue';
import ProfilesView from '@/views/ProfilesView.vue';
import AppsView from '@/views/AppsView.vue';
import SettingsView from '@/views/SettingsView.vue';
import VkTurnSettingsView from '@/views/VkTurnSettingsView.vue';
import LogsView from '@/views/LogsView.vue';
import AboutView from '@/views/AboutView.vue';
import ConfirmDialog from '@/components/layout/ConfirmDialog.vue';
import ToastHost from '@/components/layout/ToastHost.vue';
import FirstLaunchView from '@/views/firstlaunch/FirstLaunchView.vue';
import { OnboardingService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import { overlay } from '@/stores/nav.js';
import { showOnboarding, openOnboarding } from '@/stores/onboarding.js';
import { updateAvailable, startUpdatePolling } from '@/stores/update.js';
import { showToast } from '@/stores/toast.js';

// Relay field -> human label for patch toasts.
const PATCH_LABELS = {
  dns: 'DNS',
  peer: 'Endpoint',
  turn_host: 'TURN host',
  turn_port: 'TURN port',
  wrap: 'WRAP',
  vk_auth: 'VK ID',
  threads: 'Потоки',
  vk_links: 'VK-ссылки',
};

const tabs = [
  { id: 'home', label: 'Главная', icon: House, view: HomeView },
  { id: 'profiles', label: 'Профили', icon: User, view: ProfilesView },
  { id: 'apps', label: 'Приложения', icon: LayoutGrid, view: AppsView },
  { id: 'settings', label: 'Настройки', icon: Settings, view: SettingsView },
];

const active = ref('home');
const activeView = computed(() => tabs.find((tab) => tab.id === active.value).view);

// Global WebKitGTK focus-scroll guard: focusing a control makes WebKit scroll a
// non-scrolling ancestor to reveal it, displacing the fixed-height layout off-screen
// permanently. Pin the app shell and the document back to 0 on any scroll; the real
// scrollers (<main>, the settings/links panes) are descendants and keep working.
const rootEl = ref(null);
function pinShell() {
  if (rootEl.value && rootEl.value.scrollTop) rootEl.value.scrollTop = 0;
  const app = document.getElementById('app');
  if (app && app.scrollTop) app.scrollTop = 0;
  const se = document.scrollingElement;
  if (se && se.scrollTop) se.scrollTop = 0;
}
onMounted(() => window.addEventListener('scroll', pinShell, true));
onBeforeUnmount(() => window.removeEventListener('scroll', pinShell, true));

// Global toasts: a settings change that forced a reconnect, and a live-patch that
// could only take effect on the next restart (or failed).
const offs = [];
onMounted(async () => {
  try {
    if (!(await OnboardingService.Seen())) openOnboarding();
  } catch {
    // backend not available (pure-vite preview) -> skip onboarding
  }
  startUpdatePolling();
  offs.push(Events.On('connection:notice', (ev) => showToast(ev?.data?.message, { type: ev?.data?.kind || 'info' })));
  offs.push(
    Events.On('connection:patch', (ev) => {
      const d = ev?.data;
      if (!d) return;
      if (d.state === 'reverted_needs_restart' || d.state === 'failed') {
        const label = PATCH_LABELS[d.field] || d.field;
        showToast(`«${label}» применится после перезапуска`, { type: 'warn' });
      }
    }),
  );
});
onBeforeUnmount(() => offs.forEach((off) => off && off()));
</script>
