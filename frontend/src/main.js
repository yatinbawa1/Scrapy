import './style.css'
import App from './App.svelte'
import { mount } from 'svelte'

console.log('[main.js] Starting mount of App component...');
const target = document.getElementById('app');
console.log('[main.js] Target element:', target);
const app = mount(App, {
  target: target,
});
console.log('[main.js] Mount complete, app:', app);

export default app
