<template>
  <div class="flex items-center justify-between gap-3 py-3.5">
    <div class="min-w-0">
      <p class="text-[17px]">{{ title }}</p>
      <p v-if="subtitle" class="mt-0.5 text-sm text-wings-muted">{{ subtitle }}</p>
    </div>
    <OneuiSwitch :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)" />
  </div>
</template>

<script setup>
import OneuiSwitch from '@/components/controls/OneuiSwitch.vue';

defineProps({
  modelValue: { type: Boolean, default: false },
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
});
defineEmits(['update:modelValue']);
</script>
