// Tries generated ESM first, falls back to window.go.main.App at runtime
import * as Main from '../../wailsjs/go/main/App';

const win = () => globalThis.window?.go?.main?.App || null;
const pick = (...names) => names.map(n => Main[n] || win()?.[n]).find(fn => typeof fn === 'function');
const call = async (name, ...args) => {
  const fn = pick(name);
  if (!fn) throw new Error(`Missing backend export: ${name}`);
  return fn(...args);
};

// PUSH
export const listPushableProjects = () => call('ListPushableProjects');
export const getDiffForProject     = (name) => call('GetDiffForProject', name);
export const pushProject           = (name, msg) => {
  const fn = pick('PushProjectWithMessage','PushProject','Push');
  if (!fn) throw new Error('Push export missing');
  try { return fn(name, msg); } catch { return fn({ projectName:name, message:msg }); }
};

// PULL
export const listRemoteProjects    = () => call('ListRemoteProjects');
export const getPullStatus         = (name) => (pick('GetPullStatus') ? call('GetPullStatus', name) : { localNewer:false });
export const getCommitHistory      = (name, limit=5) => (pick('GetCommitHistory') ? call('GetCommitHistory', name, limit) : []);
export const pullProject           = (name, { allowDelete=false, commitId='' }={}) => {
  const fn = pick('PullProject','PullHead');
  if (!fn) throw new Error('Pull export missing');
  try { return fn(name, commitId, allowDelete); } catch {}
  try { return fn({ projectName:name, commitId, allowDelete }); } catch {}
  if (fn.name === 'PullHead') return fn(name, allowDelete);
  throw new Error('Pull signature mismatch');
};

// (Optional) Wails runtime events
export function onEvent(eventName, handler) {
  try {
    const { EventsOn } = require('../../wailsjs/runtime/runtime.js');
    return EventsOn(eventName, handler);
  } catch { /* ignore in dev without generator */ }
}
