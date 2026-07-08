<template>
  <div ref="rootEl" class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <VkLinksView v-if="linksOpen" :client="client" @close="linksOpen = false" />
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
        <h1 class="font-sharp text-[22px] font-bold text-white">Настройки VK TURN</h1>
      </header>

      <div v-if="!ready" class="flex-1">
        <SamsungSectionLoader />
      </div>

      <p v-else-if="!hasProfile" class="px-6 pt-16 text-center text-sm text-wings-muted">
        Не удалось подготовить профиль. Импортируйте ссылку на вкладке «Профили».
      </p>

      <div v-else class="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
        <SamsungCard kicker="Клиент" subtitle="Общие параметры, единые для всех профилей">
          <div class="divide-y divide-wings-divider">
            <button
              type="button"
              class="flex w-full items-center justify-between gap-3 py-3.5 text-left"
              @click="linksOpen = true"
            >
              <span class="text-[17px] text-wings-text">VK-ссылки</span>
              <span class="flex items-center gap-1.5 text-wings-muted">
                <PatchDot :state="patch.vk_links" />
                <span class="text-[15px]">{{ client.vkLinks.length }} шт.</span>
                <ChevronRight :size="18" class="shrink-0" />
              </span>
            </button>
            <div class="flex items-center gap-2 py-3">
              <div class="flex-1">
                <OneuiInput v-model="client.threads" label="Потоки" type="number" inputmode="numeric" />
              </div>
              <PatchDot :state="patch.threads" />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="client.credsGroupSize"
                label="Воркеров на TURN-учётку"
                type="number"
                inputmode="numeric"
              />
            </div>
            <div class="flex items-center gap-2">
              <div class="flex-1">
                <SwitchRow
                  v-model="useVkId"
                  title="Использовать VK ID"
                  subtitle="TURN-креды через вход в VK вместо анонимного режима"
                />
              </div>
              <PatchDot :state="patch.vk_auth" />
            </div>
            <div v-if="useVkId" class="py-3">
              <p class="mb-2 text-sm" :class="vkAuth.loggedIn ? 'text-[#16965A]' : 'text-wings-muted'">
                {{ vkAuth.loggedIn ? 'Вход в VK выполнен' : 'Вход в VK не выполнен' }}
              </p>
              <div class="flex flex-wrap gap-2">
                <SamsungButton variant="secondary" :busy="vkBusy" @click="doVkLogin">
                  {{ vkAuth.loggedIn ? 'Войти заново' : 'Войти в VK' }}
                </SamsungButton>
                <SamsungButton v-if="vkAuth.loggedIn" variant="secondary" :busy="vkBusy" @click="doVkClear">
                  Очистить куки VK
                </SamsungButton>
              </div>
            </div>
            <OneuiSelect label="TURN session mode" v-model="client.turnSessionMode" :options="sessionModeOptions" />
            <OneuiSelect label="Отпечаток браузера" v-model="client.browserFingerprint" :options="browserFpOptions" />
            <OneuiSelect label="Режим" v-model="client.runtimeMode" :options="runtimeModeOptions" />
            <SwitchRow v-model="client.restartOnNetworkChange" title="Перезапуск при смене сети" />
            <div class="py-3">
              <OneuiInput v-model="client.localEndpoint" label="Локальный endpoint" placeholder="127.0.0.1:9000" />
            </div>
          </div>
        </SamsungCard>

        <SamsungCard kicker="Proxy" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <div class="flex items-center gap-2 py-3">
              <div class="flex-1">
                <OneuiInput v-model="form.vkTurnEndpoint" label="Endpoint" placeholder="host:port" />
              </div>
              <PatchDot :state="patch.peer" />
            </div>
            <SwitchRow v-model="form.settings.useUdp" title="Использовать UDP" />
            <SwitchRow v-model="form.settings.noObfuscation" title="Без обфускации" />
            <SwitchRow v-model="form.settings.manualCaptcha" title="Ручная captcha" />
            <OneuiSelect
              label="Авторешение captcha"
              v-model="form.settings.captchaAutoSolver"
              :options="captchaSolverOptions"
            />
            <div class="flex items-center gap-2 py-3">
              <div class="flex-1"><OneuiInput v-model="form.settings.turnHost" label="TURN host (override)" /></div>
              <PatchDot :state="patch.turn_host" />
            </div>
            <div class="flex items-center gap-2 py-3">
              <div class="flex-1"><OneuiInput v-model="form.settings.turnPort" label="TURN port (override)" /></div>
              <PatchDot :state="patch.turn_port" />
            </div>
            <div class="flex items-center gap-2">
              <div class="flex-1">
                <OneuiSelect label="DNS resolver" v-model="form.settings.dnsMode" :options="dnsModeOptions" />
              </div>
              <PatchDot :state="patch.dns" />
            </div>
            <div class="py-3">
              <OneuiTextarea v-model="form.settings.userDns" label="Свои DNS-резолверы" :rows="3" :spellcheck="false" />
            </div>
          </div>
        </SamsungCard>

        <SamsungCard kicker="WRAP" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <div class="flex items-center gap-2">
              <div class="flex-1">
                <OneuiSelect label="Режим WRAP" v-model="form.settings.wrapMode" :options="wrapModeOptions" />
              </div>
              <PatchDot :state="patch.wrap" />
            </div>
            <div class="flex items-center gap-2">
              <div class="flex-1">
                <OneuiSelect label="Шифр WRAP" v-model="form.settings.wrapCipher" :options="wrapCipherOptions" />
              </div>
              <PatchDot :state="patch.wrap" />
            </div>
            <div class="flex items-center gap-2 py-3">
              <div class="flex-1">
                <OneuiInput
                  v-model="form.settings.wrapKeyHex"
                  label="Ключ WRAP (hex)"
                  placeholder="64 hex-символа (32 байта)"
                  :spellcheck="false"
                />
              </div>
              <PatchDot :state="patch.wrap" />
            </div>
            <div class="py-3">
              <SamsungButton variant="secondary" @click="generateWrapKey">Сгенерировать новый ключ</SamsungButton>
            </div>
            <div class="flex items-center gap-2">
              <div class="flex-1">
                <SwitchRow v-model="form.settings.wrapSendKey" title="Передавать ключ in-band" />
              </div>
              <PatchDot :state="patch.wrap" />
            </div>
          </div>
        </SamsungCard>

        <SamsungCard v-if="!isAwg" kicker="WireGuard" :subtitle="managedNote" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <div :class="sectionLabel">Интерфейс</div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.privateKey"
                label="Приватный ключ"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.addresses" label="Адреса" placeholder="10.0.0.2/32" :disabled="isManaged" />
            </div>
            <div class="py-3"><OneuiInput v-model="form.wg.dns" label="DNS" placeholder="1.1.1.1, 1.0.0.1" /></div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.mtu" label="MTU" type="number" inputmode="numeric" :disabled="isManaged" />
            </div>
            <div :class="sectionLabel">Пир</div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.publicKey"
                label="Публичный ключ"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.presharedKey"
                label="Preshared key"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.allowedIps" label="Allowed IPs" placeholder="0.0.0.0/0, ::/0" />
            </div>
            <div class="py-3"><OneuiInput v-model="form.wg.endpoint" label="Endpoint" :disabled="isManaged" /></div>
          </div>
        </SamsungCard>

        <SamsungCard v-else kicker="AmneziaWG" :subtitle="managedNote" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <div :class="sectionLabel">RAW-конфиг</div>
            <div class="py-3">
              <OneuiTextarea
                v-model="rawAwg"
                label="awg-quick"
                :rows="8"
                placeholder="[Interface]&#10;PrivateKey = ...&#10;&#10;[Peer]&#10;..."
                :spellcheck="false"
                :disabled="isManaged"
              />
              <div v-if="!isManaged" class="mt-2">
                <SamsungButton variant="secondary" @click="pasteRawFromClipboard">
                  <template #icon><ClipboardPaste :size="18" /></template>
                  Вставить из буфера
                </SamsungButton>
              </div>
            </div>
            <div :class="sectionLabel">Интерфейс</div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.privateKey"
                label="Приватный ключ"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.addresses" label="Адреса" placeholder="10.0.0.2/32" :disabled="isManaged" />
            </div>
            <div class="py-3"><OneuiInput v-model="form.wg.dns" label="DNS" placeholder="1.1.1.1, 1.0.0.1" /></div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.mtu" label="MTU" type="number" inputmode="numeric" :disabled="isManaged" />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.jc"
                label="Jc — junk-пакетов"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.jmin"
                label="Jmin — мин. размер junk"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.jmax"
                label="Jmax — макс. размер junk"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.s1"
                label="S1 — junk в init"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.s2"
                label="S2 — junk в response"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.s3"
                label="S3 — junk в cookie"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.s4"
                label="S4 — junk в transport"
                type="number"
                inputmode="numeric"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3"><OneuiInput v-model="form.wg.h1" label="H1 — magic init" :disabled="isManaged" /></div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.h2" label="H2 — magic response" :disabled="isManaged" />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.h3" label="H3 — magic underload" :disabled="isManaged" />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.h4" label="H4 — magic transport" :disabled="isManaged" />
            </div>
            <div :class="sectionLabel">Пир</div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.publicKey"
                label="Публичный ключ"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput
                v-model="form.wg.presharedKey"
                label="Preshared key"
                :spellcheck="false"
                :disabled="isManaged"
              />
            </div>
            <div class="py-3">
              <OneuiInput v-model="form.wg.allowedIps" label="Allowed IPs" placeholder="0.0.0.0/0, ::/0" />
            </div>
            <div class="py-3"><OneuiInput v-model="form.wg.endpoint" label="Endpoint" :disabled="isManaged" /></div>
          </div>
        </SamsungCard>

        <p v-if="saveError" class="mt-4 text-sm text-wings-danger">{{ saveError }}</p>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { ChevronLeft, ChevronRight, ClipboardPaste } from 'lucide-vue-next';
import { Clipboard, Events } from '@wailsio/runtime';
import { ProfilesService, VKAuthService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import { closeOverlay } from '@/stores/nav.js';
import { parseAwgQuick, buildAwgQuick } from '@/lib/awgquick.js';
import VkLinksView from '@/views/VkLinksView.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import OneuiTextarea from '@/components/controls/OneuiTextarea.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import SamsungSectionLoader from '@/components/layout/SamsungSectionLoader.vue';
import SwitchRow from '@/components/layout/SwitchRow.vue';
import PatchDot from '@/components/layout/PatchDot.vue';

const sectionLabel = 'px-1 pb-1 pt-4 text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker';

const runtimeModeOptions = [
  { value: 'vpn', label: 'VPN' },
  { value: 'proxy', label: 'Только proxy' },
];
const captchaSolverOptions = [
  { value: 'v2', label: 'Улучшенный (по умолчанию)' },
  { value: 'v1', label: 'Классический' },
];
const sessionModeOptions = [
  { value: 'auto', label: 'Auto' },
  { value: 'mainline', label: 'Mainline' },
  { value: 'mu', label: 'mu/v1' },
];
const browserFpOptions = [
  { value: 'auto', label: 'Авто (случайный)' },
  { value: 'chrome', label: 'Chrome' },
  { value: 'edge', label: 'Edge' },
  { value: 'safari', label: 'Safari' },
  { value: 'firefox', label: 'Firefox' },
];
const dnsModeOptions = [
  { value: 'auto', label: 'Авто (UDP → DoH fallback)' },
  { value: 'udp', label: 'Только UDP/53' },
  { value: 'doh', label: 'Только DNS-over-HTTPS' },
];
const wrapModeOptions = [
  { value: 'off', label: 'Выключено' },
  { value: 'preferred', label: 'Предпочтительно' },
  { value: 'required', label: 'Обязательно' },
];
const wrapCipherOptions = [
  { value: 'srtp-aes-gcm', label: 'SRTP / AES-256-GCM (ARM AES-NI)' },
  { value: 'srtp-chacha20-poly1305', label: 'SRTP / ChaCha20-Poly1305 (программный)' },
];

const ready = ref(false);
const hasProfile = ref(false);
const saveError = ref('');
const form = reactive({ id: '', title: '', vkTurnEndpoint: '', settings: {}, wg: {} });

// Device-global client parameters, shared across every profile.
const client = reactive({
  vkLinks: [],
  vkLinkSecondary: '',
  threads: 24,
  credsGroupSize: 12,
  vkAuthMode: 'anonymous',
  turnSessionMode: 'mu',
  browserFingerprint: 'safari',
  runtimeMode: 'vpn',
  restartOnNetworkChange: true,
  localEndpoint: '127.0.0.1:9000',
});
const linksOpen = ref(false);

// The raw awg-quick editor text; a two-way view over form.wg's structured fields.
const rawAwg = ref('');
let syncing = false;

// The global sub-backend mode (chosen on Профили/Настройки) decides which transport
// section shows.
const subBackend = ref('wg');
const isAwg = computed(() => subBackend.value === 'awg');

// Managed (provisioned) profiles get their WireGuard config minted by the node on
// connect, so the transport fields are read-only - except DNS and Allowed IPs, which
// are the client's to tune.
const isManaged = computed(() => !!form.managed);
const managedNote = computed(() =>
  isManaged.value ? 'Выдаётся узлом при подключении; клиенту доступны только DNS и Allowed IPs' : '',
);

const useVkId = computed({
  get: () => client.vkAuthMode === 'account',
  set: (v) => {
    client.vkAuthMode = v ? 'account' : 'anonymous';
    if (v) refreshVkAuth();
  },
});

// Focusing a control (toggling a switch, tabbing into an input) makes WebKitGTK scroll
// an ancestor to bring it into view; in this fixed-height flex layout that displaces
// the whole page off-screen and never comes back (only the inner scroller is meant to
// scroll). Pin every ancestor of the scroller - and the document - back to 0 on any
// scroll, so stray focus scrolls are undone while the inner scroller keeps working.
const rootEl = ref(null);
function pinAncestors() {
  let el = rootEl.value;
  while (el) {
    if (el.scrollTop) el.scrollTop = 0;
    if (el.scrollLeft) el.scrollLeft = 0;
    el = el.parentElement;
  }
  const se = document.scrollingElement;
  if (se && se.scrollTop) se.scrollTop = 0;
}
onMounted(() => window.addEventListener('scroll', pinAncestors, true));
onBeforeUnmount(() => window.removeEventListener('scroll', pinAncestors, true));

// Live-patch progress from the relay, keyed by the relay field (dns | peer |
// turn_host | turn_port | wrap | vk_auth | threads | vk_links): state is applying |
// failed | reverted_needs_restart; "applied" clears the field. Editing a
// live-patchable setting while connected patches the relay without a restart.
const patch = reactive({});
let patchOff = null;
function onPatch(ev) {
  const d = ev?.data;
  if (!d?.field) return;
  if (d.state === 'applied') delete patch[d.field];
  else patch[d.field] = d.state;
}
onMounted(() => {
  patchOff = Events.On('connection:patch', onPatch);
});
onBeforeUnmount(() => {
  if (patchOff) patchOff();
});

// VK account sign-in state (native WebKitGTK login window on the Go side).
const vkAuth = reactive({ loggedIn: false });
const vkBusy = ref(false);
async function refreshVkAuth() {
  try {
    const s = await VKAuthService.Status();
    vkAuth.loggedIn = !!s?.loggedIn;
  } catch {
    /* keep last known state */
  }
}
async function doVkLogin() {
  vkBusy.value = true;
  try {
    const s = await VKAuthService.Login();
    vkAuth.loggedIn = !!s?.loggedIn;
    saveError.value = '';
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось войти в VK');
  } finally {
    vkBusy.value = false;
  }
}
async function doVkClear() {
  vkBusy.value = true;
  try {
    const s = await VKAuthService.ClearCookies();
    vkAuth.loggedIn = !!s?.loggedIn;
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось очистить куки VK');
  } finally {
    vkBusy.value = false;
  }
}

function generateWrapKey() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  form.settings.wrapKeyHex = Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
}

async function pasteRawFromClipboard() {
  try {
    const text = await Clipboard.Text();
    if (text && text.trim()) rawAwg.value = text.trim();
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось прочитать буфер обмена');
  }
}

function toInt(v, fallback) {
  const n = parseInt(v, 10);
  return Number.isFinite(n) ? n : fallback;
}

// --- profile autosave (per-profile fields) ---
let profileTimer = null;
function scheduleSaveProfile() {
  clearTimeout(profileTimer);
  profileTimer = setTimeout(saveProfile, 400);
}
async function saveProfile() {
  const payload = JSON.parse(JSON.stringify(form));
  payload.wg.mtu = toInt(payload.wg.mtu, 1280);
  // Auto-name a still-placeholder manual profile after its server endpoint host.
  const ep = (payload.vkTurnEndpoint || '').trim();
  if (ep && (!payload.title || payload.title.startsWith('Пустой профиль'))) {
    payload.title = ep.split(':')[0] || ep;
    form.title = payload.title;
  }
  try {
    await ProfilesService.Update(payload);
    saveError.value = '';
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось сохранить настройки');
  }
}

// --- global client autosave ---
let clientTimer = null;
function scheduleSaveClient() {
  clearTimeout(clientTimer);
  clientTimer = setTimeout(saveClient, 400);
}
async function saveClient() {
  const payload = {
    vkLinks: [...client.vkLinks],
    vkLinkSecondary: client.vkLinkSecondary || '',
    threads: toInt(client.threads, 24),
    credsGroupSize: toInt(client.credsGroupSize, 12),
    vkAuthMode: client.vkAuthMode || 'anonymous',
    turnSessionMode: client.turnSessionMode || 'mu',
    browserFingerprint: client.browserFingerprint || 'safari',
    runtimeMode: client.runtimeMode || 'vpn',
    restartOnNetworkChange: !!client.restartOnNetworkChange,
    localEndpoint: client.localEndpoint || '127.0.0.1:9000',
  };
  try {
    await ProfilesService.SetClientSettings(payload);
    saveError.value = '';
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось сохранить параметры клиента');
  }
}

let watchesRegistered = false;
function registerWatches() {
  if (watchesRegistered) return;
  watchesRegistered = true;
  watch(form, scheduleSaveProfile, { deep: true });
  watch(client, scheduleSaveClient, { deep: true });
  // raw -> structured: parse the awg-quick text into form.wg fields.
  watch(
    rawAwg,
    (v) => {
      if (syncing) return;
      syncing = true;
      Object.assign(form.wg, parseAwgQuick(v));
      syncing = false;
    },
    { flush: 'sync' },
  );
  // structured -> raw: rebuild the awg-quick text when a field is edited directly.
  watch(
    () => JSON.stringify(form.wg),
    () => {
      if (syncing) return;
      syncing = true;
      rawAwg.value = buildAwgQuick(form.wg);
      syncing = false;
    },
    { flush: 'sync' },
  );
}

function bindForm(active) {
  Object.assign(form, JSON.parse(JSON.stringify(active)));
  rawAwg.value = buildAwgQuick(form.wg);
  hasProfile.value = true;
  registerWatches();
}

function bindClient(c) {
  if (!c) return;
  client.vkLinks = Array.isArray(c.vkLinks) ? [...c.vkLinks] : [];
  client.vkLinkSecondary = c.vkLinkSecondary || '';
  client.threads = c.threads || 24;
  client.credsGroupSize = c.credsGroupSize || 12;
  client.vkAuthMode = c.vkAuthMode || 'anonymous';
  client.turnSessionMode = c.turnSessionMode || 'mu';
  client.browserFingerprint = c.browserFingerprint || 'safari';
  client.runtimeMode = c.runtimeMode || 'vpn';
  client.restartOnNetworkChange = c.restartOnNetworkChange !== false;
  client.localEndpoint = c.localEndpoint || '127.0.0.1:9000';
}

onMounted(async () => {
  try {
    let result = await ProfilesService.List();
    subBackend.value = result.subBackend || 'wg';
    bindClient(result.client);
    if (client.vkAuthMode === 'account') refreshVkAuth();
    let active = (result.profiles ?? []).find((p) => p.id === result.activeId);
    if (!active) {
      // No profile for this backend yet: create an empty one to fill in by hand.
      result = await ProfilesService.CreateProfile();
      subBackend.value = result.subBackend || subBackend.value;
      active = (result.profiles ?? []).find((p) => p.id === result.activeId);
    }
    if (active) bindForm(active);
  } catch (e) {
    saveError.value = String(e?.message ?? e ?? 'Не удалось подготовить профиль');
  } finally {
    ready.value = true;
  }
});
</script>
