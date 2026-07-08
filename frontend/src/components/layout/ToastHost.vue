<template>
  <div class="pointer-events-none fixed inset-x-0 top-3 z-[9998] flex flex-col items-center gap-2 px-4">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="pointer-events-auto max-w-full rounded-full px-4 py-2 text-center text-sm font-medium shadow-[0_10px_28px_rgba(0,0,0,0.45)]"
        :class="toastClass(t.type)"
        @click="dismissToast(t.id)"
      >
        {{ t.message }}
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup>
import { toasts, dismissToast } from '@/stores/toast.js';

function toastClass(type) {
  if (type === 'warn') return 'bg-[#F2A93B] text-[#10131A]';
  if (type === 'error') return 'bg-wings-danger text-white';
  return 'border border-wings-border bg-wings-card text-wings-text';
}
</script>

<style scoped>
/* Slide in from the top, exit upward. */
.toast-enter-active,
.toast-leave-active {
  transition:
    opacity 0.28s ease,
    transform 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(-18px);
}
.toast-move {
  transition: transform 0.28s cubic-bezier(0.4, 0, 0.2, 1);
}
</style>
