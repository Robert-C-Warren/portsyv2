import './style.css'
import App from './App.svelte'

const app = new App({
  target: document.getElementById('app') || (() => {
    const el = document.createElement('div'); el.id = 'app'; document.body.appendChild(el); return el;
  })
})

export default app
