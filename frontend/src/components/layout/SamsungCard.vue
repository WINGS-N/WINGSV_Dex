<template>
  <section :class="['surface-card', $attrs.class]">
    <header
      v-if="kicker || title || $slots.title || $slots.actions"
      :class="['admin-card-header', collapsible ? 'admin-card-header-collapsible' : '']"
      @click="collapsible ? (open = !open) : null"
    >
      <div>
        <div v-if="kicker" class="text-[12px] font-bold uppercase tracking-[0.14em] text-wings-kicker">
          {{ kicker }}
        </div>
        <slot name="title">
          <h2 v-if="title" :class="['admin-card-title', collapsible ? 'admin-card-title-collapsible' : '']">
            <ChevronDown
              v-if="collapsible"
              :class="['card-chevron', open ? 'card-chevron-open' : '']"
              aria-hidden="true"
            />
            <span>{{ title }}</span>
          </h2>
        </slot>
        <p v-if="subtitle && (!collapsible || open)" class="body-copy">{{ subtitle }}</p>
        <slot name="subtitle" />
      </div>
      <div v-if="$slots.actions" class="flex items-center gap-2" @click.stop>
        <slot name="actions" />
      </div>
    </header>
    <slot v-if="!collapsible || open" />
  </section>
</template>

<script setup>
import { ref } from 'vue';
import { ChevronDown } from 'lucide-vue-next';

const props = defineProps({
  kicker: { type: String, default: '' },
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  collapsible: { type: Boolean, default: false },
  defaultCollapsed: { type: Boolean, default: false },
});
defineOptions({ inheritAttrs: false });

const open = ref(!props.defaultCollapsed);
</script>

<style scoped>
.admin-card-header-collapsible {
  cursor: pointer;
  user-select: none;
}
.admin-card-title-collapsible {
  display: inline-flex;
  align-items: center;
  gap: 0.4em;
}
.card-chevron {
  width: 1.1em;
  height: 1.1em;
  flex-shrink: 0;
  transition: transform 0.2s ease;
  transform: rotate(-90deg);
}
.card-chevron-open {
  transform: rotate(0deg);
}
</style>
