<template>
  <div class="oneui-field">
    <label v-if="label" class="field-label">{{ label }}</label>
    <input
      :type="type"
      :class="['text-input', narrow ? 'form-input-narrow' : '']"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :inputmode="inputmode"
      :autocomplete="autocomplete"
      :min="min"
      :max="max"
      :step="step"
      @input="onInput"
    />
    <p v-if="error" class="admin-error mt-2">{{ error }}</p>
  </div>
</template>

<script setup>
defineProps({
  modelValue: { type: [String, Number], default: '' },
  type: { type: String, default: 'text' },
  label: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  error: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  readonly: { type: Boolean, default: false },
  narrow: { type: Boolean, default: false },
  inputmode: { type: String, default: undefined },
  autocomplete: { type: String, default: undefined },
  min: { type: [String, Number], default: undefined },
  max: { type: [String, Number], default: undefined },
  step: { type: [String, Number], default: undefined },
});
const emit = defineEmits(['update:modelValue']);
function onInput(event) {
  const raw = event.target.value;
  emit('update:modelValue', raw);
}
</script>

<style scoped>
/* Single-root field: label sits directly above its input with a tight gap, and
   the whole field is one flex/grid item so it sizes correctly in rows. */
.oneui-field {
  display: flex;
  flex-direction: column;
}
.oneui-field :deep(.field-label) {
  margin-bottom: 4px;
}
</style>
