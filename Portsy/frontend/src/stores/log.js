// frontend/src/stores/log.js
// Lightweight in-app log with a ring buffer and convenience helpers.

import { writable } from "svelte/store";

/** @typedef {'info'|'warn'|'error'|'success'|'debug'} LogLevel */
/** @typedef {{ id:number; ts:number; level:LogLevel; project?:string|null; message:string; meta?:any }} LogEntry */

function createLogStore(maxEntries = 500) {
  let nextId = 1;
  const { subscribe, update, set } = writable(/** @type {LogEntry[]} */([]));

  /** Internal: push a new entry and trim buffer. */
  function push(level, message, project = null, meta) {
    /** @type {LogEntry} */
    const entry = { id: nextId++, ts: Date.now(), level, project, message, meta };
    update(arr => {
      const next = arr.length ? [...arr, entry] : [entry];
      // Trim from the left without mutating the same array instance
      return next.length > maxEntries ? next.slice(-maxEntries) : next;
    });
  }

  /** Clear all, or only a project's entries if `project` provided. */
  function clear(project) {
    if (!project) return set([]);
    update(arr => arr.filter(e => e.project !== project));
  }

  /** Last N entries (optionally by project). */
  function tail(n = 50, project) {
    if (n <= 0) return [];
    let out;
    update(arr => {
      const src = project ? arr.filter(e => e.project === project) : arr;
      out = src.slice(-n);
      return arr; // no state change
    });
    return out;
  }

  /** Change ring buffer capacity at runtime. */
  function setMaxEntries(n) {
    maxEntries = Math.max(1, Number(n) || 1);
    update(arr => (arr.length > maxEntries ? arr.slice(-maxEntries) : arr));
  }

  /** Scoped helper that pre-fills the project field. */
  function withProject(project) {
    const pre = (level) => (msg, meta) => push(level, msg, project, meta);
    return {
      info:    pre('info'),
      warn:    pre('warn'),
      error:   pre('error'),
      success: pre('success'),
      debug:   pre('debug'),
    };
  }

  /** Optional: mirror to console for dev. Call once in App.svelte if desired. */
  function attachConsoleBridge(enabled = true) {
    if (!enabled) return () => {};
    const unsub = subscribe(entries => {
      const last = entries[entries.length - 1];
      if (!last) return;
      const { level, message, project, meta } = last;
      const tag = project ? `[${project}]` : '';
      const line = `${tag} ${message}`;
      // Choose console sink based on level
      (level === 'error' ? console.error :
       level === 'warn'  ? console.warn  :
       level === 'debug' ? console.debug :
       console.log)(line, meta ?? '');
    });
    return unsub;
  }

  return {
    subscribe,
    push,
    clear,
    tail,
    setMaxEntries,
    withProject,
    attachConsoleBridge,
    // Sugar
    info:    (msg, project, meta) => push('info', msg, project, meta),
    warn:    (msg, project, meta) => push('warn', msg, project, meta),
    error:   (msg, project, meta) => push('error', msg, project, meta),
    success: (msg, project, meta) => push('success', msg, project, meta),
    debug:   (msg, project, meta) => push('debug', msg, project, meta),
  };
}

// Default singleton store (matches your original export shape)
export const logStore = createLogStore(500);
