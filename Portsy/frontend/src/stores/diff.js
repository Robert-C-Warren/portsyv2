// frontend/src/stores/diffs.js
import { writable } from 'svelte/store';

const _diffs = writable(new Map());
let _wired = false;

console.log('[diffs.js] module loaded');

export const diffs = {
  subscribe: _diffs.subscribe,
  // debug helper
  debugSnapshot() {
    let snap;
    _diffs.update(m => (snap = Array.from(m.entries()), m));
    return snap;
  }
};

export function initDiffBus() {
  if (_wired) {
    console.log('[diffs.js] bus already wired');
    return;
  }
  const w = /** @type {any} */ (window);
  const on = w?.runtime?.Events?.On;

  if (!on) {
    console.warn('[diffs.js] window.runtime.Events.On is not ready yet; polling...');
    const timer = setInterval(() => {
      const on2 = w?.runtime?.Events?.On;
      if (on2) {
        clearInterval(timer);
        wire(on2);
      }
    }, 200);
    setTimeout(() => clearInterval(timer), 10000); // stop after 10s
    return;
  }

  wire(on);

  function wire(On) {
    _wired = true;
    console.log('[diffs.js] wiring event bus');
    On('project:diff', (payload) => {
      const key =
        payload.projectId ||
        payload.project ||
        payload.name ||
        payload.id ||
        payload.absolutePath ||
        payload.Path ||
        'UNKNOWN_KEY';

      const norm = {
        added:    Array.isArray(payload.added)    ? payload.added    : [],
        modified: Array.isArray(payload.modified) ? payload.modified : [],
        deleted:  Array.isArray(payload.deleted)  ? payload.deleted  : [],
      };

      console.log('[diffs.js] received project:diff', { key, counts: {
        added: norm.added.length, modified: norm.modified.length, deleted: norm.deleted.length
      }, raw: payload });

      _diffs.update(m => (m.set(key, norm), m));
    });
  }
}
