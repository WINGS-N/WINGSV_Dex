<template>
  <header class="flex items-center justify-between px-6 pb-4 pt-7">
    <h1 class="font-sharp text-[26px] font-bold leading-none tracking-tight text-white">WINGS V</h1>
    <div v-if="$slots.action" class="flex items-center gap-2">
      <slot name="action" />
    </div>
  </header>
</template>
