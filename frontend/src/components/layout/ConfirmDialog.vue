<template>
  <SamsungModal :model-value="confirmState.open" :title="confirmState.title" @update:model-value="settleConfirm(false)">
    <div v-if="confirmState.icon === 'warning'" class="flex flex-col items-center gap-4 py-2 text-center">
      <AlertTriangle :size="56" class="text-amber-400" />
      <p class="text-[17px] font-bold leading-snug text-wings-text">{{ confirmState.message }}</p>
      <p class="text-sm text-wings-muted">Вы уверены, что хотите продолжить?</p>
    </div>
    <p v-else class="body-copy confirm-message">{{ confirmState.message }}</p>

    <p v-if="remaining > 0" class="mt-3 text-center text-sm text-wings-muted">
      Продолжить можно через {{ remaining }} с
    </p>

    <template #actions>
      <SamsungButton
        :variant="confirmState.danger ? 'danger' : 'primary'"
        :disabled="remaining > 0"
        @click="settleConfirm(true)"
      >
        {{ confirmState.confirmText }}
      </SamsungButton>
      <SamsungButton v-if="confirmState.cancelText" variant="secondary" @click="settleConfirm(false)">
        {{ confirmState.cancelText }}
      </SamsungButton>
    </template>
  </SamsungModal>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue';
import { AlertTriangle } from 'lucide-vue-next';
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import { confirmState, settleConfirm } from '@/stores/confirm.js';

// Counts down while a countdown dialog is open, keeping the confirm button disabled until it
// reaches zero (then the danger button shows its characteristic red).
const remaining = ref(0);
let timer = null;

function stop() {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
}

watch(
  () => confirmState.open,
  (open) => {
    stop();
    if (open && confirmState.countdown > 0) {
      remaining.value = confirmState.countdown;
      timer = setInterval(() => {
        remaining.value -= 1;
        if (remaining.value <= 0) stop();
      }, 1000);
    } else {
      remaining.value = 0;
    }
  },
);

onBeforeUnmount(stop);
</script>

<style scoped>
.confirm-message {
  white-space: pre-line;
}
</style>
