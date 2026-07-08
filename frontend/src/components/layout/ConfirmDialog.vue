<template>
  <SamsungModal :model-value="confirmState.open" :title="confirmState.title" @update:model-value="settleConfirm(false)">
    <p class="body-copy confirm-message">{{ confirmState.message }}</p>
    <template #actions>
      <SamsungButton :variant="confirmState.danger ? 'danger' : 'primary'" @click="settleConfirm(true)">
        {{ confirmState.confirmText }}
      </SamsungButton>
      <SamsungButton v-if="confirmState.cancelText" variant="secondary" @click="settleConfirm(false)">
        {{ confirmState.cancelText }}
      </SamsungButton>
    </template>
  </SamsungModal>
</template>

<script setup>
import SamsungModal from '@/components/layout/SamsungModal.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';
import { confirmState, settleConfirm } from '@/stores/confirm.js';
</script>

<style scoped>
.confirm-message {
  white-space: pre-line;
}
</style>
