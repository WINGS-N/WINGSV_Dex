<template>
  <span v-if="state === 'applying'" class="patchdot" aria-hidden="true">
    <span class="patchdot-scale">
      <span class="samsung-loader">
        <span class="samsung-loader-dot samsung-loader-dot-top"></span>
        <span class="samsung-loader-dot samsung-loader-dot-right"></span>
        <span class="samsung-loader-dot samsung-loader-dot-bottom"></span>
        <span class="samsung-loader-dot samsung-loader-dot-left"></span>
      </span>
    </span>
  </span>
  <span v-else-if="state === 'failed' || state === 'reverted_needs_restart'" class="shrink-0 text-[15px] leading-none">
    ⚠️
  </span>
</template>

<script setup>
// Per-row live-patch status: the classic Samsung four-dot loader while the relay
// applies the field, a warning glyph when it could only take effect on the next
// restart. Nothing when idle or applied.
defineProps({
  state: { type: String, default: '' },
});
</script>

<style scoped>
.patchdot {
  position: relative;
  display: inline-block;
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
/* Scale on this static wrapper, not on the loader itself - the loader has its own rotate
   animation that would override a static transform. Origin pinned to the box centre. */
.patchdot-scale {
  position: absolute;
  top: 50%;
  left: 50%;
  transform-origin: top left;
  transform: scale(0.45);
}
</style>
