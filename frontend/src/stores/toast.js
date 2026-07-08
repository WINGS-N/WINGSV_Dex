import { reactive } from 'vue';

// Transient top pills. type: info | warn | error.
export const toasts = reactive([]);
let seq = 0;

export function showToast(message, opts = {}) {
  if (!message) return;
  const id = ++seq;
  toasts.push({ id, message, type: opts.type || 'info' });
  const duration = opts.duration || 3400;
  setTimeout(() => dismissToast(id), duration);
  return id;
}

export function dismissToast(id) {
  const i = toasts.findIndex((t) => t.id === id);
  if (i >= 0) toasts.splice(i, 1);
}
