<template>
  <label v-if="label" class="field-label">{{ label }}</label>
  <textarea
    class="text-input"
    :rows="rows"
    :value="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readonly"
    :spellcheck="spellcheck"
    @input="onInput"
  />
  <p v-if="error" class="admin-error mt-2">{{ error }}</p>
</template>

<script setup>
defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  spellcheck: { type: Boolean, default: true },
  rows: { type: [Number, String], default: 3 },
});
const emit = defineEmits(['update:modelValue']);
function onInput(event) {
  emit('update:modelValue', event.target.value);
}
</script>
