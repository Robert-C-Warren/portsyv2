// Live diff store with Wails event bridge.
// - Normalizes diff payloads from various backend shapes
// - Uses Svelte store + Map<string, Diff>
// - Safe event wiring with unsubscribe + optional stop()
import { writable } from 'svelte/store';

// ----- Types (JSDoc for great IDE justice) ------
/**
 * @typedef {{ added:string[]; modified:string[]; deleted:string[]; ts:number }} Diff
 */

// Internal store
const _store = writable(/** @type {Map<string, Diff>} */ (new Map()));
let _wired = false;
let _unsubs = /** @type {Array<() => void} */ ([]);

console.log('[diff] module loaded');

export const diffs = {
  subscribe: _store.subscribe,
  
  // debugSnapsho: returns [[key, diff], ...] without exposing the internal Map 
  debugSnapshot() {
    let snap;
    _store.update(m => (snap = Array.from(m.entries()), m));
    return snap;
  },

  // Get a single project's diff (undefined if missing) 
  get(key) {
    let val;
    _store.update(m => (val = m.get(normalizeKey(key)), m));
    return /** @type {Diff|undefined} */ (val);
  },
};

// ---- Public Helpers ----
// Clear all diffs.
export function clearAll() {
  _store.set(new Map());
}

// Remove one project's diff by key.
export function remove(projectKey) {
  const k = normalizeKey(projectKey);
  _store.update(m => {
    const next = new Map(m);
    next.delete(k);
    return next;
  });
}

// Manually set a diff for a project
export function set(projectKey, diff) {
  const k = normalizeKey(projectKey);
  const safe = normalizeDiff(diff);
  _store.update(m => {
    const next = new Map(m);
    next.set(k, safe);
    return next;
  })
}

// Stop the event bus and clear unsubscribers
export function stopDiffBus() {
  for (const un of _unsubs) try { un(); } catch {}
  _unsubs = [];
  _wired = false;
  console.log('[diff] bus stopped');
}

// Initialize event listeners exactly once. Safe to call multiple times.
export async function initDiffBus() {
  if (_wired) {
    console.log('[diff] bus already wired');
    return;
  }

  const On = await getEventsOn();
  if (!On) {
    console.warn('[diff] Wails EventsOn not available (yet). Will not wire bus.');
    return;
  }

  console.log('[diff] wiring event bus')
  _wired = true;

  // 1) Live project diff updates
  const offDiff = On('project:diff', (payload) => {
    const key = deriveKey(payload) || 'unknown';
    const norm = normalizeDiff(payload);
    console.log('[diff] project:diff', key, {
      added: norm.added.length, modified: norm.modified.length, deleted: norm.deleted.length
    });
    _store.update(m => {
      const next = new Map(m);
      next.set(key, { ...norm, ts: Date.now() });
      return next;
    })
  })

  // 2) After a push completes, clear that project's diff
  const offPushed = On('pushDone', (payload) => {
    const key = deriveKey(payload);
    if (!key) return;
    _store.update(m => {
      const next = new Map(m);
      next.delete(key);
      return next;
    })
  })

  _unsubs.push(offDiff || (() => {}), offPushed || (() => {}));
}

// ---- Internals ----
// Try to acquire Wails EventsOn in a robust way
async function getEventsOn() {
  // Prefer generated runtime module
  try {
    const mod = await import('../../wailsjs/runtime/runtime.js');
    if (typeof mod.EventsOn === 'function') return mod.EventsOn;
  } catch {}
  // Fallback to a global if present
  const on = globalThis?.window?.runtime?.Events?.On;
  return typeof on === 'function' ? on : null;
}

// Normalize a project key: case-insensitive; prefer id/path; fallback to name
function deriveKey(p) {
  const raw = 
  p?.projectId ??
  p?.id ??
  p?.absolutePath ??
  p?.Path ??
  p?.project ??
  p?.name ??
  '';
  return normalizeKey(raw);
}
function normalizeKey(k) {
  return String(k || '').trim().toLowerCase();
}

// Normalize many diff payload shapes into {added, modified, deleted}.
function normalizeDiff(payload) {
  // Already in target shape?
  if (payload && Array.isArray(payload.added || payload.modified || payload.deleted )) {
    return {
      added: toPathArray(payload.added),
      modified: toPathArray(payload.modified || payload.changed),
      deleted: toPathArray(payload.deleted || payload.removed)
    };
  }

  // Grouped fields with various casings
  const added = payload?.added ?? payload?.Added;
  const changed = payload?.changed ?? payload?.Changed ?? payload?.modified?? payload?.Modified;
  const removed = payload?.removed ?? payload?.Removed ?? payload?.deleted ?? payload.Deleted;
  if (added || changed || removed) {
    return {
      added: toPathArray(added),
      modified: toPathArray(changed),
      deleted: toPathArray(removed)
    };
  }

  // Flat event arrays: [ { path, status|type|action }, ... ]
  const files = payload?.files ?? payload?.Files ?? payload?.events ?? payload?.Events;
  if (Array.isArray(files)) {
    const out = { added: [], modified: [], deleted: [] };
    for (const e of files) {
      const path = e?.path ?? e?.Path ?? String(e || '');
      const verb = String(e?.status ?? e?.Status ?? e?.type ?? e?.Type ?? e?.action ?? e?.Action ?? '').toLowerCase();
      if (!path) continue;
      if (['added', 'new', 'create', 'created'].includes(verb)) out.added.push(path);
      else if (['removed', 'remove', 'delete', 'deleted'].includes(verb)) out.deleted.push(path);
      else out.modified.push(path); // default bucket
    }
    return out;
  }

  // String array? treat as "modified"
  if (Array.isArray(payload) && typeof payload[0] === 'string') {
    return { added: [], modified: payload.slice(), deleted: [] };
  }

  // Unknown -> empty
  return { added: [], modified: [], deleted: [] };
}

// Convert many value shapes to a string[] of paths.
function toPathArray(v) {
  if (!v) return [];
  if (Array.isArray(v)) {
    return v.map(x => (typeof x === 'string' ? x : (x?.path ?? x?.Path ?? ''))).filter(Boolean);
  }
  // Single value
  if (typeof v === 'string') return [v];
  const p = v?.path ?? v?.Path;
  return p ? [p] : [];
}