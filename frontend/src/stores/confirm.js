import { reactive } from 'vue';

// A single app-wide confirmation dialog. confirm(options) returns a Promise that
// resolves true when the user confirms and false when they cancel/dismiss, so a
// caller can `if (!(await confirm({...}))) return;` in place of window.confirm.
export const confirmState = reactive({
  open: false,
  title: 'Подтверждение',
  message: '',
  confirmText: 'Подтвердить',
  cancelText: 'Отмена',
  danger: false,
  resolve: null,
});

export function confirm(options = {}) {
  return new Promise((resolve) => {
    confirmState.title = options.title || 'Подтверждение';
    confirmState.message = options.message || '';
    confirmState.confirmText = options.confirmText || 'Подтвердить';
    // Pass cancelText: '' for a single-button (acknowledge-only) dialog.
    confirmState.cancelText = options.cancelText === undefined ? 'Отмена' : options.cancelText;
    confirmState.danger = !!options.danger;
    confirmState.resolve = resolve;
    confirmState.open = true;
  });
}

export function settleConfirm(value) {
  if (!confirmState.open) return;
  confirmState.open = false;
  const resolve = confirmState.resolve;
  confirmState.resolve = null;
  if (resolve) resolve(value);
}
