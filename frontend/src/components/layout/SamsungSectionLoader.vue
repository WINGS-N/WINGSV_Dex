<template>
  <div class="loading-layout">
    <span class="loading-dots">
      <span class="samsung-loader" aria-hidden="true">
        <span class="samsung-loader-dot samsung-loader-dot-top"></span>
        <span class="samsung-loader-dot samsung-loader-dot-right"></span>
        <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
        <span class="samsung-loader-dot samsung-loader-dot-left"></span>
      </span>
    </span>
    <p v-if="label" class="admin-muted mt-3">{{ label }}</p>
  </div>
</template>

<script setup>
defineProps({
  /** Optional helper text shown beneath the loader. */
  label: { type: String, default: '' },
});
</script>

<style scoped>
.loading-layout {
  display: flex;
  min-height: 220px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

/* The four dots orbit the loader box's top-left origin, so centring the 11.25px box
   leaves the cluster reading shifted up-left. Give it a fixed box and pin the loader
   origin to that box's centre so the visible cluster is what gets centred. */
.loading-dots {
  position: relative;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
}

.loading-dots :deep(.samsung-loader) {
  position: absolute;
  top: 50%;
  left: 50%;
}
</style>
