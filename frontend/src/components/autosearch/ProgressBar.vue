<template>
  <div
    class="rounded-2xl border px-4 py-3"
    :class="glass ? 'border-white/15 bg-white/10' : 'border-wings-divider bg-wings-surface'"
  >
    <div v-if="value !== undefined" class="mb-2 h-1.5 w-full overflow-hidden rounded-full bg-white/10">
      <div class="h-full rounded-full bg-wings-accent transition-all duration-300" :style="{ width: pct + '%' }" />
    </div>
    <p class="text-center text-[15px]" :class="glass ? 'text-white/85' : 'text-wings-muted'">{{ label }}</p>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  value: { type: Number, default: undefined },
  label: { type: String, default: '' },
  glass: { type: Boolean, default: false },
});
const pct = computed(() => Math.min(100, Math.max(0, Math.round((props.value || 0) * 100))));
</script>
