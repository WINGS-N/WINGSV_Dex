<template>
  <div class="surface-card !p-4">
    <div class="flex items-center gap-1.5 text-wings-muted">
      <component
        :is="icon"
        v-if="icon"
        :size="15"
        :stroke-width="iconColor ? 2.6 : 2"
        :style="iconColor ? { color: iconColor } : null"
      />
      <span class="text-[13px]">{{ title }}</span>
    </div>
    <p class="mt-1.5 text-[19px] font-semibold">{{ value }}</p>
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  value: { type: [String, Number], default: '—' },
  icon: { type: [Object, Function], default: null },
  // Optional CSS colour for the icon (green downlink / blue uplink); null keeps muted.
  iconColor: { type: String, default: null },
});
</script>
