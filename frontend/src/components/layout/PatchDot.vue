<template>
  <span v-if="state === 'applying'" class="patchdot" aria-hidden="true">
    <span class="samsung-loader">
      <span class="samsung-loader-dot samsung-loader-dot-top"></span>
      <span class="samsung-loader-dot samsung-loader-dot-right"></span>
      <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
      <span class="samsung-loader-dot samsung-loader-dot-left"></span>
    </span>
  </span>
  <span v-else-if="state === 'failed' || state === 'reverted_needs_restart'" class="shrink-0 text-[15px] leading-none">
    ⚠️
  </span>
</template>

<script setup>
// Per-row live-patch status: the classic Samsung four-dot loader while the relay
// applies the field, a warning glyph when it could only take effect on the next
// restart. Nothing when idle or applied. Mirrors the app's in-row SESL loader.
defineProps({
  state: { type: String, default: '' },
});
</script>

<style scoped>
/* The four dots orbit the loader's top-left origin, so pin the loader to the centre
   of a small fixed box (same trick as SamsungSectionLoader) to centre the cluster. */
.patchdot {
  position: relative;
  display: inline-block;
  width: 22px;
  height: 22px;
  flex-shrink: 0;
}
.patchdot :deep(.samsung-loader) {
  position: absolute;
  top: 50%;
  left: 50%;
}
</style>
