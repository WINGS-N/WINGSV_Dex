<template>
  <span :class="classNames">
    <slot />
  </span>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  /** online | offline | info | neutral — maps to .admin-pill .is-* */
  variant: { type: String, default: 'neutral' },
});

const classNames = computed(() => {
  switch (props.variant) {
    case 'online':
      return 'admin-pill is-online';
    case 'offline':
      return 'admin-pill is-offline';
    case 'info':
      return 'admin-pill is-info';
    case 'neutral':
    default:
      return 'admin-pill';
  }
});
</script>
