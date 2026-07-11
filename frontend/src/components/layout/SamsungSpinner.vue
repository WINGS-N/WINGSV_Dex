<template>
  <span class="dex-spin" aria-hidden="true">
    <span class="samsung-loader">
      <span class="samsung-loader-dot samsung-loader-dot-top"></span>
      <span class="samsung-loader-dot samsung-loader-dot-right"></span>
      <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
      <span class="samsung-loader-dot samsung-loader-dot-left"></span>
    </span>
  </span>
</template>

<script setup>
// Small inline variant of the standard colored Samsung loader (SamsungLoader is the
// white on-button variant; this one keeps the brand dot colors for use on cards).
</script>

<style scoped>
.dex-spin {
  position: relative;
  display: inline-block;
  width: 20px;
  height: 20px;
  flex-shrink: 0;
}
/* The dots orbit the loader box's top-left origin, so pin that origin to the box centre. */
.dex-spin :deep(.samsung-loader) {
  position: absolute;
  top: 50%;
  left: 50%;
}
</style>
