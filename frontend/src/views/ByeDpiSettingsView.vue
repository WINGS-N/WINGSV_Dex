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
      <h1 class="font-sharp text-[22px] font-bold text-white">ByeDPI</h1>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
      <SamsungCard kicker="ByeDPI">
        <div class="divide-y divide-wings-divider">
          <SwitchRow
            title="Включить ByeDPI"
            subtitle="Пускать трафик Xray через локальный обход DPI"
            v-model="form.enabled"
            @update:model-value="save"
          />
          <template v-if="form.enabled">
            <OneuiInput label="Адрес" v-model="form.proxyIp" @update:model-value="saveDebounced" />
            <OneuiInput label="Порт" type="number" v-model="form.proxyPort" @update:model-value="saveDebounced" />
            <SwitchRow title="Пароль" :model-value="form.authEnabled" @update:model-value="onAuthToggle" />
            <template v-if="form.authEnabled">
              <OneuiInput label="Логин" v-model="form.username" @update:model-value="saveDebounced" />
              <OneuiInput label="Пароль" :model-value="form.password" @update:model-value="onPassword" />
            </template>
          </template>
        </div>
      </SamsungCard>

      <template v-if="form.enabled">
        <SamsungCard kicker="Обход" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <SwitchRow
              title="Ручная команда"
              subtitle="Задать аргументы ciadpi вручную"
              v-model="form.useCommandSettings"
              @update:model-value="save"
            />
            <template v-if="form.useCommandSettings">
              <OneuiInput
                label="Аргументы"
                v-model="form.command"
                placeholder="--oob 1 --auto=torst"
                @update:model-value="saveDebounced"
              />
            </template>
            <template v-else>
              <OneuiSelect
                label="Метод десинхронизации"
                v-model="form.desyncMethod"
                :options="desyncOptions"
                @update:model-value="save"
              />
              <OneuiInput
                label="Позиция разбиения"
                type="number"
                v-model="form.splitPosition"
                @update:model-value="saveDebounced"
              />
              <OneuiInput
                v-if="form.desyncMethod === 'fake'"
                label="TTL фейка"
                type="number"
                v-model="form.fakeTtl"
                @update:model-value="saveDebounced"
              />
              <OneuiInput
                label="TTL по умолчанию (0 - выкл)"
                type="number"
                v-model="form.defaultTtl"
                @update:model-value="saveDebounced"
              />
              <SwitchRow title="TCP Fast Open" v-model="form.tcpFastOpen" @update:model-value="save" />
              <SwitchRow title="Запрет резолва доменов" v-model="form.noDomain" @update:model-value="save" />
            </template>
          </div>
        </SamsungCard>

        <SamsungCard kicker="Ресурсы" class="mt-5">
          <div class="divide-y divide-wings-divider">
            <OneuiInput
              label="Лимит соединений"
              type="number"
              v-model="form.maxConnections"
              @update:model-value="saveDebounced"
            />
            <OneuiInput
              label="Размер буфера"
              type="number"
              v-model="form.bufferSize"
              @update:model-value="saveDebounced"
            />
          </div>
        </SamsungCard>
      </template>
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

const desyncOptions = [
  { value: 'oob', label: 'OOB' },
  { value: 'disoob', label: 'Disorder OOB' },
  { value: 'split', label: 'Split' },
  { value: 'disorder', label: 'Disorder' },
  { value: 'fake', label: 'Fake' },
  { value: 'auto', label: 'Auto' },
];

const form = reactive({
  enabled: false,
  proxyIp: '127.0.0.1',
  proxyPort: 1080,
  authEnabled: true,
  username: '',
  password: '',
  maxConnections: 512,
  bufferSize: 16384,
  noDomain: false,
  tcpFastOpen: false,
  defaultTtl: 0,
  desyncMethod: 'oob',
  splitPosition: 1,
  fakeTtl: 8,
  useCommandSettings: false,
  command: '',
});

let loaded = false;
let lastPw = '';

onMounted(async () => {
  try {
    Object.assign(form, await ProfilesService.ByeDPISettings());
    lastPw = form.password;
  } catch {
    // backend not available (pure-vite preview)
  } finally {
    loaded = true;
  }
});

// ByeDPI is a local SOCKS proxy, so disabling its auth gets the same security warning as
// the xray SOCKS inbound.
async function onAuthToggle(v) {
  if (!v && !(await warnConfirm(WARN.socksAuthDisable))) return;
  form.authEnabled = v;
  save();
}

let pwTimer = null;
function onPassword(v) {
  form.password = v;
  if (pwTimer) clearTimeout(pwTimer);
  pwTimer = setTimeout(async () => {
    if (form.authEnabled && v && isPasswordTooSimple(form.username, v)) {
      if (!(await warnConfirm(WARN.socksWeak))) {
        form.password = lastPw;
        return;
      }
    }
    lastPw = v;
    save();
  }, 500);
}

async function save() {
  if (!loaded) return;
  try {
    const payload = {
      ...form,
      proxyPort: Number(form.proxyPort),
      maxConnections: Number(form.maxConnections),
      bufferSize: Number(form.bufferSize),
      defaultTtl: Number(form.defaultTtl),
      splitPosition: Number(form.splitPosition),
      fakeTtl: Number(form.fakeTtl),
    };
    const saved = await ProfilesService.SetByeDPISettings(payload);
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
  if (pwTimer) clearTimeout(pwTimer);
});
</script>
