import './style.css';
import App from './App.svelte';
// Ensure we have a mount node. If #app is missing, create and append one.
function ensureMount(id = 'app') {
  let el = document.getElementById(id);
  if (!el) {
    el = document.createElement('div');
    el.id = id;
    document.body.appendChild(el);
  }
  return el;
}

const app = new App({
  target: ensureMount(),
  // hydrate: true, // uncomment if you SSR and want hydration
  // props: { /* initial props to App.svelte go here (e.g., theme: 'dark') */ },
});

export default app;
