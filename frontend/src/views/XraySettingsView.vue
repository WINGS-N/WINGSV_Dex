<template>
  <div ref="rootEl" class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <header class="flex shrink-0 items-center gap-2 px-3 pb-3 pt-6">
      <button
        type="button"
        class="rounded-full p-1.5 text-wings-mutedStrong hover:text-wings-text"
        aria-label="Назад"
        @click="closeOverlay"
      >
        <ChevronLeft :size="24" />
      </button>
      <h1 class="font-sharp text-[22px] font-bold text-white">Настройки Xray</h1>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
      <SamsungCard kicker="Режим">
        <div class="divide-y divide-wings-divider">
          <OneuiSelect
            label="Режим работы"
            v-model="form.runtimeMode"
            :options="runtimeOptions"
            @update:model-value="save"
          />
          <OneuiSelect
            label="Транспорт"
            v-model="form.transportMode"
            :options="transportOptions"
            @update:model-value="save"
          />
          <SwitchRow title="IPv6" v-model="form.ipv6" @update:model-value="save" />
          <SwitchRow
            title="Sniffing"
            subtitle="Определять домен по трафику для маршрутизации"
            v-model="form.sniffingEnabled"
            @update:model-value="save"
          />
          <SwitchRow
            title="Пропускать QUIC"
            subtitle="Иначе QUIC (UDP 443) блокируется"
            v-model="form.proxyQuicEnabled"
            @update:model-value="save"
          />
          <SwitchRow
            title="Разрешать небезопасные сертификаты"
            :model-value="form.allowInsecure"
            @update:model-value="onAllowInsecure"
          />
          <SwitchRow title="Доступ из локальной сети" v-model="form.allowLan" @update:model-value="save" />
          <SwitchRow
            title="Перезапуск при смене сети"
            v-model="form.restartOnNetworkChange"
            @update:model-value="save"
          />
        </div>
      </SamsungCard>

      <SamsungCard kicker="DNS" class="mt-5">
        <div class="divide-y divide-wings-divider">
          <OneuiInput label="Удалённый DNS" v-model="form.remoteDns" @update:model-value="saveDebounced" />
          <OneuiInput label="Прямой DNS" v-model="form.directDns" @update:model-value="saveDebounced" />
        </div>
      </SamsungCard>

      <SamsungCard kicker="Локальный SOCKS-прокси" class="mt-5">
        <div class="divide-y divide-wings-divider">
          <SwitchRow title="Включить SOCKS" v-model="form.localProxyEnabled" @update:model-value="save" />
          <template v-if="form.localProxyEnabled">
            <OneuiInput label="Адрес" v-model="form.localProxyListenAddress" @update:model-value="saveDebounced" />
            <OneuiInput label="Порт" type="number" v-model="form.localProxyPort" @update:model-value="saveDebounced" />
            <SwitchRow
              title="Пароль"
              subtitle="Защитить локальный прокси логином и паролем"
              :model-value="form.localProxyAuthEnabled"
              @update:model-value="(v) => onAuthToggle('local', v)"
            />
            <template v-if="form.localProxyAuthEnabled">
              <OneuiInput label="Логин" v-model="form.localProxyUsername" @update:model-value="saveDebounced" />
              <OneuiInput
                label="Пароль"
                :model-value="form.localProxyPassword"
                @update:model-value="(v) => onPassword('local', v)"
              />
            </template>
          </template>
        </div>
      </SamsungCard>

      <SamsungCard kicker="Локальный HTTP-прокси" class="mt-5">
        <div class="divide-y divide-wings-divider">
          <SwitchRow title="Включить HTTP" v-model="form.httpProxyEnabled" @update:model-value="save" />
          <template v-if="form.httpProxyEnabled">
            <OneuiInput label="Адрес" v-model="form.httpProxyListenAddress" @update:model-value="saveDebounced" />
            <OneuiInput label="Порт" type="number" v-model="form.httpProxyPort" @update:model-value="saveDebounced" />
            <SwitchRow
              title="Пароль"
              subtitle="Защитить локальный прокси логином и паролем"
              :model-value="form.httpProxyAuthEnabled"
              @update:model-value="(v) => onAuthToggle('http', v)"
            />
            <template v-if="form.httpProxyAuthEnabled">
              <OneuiInput label="Логин" v-model="form.httpProxyUsername" @update:model-value="saveDebounced" />
              <OneuiInput
                label="Пароль"
                :model-value="form.httpProxyPassword"
                @update:model-value="(v) => onPassword('http', v)"
              />
            </template>
          </template>
        </div>
      </SamsungCard>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive } from 'vue';
import { ChevronLeft } from 'lucide-vue-next';
import { ProfilesService } from '@bindings/github.com/WINGS-N/wingsv-dex/internal/services';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import OneuiSelect from '@/components/controls/OneuiSelect.vue';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SwitchRow from '@/components/layout/SwitchRow.vue';
import { closeOverlay } from '@/stores/nav.js';
import { usePinnedScroll } from '@/composables/usePinnedScroll.js';
import { WARN, warnConfirm, isPasswordTooSimple } from '@/stores/proxyWarnings.js';

const rootEl = usePinnedScroll();

const runtimeOptions = [
  { value: 'vpn', label: 'VPN (весь трафик)' },
  { value: 'proxy', label: 'Только локальный прокси' },
];
const transportOptions = [
  { value: 'direct', label: 'Прямой' },
  { value: 'vk_turn_tcp', label: 'Через VK TURN' },
];

const form = reactive({
  allowLan: false,
  allowInsecure: false,
  ipv6: true,
  sniffingEnabled: true,
  proxyQuicEnabled: false,
  restartOnNetworkChange: false,
  runtimeMode: 'vpn',
  transportMode: 'direct',
  remoteDns: '',
  directDns: '',
  localProxyEnabled: false,
  localProxyPort: 10808,
  localProxyListenAddress: '127.0.0.1',
  localProxyAuthEnabled: true,
  localProxyUsername: '',
  localProxyPassword: '',
  httpProxyEnabled: false,
  httpProxyPort: 10809,
  httpProxyListenAddress: '127.0.0.1',
  httpProxyAuthEnabled: true,
  httpProxyUsername: '',
  httpProxyPassword: '',
});

let loaded = false;
const lastPw = { local: '', http: '' };

onMounted(async () => {
  try {
    const s = await ProfilesService.XraySettings();
    Object.assign(form, s);
    lastPw.local = form.localProxyPassword;
    lastPw.http = form.httpProxyPassword;
  } catch {
    // backend not available (pure-vite preview)
  } finally {
    loaded = true;
  }
});

// Disabling proxy auth or enabling allow-insecure weakens security, so gate them behind
// the countdown warning (mirroring the app); on cancel the switch stays where it was.
async function onAuthToggle(which, v) {
  if (!v && !(await warnConfirm(which === 'http' ? WARN.httpAuthDisable : WARN.socksAuthDisable))) return;
  if (which === 'http') form.httpProxyAuthEnabled = v;
  else form.localProxyAuthEnabled = v;
  save();
}

async function onAllowInsecure(v) {
  if (v && !(await warnConfirm(WARN.allowInsecure))) return;
  form.allowInsecure = v;
  save();
}

// A weak password warns after a short pause; on cancel the field reverts to the last value
// that was accepted, so the weak one is never persisted.
const pwTimers = {};
function onPassword(which, v) {
  const field = which === 'http' ? 'httpProxyPassword' : 'localProxyPassword';
  form[field] = v;
  if (pwTimers[which]) clearTimeout(pwTimers[which]);
  pwTimers[which] = setTimeout(async () => {
    const authOn = which === 'http' ? form.httpProxyAuthEnabled : form.localProxyAuthEnabled;
    const user = which === 'http' ? form.httpProxyUsername : form.localProxyUsername;
    if (authOn && v && isPasswordTooSimple(user, v)) {
      if (!(await warnConfirm(which === 'http' ? WARN.httpWeak : WARN.socksWeak))) {
        form[field] = lastPw[which];
        return;
      }
    }
    lastPw[which] = v;
    save();
  }, 500);
}

async function save() {
  if (!loaded) return;
  try {
    // Ports arrive from number inputs as strings; coerce so the Go int fields parse.
    const payload = { ...form, localProxyPort: Number(form.localProxyPort), httpProxyPort: Number(form.httpProxyPort) };
    const saved = await ProfilesService.SetXraySettings(payload);
    // Adopt any credentials the backend generated on enabling auth.
    if (saved) Object.assign(form, saved);
  } catch {
    // ignore persist failure
  }
}

let debounce = null;
function saveDebounced() {
  if (debounce) clearTimeout(debounce);
  debounce = setTimeout(save, 400);
}
onBeforeUnmount(() => {
  if (debounce) clearTimeout(debounce);
  Object.values(pwTimers).forEach((t) => t && clearTimeout(t));
});
</script>
