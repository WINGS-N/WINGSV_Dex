<template>
  <div class="flex flex-col gap-2">
    <div
      v-for="r in rows"
      :key="r.id"
      class="flex items-center gap-3 rounded-2xl border px-4 py-3 transition-all duration-200"
      :class="[
        glass ? 'border-white/15 bg-white/10' : 'border-wings-divider bg-wings-surface',
        r.fading ? '-translate-y-2.5 opacity-0' : 'opacity-100',
      ]"
    >
      <div class="min-w-0 flex-1">
        <div class="truncate text-[17px]" :class="glass ? 'text-white' : 'text-wings-text'">{{ r.title }}</div>
        <div class="mt-0.5 truncate text-sm" :class="glass ? 'text-white/70' : 'text-wings-muted'">
          {{ r.metric || r.address }}
        </div>
      </div>
      <SamsungSpinner v-if="r.status === 'checking'" class="shrink-0" />
      <span
        v-else-if="r.status === 'err'"
        class="shrink-0 rounded-full bg-red-500/20 px-3 py-1 text-[13px] font-bold text-red-400"
      >
        ERR
      </span>
      <span
        v-else-if="r.latencyMs >= 0"
        class="shrink-0 rounded-full px-2.5 py-1 text-[13px] font-semibold"
        :class="badge(r.latencyMs)"
      >
        {{ r.latencyMs }} ms
      </span>
    </div>
  </div>
</template>

<script setup>
import SamsungSpinner from '@/components/layout/SamsungSpinner.vue';

defineProps({ rows: { type: Array, default: () => [] }, glass: { type: Boolean, default: false } });

function badge(ms) {
  if (ms <= 150) return 'bg-emerald-500/20 text-emerald-400';
  if (ms <= 350) return 'bg-amber-500/20 text-amber-400';
  return 'bg-red-500/20 text-red-400';
}
</script>
