import { createApp } from 'vue';
import App from './App.vue';
import './stores/theme.js';
import './styles.css';

createApp(App).mount('#app');

// Reveal the app and fade out the pre-mount boot loader once Vue has taken over.
const bootLoader = document.getElementById('boot-loader');
if (bootLoader) {
  bootLoader.classList.add('is-hidden');
  bootLoader.addEventListener('transitionend', () => bootLoader.remove(), { once: true });
}
