<template>
  <button
    type="button"
    :class="classNames"
    :disabled="disabled || busy"
    :aria-label="ariaLabel"
    :title="title"
    :aria-busy="busy ? 'true' : undefined"
    @click="onClick"
  >
    <SamsungLoader v-if="busy" />
    <slot v-else />
  </button>
</template>

<script setup>
import { computed } from 'vue';
import SamsungLoader from '@/components/layout/SamsungLoader.vue';

const props = defineProps({
  /** default | danger */
  variant: { type: String, default: 'default' },
  /** small | medium */
  size: { type: String, default: 'medium' },
  ariaLabel: { type: String, default: undefined },
  title: { type: String, default: undefined },
  busy: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['click']);

const classNames = computed(() => {
  const base = props.size === 'small' ? 'icon-button' : 'admin-row-action-btn';
  return props.variant === 'danger' ? `${base} admin-row-action-danger` : base;
});

function onClick(event) {
  if (props.busy) return;
  emit('click', event);
}
</script>
