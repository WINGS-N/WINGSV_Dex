<template>
  <label class="oneui-switch" :class="{ 'is-disabled': disabled }">
    <input type="checkbox" class="oneui-switch-input" :checked="modelValue" :disabled="disabled" @change="onChange" />
    <span class="oneui-switch-track" aria-hidden="true">
      <span class="oneui-switch-thumb"></span>
    </span>
  </label>
</template>

<script setup>
defineProps({
  modelValue: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'change']);
function onChange(event) {
  emit('update:modelValue', event.target.checked);
  emit('change', event.target.checked);
}
</script>
