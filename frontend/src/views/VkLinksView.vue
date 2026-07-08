<template>
  <div ref="rootEl" class="flex min-h-0 flex-1 flex-col overflow-hidden">
    <header class="flex shrink-0 items-center gap-2 px-3 pb-3 pt-6">
      <button
        type="button"
        class="rounded-full p-1.5 text-wings-mutedStrong hover:text-wings-text"
        aria-label="Назад"
        @click="$emit('close')"
      >
        <ChevronLeft :size="24" />
      </button>
      <h1 class="font-sharp text-[22px] font-bold text-white">VK-ссылки</h1>
    </header>

    <div class="min-h-0 flex-1 overflow-y-auto px-4 pb-8">
      <SamsungCard kicker="Пул" subtitle="Ссылки на VK-звонки; из них реле добывает TURN-креды. Первая — основная.">
        <ul v-if="client.vkLinks.length" class="space-y-1.5 py-3">
          <li
            v-for="(l, i) in client.vkLinks"
            :key="i"
            class="flex items-center gap-2 rounded-xl bg-white/[0.03] px-3 py-2.5"
          >
            <span class="min-w-0 flex-1 truncate text-sm text-wings-text">{{ l }}</span>
            <span
              v-if="i === 0"
              class="shrink-0 rounded-full bg-wings-accent/15 px-2 py-0.5 text-[11px] font-semibold text-wings-accent"
            >
              основная
            </span>
            <button
              type="button"
              class="shrink-0 rounded-full p-1 text-wings-mutedStrong hover:text-wings-danger"
              aria-label="Удалить ссылку"
              @click="removeLink(i)"
            >
              <Trash2 :size="16" />
            </button>
          </li>
        </ul>
        <p v-else class="py-3 text-sm text-wings-muted">Ссылок пока нет.</p>
        <div class="flex items-end gap-2 pb-2">
          <div class="flex-1">
            <OneuiInput v-model="newLink" placeholder="https://vk.com/call/join/..." :spellcheck="false" />
          </div>
          <SamsungButton variant="secondary" :disabled="!newLink.trim()" @click="addLink">Добавить</SamsungButton>
        </div>
      </SamsungCard>

      <SamsungCard
        kicker="Резерв"
        subtitle="Запасная ссылка; задействуется, когда все ссылки пула в cooldown."
        class="mt-5"
      >
        <div class="py-3">
          <OneuiInput v-model="client.vkLinkSecondary" label="Резервная VK-ссылка" :spellcheck="false" />
        </div>
      </SamsungCard>
    </div>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { ChevronLeft, Trash2 } from 'lucide-vue-next';
import OneuiInput from '@/components/controls/OneuiInput.vue';
import SamsungCard from '@/components/layout/SamsungCard.vue';
import SamsungButton from '@/components/layout/SamsungButton.vue';

const props = defineProps({
  // The reactive global client settings; mutated in place so the parent's autosave
  // watch picks up link add/remove/secondary edits.
  client: { type: Object, required: true },
});
defineEmits(['close']);

const newLink = ref('');

function addLink() {
  const v = newLink.value.trim();
  if (!v) return;
  if (!props.client.vkLinks.includes(v)) props.client.vkLinks.push(v);
  newLink.value = '';
}
function removeLink(i) {
  props.client.vkLinks.splice(i, 1);
}

// Same WebKitGTK focus-scroll guard as the settings screen: pin stray ancestor
// scrolls back to 0 so focusing an input never displaces the page off-screen.
const rootEl = ref(null);
function pinAncestors() {
  let el = rootEl.value;
  while (el) {
    if (el.scrollTop) el.scrollTop = 0;
    el = el.parentElement;
  }
  const se = document.scrollingElement;
  if (se && se.scrollTop) se.scrollTop = 0;
}
onMounted(() => window.addEventListener('scroll', pinAncestors, true));
onBeforeUnmount(() => window.removeEventListener('scroll', pinAncestors, true));
</script>
