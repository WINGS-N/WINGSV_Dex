import { ref } from 'vue';

// A single full-screen overlay stacked over the tab UI (e.g. the VK TURN settings
// screen opened from Options). null means the tab UI is shown.
export const overlay = ref(null);

export function openOverlay(name) {
  overlay.value = name;
}

export function closeOverlay() {
  overlay.value = null;
}
