<template>
  <div ref="root" :class="label ? 'block w-full' : 'oneui-select-wrapper'">
    <button
      ref="trigger"
      type="button"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :class="
        label
          ? 'flex w-full items-center justify-between gap-3 py-3.5 text-left disabled:opacity-50'
          : 'oneui-select text-left'
      "
      @click="toggle"
    >
      <template v-if="label">
        <span class="text-[17px] text-wings-text">{{ label }}</span>
        <span class="flex min-w-0 items-center gap-1.5 text-wings-muted">
          <span class="truncate text-[15px]">{{ selectedLabel }}</span>
          <ChevronDown
            :size="18"
            class="shrink-0 transition-transform"
            :class="{ 'rotate-180': open }"
            aria-hidden="true"
          />
        </span>
      </template>
      <span v-else class="block truncate">{{ selectedLabel }}</span>
    </button>
    <ChevronDown v-if="!label" class="oneui-select-chevron" :class="{ 'rotate-180': open }" aria-hidden="true" />

    <Teleport to="body">
      <ul
        v-if="open"
        ref="panel"
        role="listbox"
        :style="panelStyle"
        class="max-h-[280px] overflow-y-auto rounded-2xl border border-wings-border bg-wings-card py-1 shadow-[0_18px_42px_rgba(0,0,0,0.45)]"
      >
        <li
          v-for="opt in options"
          :key="opt.value"
          role="option"
          :aria-selected="opt.value === modelValue"
          class="flex cursor-pointer items-center justify-between gap-3 px-4 py-2.5 text-[14px] transition-colors hover:bg-white/[0.06]"
          :class="opt.value === modelValue ? 'text-wings-accent' : 'text-wings-text'"
          @click="select(opt.value)"
        >
          <span class="truncate">{{ opt.label }}</span>
          <Check v-if="opt.value === modelValue" :size="18" class="shrink-0 text-wings-accent" />
        </li>
      </ul>
    </Teleport>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref } from 'vue';
import { Check, ChevronDown } from 'lucide-vue-next';

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  options: { type: Array, required: true },
  disabled: { type: Boolean, default: false },
  /** When set, render as a full-width row (title left, value + chevron right) that
   *  is itself the dropdown opener, instead of the compact pill trigger. */
  label: { type: String, default: '' },
});
const emit = defineEmits(['update:modelValue', 'change']);

const root = ref(null);
const trigger = ref(null);
const panel = ref(null);
const open = ref(false);
const panelStyle = ref({});

const selectedLabel = computed(() => {
  const found = props.options.find((o) => o.value === props.modelValue);
  return found ? found.label : '';
});

// Position the teleported panel under (or, if short on space, above) the trigger.
function positionPanel() {
  const r = trigger.value.getBoundingClientRect();
  const est = Math.min(props.options.length, 6) * 42 + 10;
  const openAbove = r.bottom + 4 + est > window.innerHeight && r.top - est - 4 > 0;
  panelStyle.value = {
    position: 'fixed',
    left: `${Math.round(r.left)}px`,
    width: `${Math.round(r.width)}px`,
    top: openAbove ? `${Math.round(r.top - 4)}px` : `${Math.round(r.bottom + 4)}px`,
    transform: openAbove ? 'translateY(-100%)' : 'none',
    zIndex: 1000,
  };
}

function toggle() {
  if (props.disabled) return;
  if (open.value) {
    close();
    return;
  }
  positionPanel();
  open.value = true;
  nextTick(() => {
    document.addEventListener('click', onDocClick);
    document.addEventListener('keydown', onKey);
    window.addEventListener('scroll', close, true);
    window.addEventListener('resize', close);
  });
}

function close() {
  if (!open.value) return;
  open.value = false;
  document.removeEventListener('click', onDocClick);
  document.removeEventListener('keydown', onKey);
  window.removeEventListener('scroll', close, true);
  window.removeEventListener('resize', close);
}

function select(value) {
  emit('update:modelValue', value);
  emit('change', value);
  close();
}

function onDocClick(e) {
  const t = e.target;
  if (root.value?.contains(t) || panel.value?.contains(t)) return;
  close();
}

function onKey(e) {
  if (e.key === 'Escape') close();
}

onBeforeUnmount(close);
</script>
