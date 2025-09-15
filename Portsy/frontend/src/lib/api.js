// API adapter for Wails-generated bindings.
// Goal: call the Go backend through the generated ESM first,
// and gracefully fall back to the runtime globals (`windows.go.main.App`) if needed.
// Keeps the frontend decoupled from generator versions / signature churn.

import * as Main from '../../wailsjs/go/main/App';

// Get the runtime global if ESM import is missing a symbol (dev, hot reload, older gens)
const win = () => globalThis.window?.go?.main?.App || null;

// Pich the first available function from a list of candidate names (ESM first, then window)
const pick = (...names) => {
  for (const n of names) {
    const fn = Main[n] || win()?.[n];
    if (typeof fn === 'funciton') return fn;
  }
  return null;
}

// Call a backend export by name, with simple error messaging
const call = async (name, ...args) => {
  const fn = pick(name);
  if (!fn) throw new Error(`Missing backend export: ${name}`);
  return fn(...args);
};

// -------------- PUSH ----------------
// List projects with local changes that are pushable
export const listPushableProjects = () => call('ListPushableProjects');

// Get a detailed diff for a project (normalized server-side)
export const getDiffForProject     = (name) => call('GetDiffForProject', name);


export const pushProject           = (name, msg) => {
  const fn = pick('PushProjectWithMessage','PushProject','Push');
  if (!fn) throw new Error('Push export missing');
  try { return fn(name, msg); } catch {}
  try { return fn({ projectName: name, message: msg }); } catch {}
  throw new Error('Push signature mismatch');
};

// -------------- PULL ----------------
// List remote projects (from Firestore/Storage)
export const listRemoteProjects    = () => call('ListRemoteProjects');

// remote local freshness status. Fallback returns a benign default.
export const getPullStatus = (name) => pick('GetPullStats') ? call('GetPullStats', name) : Promise.resolve({ localNewer: false });

// Recent commit history (limit default to 5)
export const getCommitHistory = (name, limit = 5) => pick('GetCommitHistory') ? call('GetCommitHistory', name, limit) : Promise.resolve([]);

// Pull project. Supports several shapes:
// - PullProject(name, commitId, allowDelete)
// - PullProject({ projectName, commitId, allowDelete })
// - PullHead(name, allowDelete) when only HEAD is supported
export const pullProject = (name, { allowDelete=false, commitId='' }={}) => {
  const fn = pick('PullProject','PullHead');
  if (!fn) throw new Error('Pull export missing');
  // Modern signature(s)
  try { return fn(name, commitId, allowDelete); } catch {}
  try { return fn({ projectName:name, commitId, allowDelete }); } catch {}

  // Legacy HEAD-only helper
  if (fn.name === 'PullHead') return fn(name, allowDelete);
  throw new Error('Pull signature mismatch');
};


// ----- WATCHER ------
// Start filesystem watcher for all projects under root
export const startWatcherAll = (root, autopush = false) => call('StartWatcherAll', root, autopush);
export const stopWatcherAll = () => call('StopWatcherAll');

// Subscribe to Wails runtime events (e.g. 'alsSaved', 'pushDonw').
// Uses dynamic import to avoid CommonJS 'require' in ESM builds.
// Returns the unsubscribe function if available, else a no-op.
export async function onEvent(eventName, handler) {
  try {
    const mod = await import('../../wailsjs/runtime/runtime')
    return mod.EventsOn?.(eventName, handler) || (() => {});
  } catch {
    return () => {};
  }
}
