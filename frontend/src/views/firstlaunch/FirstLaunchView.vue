<template>
  <div class="fl-root" :class="{ 'fl-exiting': exiting }">
    <!-- Drifting SUW sky gradient. Outer wrap carries the per-step parallax; the img
         carries the slow ambient drift so the two transforms do not fight. -->
    <div class="fl-bg-wrap" :style="{ transform: `translateY(${parallaxY}px)` }">
      <img class="fl-bg" src="/onboarding/suw_intro_bg.webp" alt="" />
    </div>
    <div class="fl-scrim"></div>

    <!-- SUW intro, fades out to reveal the static gradient. An animated WebP played as an
         image, not a <video>: WebKitGTK here has no working media backend (a <video> never
         gets past loadstart), while animated WebP decodes as a normal image. -->
    <img
      v-if="showVideo && introSrc"
      class="fl-intro"
      :class="{ 'fl-intro-hidden': videoDone }"
      :src="introSrc"
      alt=""
    />

    <div class="fl-content">
      <Transition name="fl-step" mode="out-in">
        <component
          :is="current.comp"
          :key="stepId"
          v-bind="current.props"
          @next="handleNext"
          @skip="complete"
          @choice="handleChoice"
          @import="handleVkImport"
          @finish="complete"
        />
      </Transition>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ShieldCheck, Radar, ListChecks, ScanSearch } from 'lucide-vue-next';
import { Clipboard } from '@wailsio/runtime';
import { ProfilesService, OnboardingService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import { closeOnboarding } from '@/stores/onboarding.js';
import { showToast } from '@/stores/toast.js';
import IntroStep from '@/views/firstlaunch/IntroStep.vue';
import ConnectionStep from '@/views/firstlaunch/ConnectionStep.vue';
import VkTurnStep from '@/views/firstlaunch/VkTurnStep.vue';
import XrayStep from '@/views/firstlaunch/XrayStep.vue';
import StubStep from '@/views/firstlaunch/StubStep.vue';
import DoneStep from '@/views/firstlaunch/DoneStep.vue';

// permissions/xray/auto-search have no desktop equivalent yet, so they reuse StubStep.
const STEPS = {
  intro: { comp: IntroStep },
  permissions: {
    comp: StubStep,
    props: {
      title: 'Разрешения',
      subtitle:
        'При подключении WINGS V DeX запросит повышение прав (pkexec на Linux, UAC на Windows), чтобы поднять WireGuard.',
      icon: ShieldCheck,
    },
  },
  connection: { comp: ConnectionStep },
  vkturn: { comp: VkTurnStep },
  xray: { comp: XrayStep },
  autosearch_settings: {
    comp: StubStep,
    props: {
      title: 'Автопоиск',
      subtitle: 'Автоматический подбор профилей появится в Dex позже.',
      icon: Radar,
    },
  },
  autosearch_mode: {
    comp: StubStep,
    props: { title: 'Режим автопоиска', subtitle: 'Выбор режима проверки появится в Dex позже.', icon: ListChecks },
  },
  autosearch_run: {
    comp: StubStep,
    props: { title: 'Проверка профилей', subtitle: 'Прогон профилей появится в Dex позже.', icon: ScanSearch },
  },
  done: { comp: DoneStep },
};
const ORDER = Object.keys(STEPS);

const stepId = ref('intro');
const current = computed(() => STEPS[stepId.value]);
const parallaxY = computed(() => -ORDER.indexOf(stepId.value) * 10);

const showVideo = ref(true);
const videoDone = ref(false);
const introSrc = ref('');
const exiting = ref(false);
let videoTimer = null;
let exitTimer = null;
let gateTimer = null;
let introStarted = false;

onMounted(() => {
  // Start the intro only once the OS window is actually presented, otherwise the ~1.9s
  // WebP can play (and finish) while the window is still coming up and the user misses it.
  if (document.visibilityState === 'visible' && document.hasFocus()) {
    beginIntro();
    return;
  }
  window.addEventListener('focus', beginIntro, { once: true });
  document.addEventListener('visibilitychange', onVisible);
  // Fallback so it never gets stuck if neither event fires.
  gateTimer = setTimeout(beginIntro, 2500);
});

onBeforeUnmount(() => {
  clearTimeout(videoTimer);
  clearTimeout(exitTimer);
  clearTimeout(gateTimer);
  window.removeEventListener('focus', beginIntro);
  document.removeEventListener('visibilitychange', onVisible);
});

function onVisible() {
  if (document.visibilityState === 'visible') beginIntro();
}

function beginIntro() {
  if (introStarted) return;
  introStarted = true;
  clearTimeout(gateTimer);
  window.removeEventListener('focus', beginIntro);
  document.removeEventListener('visibilitychange', onVisible);
  // Setting the src now (re)starts the animated WebP from frame 0 with the window visible.
  introSrc.value = '/onboarding/suw_intro_in.webp';
  // The WebP runs ~1.9s; then fade it out to reveal the drifting gradient.
  videoTimer = setTimeout(finishVideo, 1900);
}

function finishVideo() {
  if (videoDone.value) return;
  videoDone.value = true;
  clearTimeout(videoTimer);
  // Drop the element after the fade so it stops animating.
  setTimeout(() => {
    showVideo.value = false;
  }, 700);
}

function goTo(id) {
  stepId.value = id;
}

function handleNext() {
  const map = {
    intro: 'permissions',
    permissions: 'connection',
    vkturn: 'done',
    xray: 'done',
    autosearch_settings: 'autosearch_mode',
    autosearch_mode: 'autosearch_run',
    autosearch_run: 'done',
  };
  const to = map[stepId.value];
  if (to) goTo(to);
}

function handleChoice(choice) {
  if (choice === 'vk_turn') return goTo('vkturn');
  if (choice === 'xray') return goTo('xray');
  if (choice === 'auto_search') return goTo('autosearch_settings');
  if (choice === 'qr') return showToast('Сканирование QR недоступно на десктопе', { type: 'info' });
  if (choice === 'paste') return importFromClipboard(true);
}

function handleVkImport() {
  importFromClipboard(false);
}

// A successful import from the connection step ends onboarding; from the VK TURN step it
// advances to the Done screen.
async function importFromClipboard(finishOnSuccess) {
  let text = '';
  try {
    text = await Clipboard.Text();
  } catch {
    // no clipboard access
  }
  if (!text || !text.trim()) {
    showToast('Буфер обмена пуст', { type: 'warn' });
    return;
  }
  try {
    // Understands wingsv:// (VK TURN or Xray), vless:// and http(s) subscription URLs,
    // switching the network backend to match what was pasted.
    await ProfilesService.SmartImport(text.trim());
  } catch {
    showToast('В буфере нет подходящей ссылки или подписки', { type: 'warn' });
    return;
  }
  showToast('Профиль импортирован', { type: 'info' });
  if (finishOnSuccess) complete();
  else goTo('done');
}

function complete() {
  if (exiting.value) return;
  exiting.value = true;
  OnboardingService.MarkSeen().catch(() => {});
  exitTimer = setTimeout(closeOnboarding, 850);
}
</script>

<style scoped>
.fl-root {
  position: fixed;
  inset: 0;
  z-index: 60;
  overflow: hidden;
  background: #2a4a6a;
  transition: opacity 0.8s cubic-bezier(0.22, 0.25, 0, 1);
}
.fl-exiting {
  opacity: 0;
}
.fl-bg-wrap {
  position: absolute;
  inset: 0;
  transition: transform 1.1s cubic-bezier(0.22, 0.25, 0, 1);
}
.fl-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transform: scale(1.18);
  transform-origin: center;
  animation: fl-drift 26s ease-in-out infinite;
  will-change: transform;
}
@keyframes fl-drift {
  0% {
    transform: scale(1.18) translate(0, 0);
  }
  25% {
    transform: scale(1.18) translate(1.4%, -1.2%);
  }
  50% {
    transform: scale(1.18) translate(0, 1.2%);
  }
  75% {
    transform: scale(1.18) translate(-1.4%, -0.8%);
  }
  100% {
    transform: scale(1.18) translate(0, 0);
  }
}
/* Faint top scrim so the status area / skip pill stays legible over the sky. */
.fl-scrim {
  position: absolute;
  inset: 0 0 auto 0;
  height: 22%;
  background: linear-gradient(to bottom, rgba(8, 29, 64, 0.2), rgba(8, 29, 64, 0));
  pointer-events: none;
}
.fl-intro {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  /* A background layer (below the step content, above the static gradient): it plays over
     the static sky and fades out to reveal it, while the title/buttons stay on top. */
  pointer-events: none;
  transition: opacity 0.65s cubic-bezier(0.22, 0.25, 0, 1);
}
.fl-intro-hidden {
  opacity: 0;
}
.fl-content {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
}
.fl-content > * {
  flex: 1;
  min-height: 0;
}

.fl-step-enter-active {
  transition:
    opacity 0.6s cubic-bezier(0.22, 0.25, 0, 1),
    transform 0.6s cubic-bezier(0.22, 0.25, 0, 1);
}
.fl-step-leave-active {
  transition:
    opacity 0.5s cubic-bezier(0.22, 0.25, 0, 1),
    transform 0.5s cubic-bezier(0.22, 0.25, 0, 1);
}
.fl-step-enter-from {
  opacity: 0;
  transform: translateY(18px);
}
.fl-step-leave-to {
  opacity: 0;
  transform: translateY(-18px);
}
</style>
