import { ref, watchEffect } from 'vue';

const STORAGE_KEY = 'wings.admin.theme';
const VALID = ['dark', 'light'];

function readStored() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (VALID.includes(raw)) return raw;
  } catch {}
  return 'dark';
}

export const currentTheme = ref(readStored());

export function setTheme(name) {
  if (!VALID.includes(name)) return;
  currentTheme.value = name;
}

watchEffect(() => {
  const theme = currentTheme.value;
  const body = typeof document !== 'undefined' ? document.body : null;
  if (body) {
    body.classList.remove('theme-light', 'theme-dark');
    body.classList.add(theme === 'light' ? 'theme-light' : 'theme-dark');
  }
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {}
});
