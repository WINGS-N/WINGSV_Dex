<template>
  <!--
    variant="default" — обычный radio-набор (Samsung circle + label).
    variant="pill"    — admin-pill-button row: тапаемые pill-кнопки c
                        опциональными иконкой и count-бейджем.
    Один и тот же data-shape options=[{value,label,icon?,count?,disabled?}],
    разные обёртки — чтобы не плодить два компонента под "выбери один".
  -->
  <div v-if="variant === 'pill'" class="admin-pill-row">
    <button
      v-for="opt in options"
      :key="String(opt.value)"
      type="button"
      :class="['admin-pill-button', opt.value === modelValue ? 'is-active' : '']"
      :disabled="disabled || opt.disabled"
      @click="onChange(opt.value)"
    >
      <component v-if="opt.icon" :is="opt.icon" class="button-icon" aria-hidden="true" />
      <span>{{ opt.label }}</span>
      <span v-if="opt.count != null" class="admin-pill-count">{{ opt.count }}</span>
    </button>
  </div>
  <div v-else class="seed-mode-row">
    <label v-for="opt in options" :key="String(opt.value)" class="seed-mode-option">
      <input
        type="radio"
        :name="groupName"
        :value="opt.value"
        :checked="opt.value === modelValue"
        :disabled="disabled || opt.disabled"
        @change="onChange(opt.value)"
      />
      <span>{{ opt.label }}</span>
    </label>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  modelValue: { type: [String, Number, Boolean], default: '' },
  options: { type: Array, required: true },
  name: { type: String, default: '' },
  disabled: { type: Boolean, default: false },
  /** "default" | "pill" — visual style; same options shape. */
  variant: { type: String, default: 'default' },
});
const emit = defineEmits(['update:modelValue', 'change']);

const groupName = computed(() => props.name || `oneui-radio-${Math.random().toString(36).slice(2, 8)}`);

function onChange(value) {
  emit('update:modelValue', value);
  emit('change', value);
}
</script>
