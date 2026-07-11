import { confirm } from '@/stores/confirm.js';

// The security warnings shown when a user weakens the local proxy protection. Each opens the
// countdown confirm (title "Предупреждение", amber triangle, red Продолжить after 4 s).
export const WARN = {
  socksAuthDisable: 'Вы отключаете аутентификацию SOCKS сервера. Это снизит безопасность вашего соединения',
  httpAuthDisable: 'Вы отключаете аутентификацию HTTP прокси. Это снизит безопасность вашего соединения',
  socksWeak: 'Задан слишком простой пароль SOCKS сервера. Это снизит безопасность вашего соединения',
  httpWeak: 'Задан слишком простой пароль HTTP прокси. Это снизит безопасность вашего соединения',
  allowInsecure:
    'Вы отключаете проверку TLS-сертификатов в outbound Xray. Соединение становится уязвимым к MITM-атакам',
};

// warnConfirm shows the security warning and resolves true only after the user waits out the
// countdown and presses Продолжить.
export function warnConfirm(message) {
  return confirm({
    title: 'Предупреждение',
    icon: 'warning',
    message,
    confirmText: 'Продолжить',
    cancelText: 'Отмена',
    danger: true,
    countdown: 4,
  });
}

const MIN_RECOMMENDED = 12;
const LONG_PASSWORD = 20;
const COMMON = new Set([
  'password',
  'password1',
  '123456',
  '12345678',
  '123456789',
  'qwerty',
  'qwerty123',
  'admin',
  'admin123',
  'root',
  'proxy',
  'socks',
  'wingsv',
  '111111',
  '000000',
]);

// isPasswordTooSimple mirrors the app's SocksAuthSecurity: empty, shorter than 12, equal to
// the username, a known-common value, an all-same or sequential run, or too few character
// categories all count as weak.
export function isPasswordTooSimple(username, password) {
  const p = (password || '').trim();
  if (!p) return true;
  if (p.length < MIN_RECOMMENDED) return true;
  const u = (username || '').trim();
  if (u && p.toLowerCase() === u.toLowerCase()) return true;
  if (COMMON.has(p.toLowerCase())) return true;
  if (isRepeated(p)) return true;
  if (isSequential(p)) return true;
  const categories = countCategories(p);
  if (p.length >= LONG_PASSWORD) return categories < 2;
  return categories < 3;
}

function isRepeated(value) {
  for (let i = 1; i < value.length; i++) {
    if (value[i] !== value[0]) return false;
  }
  return true;
}

function isSequential(value) {
  if (value.length < 6) return false;
  let ascending = true;
  let descending = true;
  for (let i = 1; i < value.length; i++) {
    const prev = value.charCodeAt(i - 1);
    const cur = value.charCodeAt(i);
    ascending = ascending && cur === prev + 1;
    descending = descending && cur === prev - 1;
  }
  return ascending || descending;
}

function countCategories(value) {
  let lower = false;
  let upper = false;
  let digit = false;
  let other = false;
  for (const ch of value) {
    if (ch >= 'a' && ch <= 'z') lower = true;
    else if (ch >= 'A' && ch <= 'Z') upper = true;
    else if (ch >= '0' && ch <= '9') digit = true;
    else other = true;
  }
  return [lower, upper, digit, other].filter(Boolean).length;
}
