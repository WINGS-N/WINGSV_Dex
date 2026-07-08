<template>
  <Teleport to="body">
    <div v-if="modelValue" class="admin-modal-backdrop" role="dialog" aria-modal="true" @click.self="onBackdropClick">
      <div class="admin-modal surface-card">
        <h2 v-if="title" class="admin-card-title">{{ title }}</h2>
        <slot />
        <div v-if="$slots.actions" class="actions-row mt-4">
          <slot name="actions" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup>
const props = defineProps({
  modelValue: { type: Boolean, required: true },
  title: { type: String, default: '' },
  /** When true, click on the backdrop does NOT close the modal — useful while
   *  a long-running action (deletion / apply) is in flight. */
  busy: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'close']);

function onBackdropClick() {
  if (props.busy) return;
  emit('update:modelValue', false);
  emit('close');
}
</script>
