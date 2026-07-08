<template>
  <component
    :is="tag"
    :type="isButton ? type : undefined"
    :class="classNames"
    :disabled="isButton ? disabled || busy : undefined"
    :aria-busy="busy ? 'true' : undefined"
    :to="to"
    :href="href"
    @click="onClick"
  >
    <SamsungLoader v-if="busy" />
    <slot v-else name="icon" />
    <span v-if="$slots.default || label">
      <slot>{{ label }}</slot>
    </span>
  </component>
</template>

<script setup>
import { computed } from 'vue';
import SamsungLoader from '@/components/layout/SamsungLoader.vue';

const props = defineProps({
  /** primary | secondary | ghost | text | danger */
  variant: { type: String, default: 'primary' },
  label: { type: String, default: '' },
  busy: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  /** native button type — submit / button / reset */
  type: { type: String, default: 'button' },
  /** if set — render as <router-link to=...> */
  to: { type: [String, Object], default: undefined },
  /** if set — render as <a href=...> */
  href: { type: String, default: undefined },
});
const emit = defineEmits(['click']);

const tag = computed(() => {
  if (props.to) return 'router-link';
  if (props.href) return 'a';
  return 'button';
});
const isButton = computed(() => tag.value === 'button');

const classNames = computed(() => {
  switch (props.variant) {
    case 'secondary':
      return 'button-secondary';
    case 'ghost':
      return 'button-ghost';
    case 'text':
      return 'button-text';
    case 'danger':
      return 'button-primary admin-row-action-danger';
    case 'primary':
    default:
      return 'button-primary';
  }
});

function onClick(event) {
  if (props.busy) {
    event.preventDefault?.();
    event.stopPropagation?.();
    return;
  }
  emit('click', event);
}
</script>
