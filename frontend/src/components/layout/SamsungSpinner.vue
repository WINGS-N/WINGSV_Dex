<template>
  <span class="dex-spin" aria-hidden="true">
    <span class="dex-spin-scale">
      <span class="samsung-loader">
        <span class="samsung-loader-dot samsung-loader-dot-top"></span>
        <span class="samsung-loader-dot samsung-loader-dot-right"></span>
        <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
        <span class="samsung-loader-dot samsung-loader-dot-left"></span>
      </span>
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
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}
/* The loader spins itself, so the scale lives on this static wrapper (not on the loader,
   whose rotate animation would override a static transform). Its origin is pinned to the
   box centre and it scales down from there. */
.dex-spin-scale {
  position: absolute;
  top: 50%;
  left: 50%;
  transform-origin: top left;
  transform: scale(0.48);
}
</style>
