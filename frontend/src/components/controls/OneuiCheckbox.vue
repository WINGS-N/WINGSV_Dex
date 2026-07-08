<template>
  <label class="oneui-checkbox" :class="{ 'is-disabled': disabled }">
    <input type="checkbox" class="oneui-checkbox-input" :checked="modelValue" :disabled="disabled" @change="onChange" />
    <span class="oneui-checkbox-box" aria-hidden="true">
      <Check class="oneui-checkbox-tick" v-if="modelValue" />
    </span>
    <span v-if="$slots.default" class="oneui-checkbox-label"><slot /></span>
  </label>
</template>

<script setup>
import { Check } from 'lucide-vue-next';

const props = defineProps({
  modelValue: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'change']);

function onChange(event) {
  emit('update:modelValue', event.target.checked);
  emit('change', event.target.checked);
}
</script>
