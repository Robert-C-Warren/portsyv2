import { writable } from "svelte/store";

const MAX_ENTRIES = 500;
let nextId = 1;

const { subscribe, update, set } = writable([]); // array of {id, ts, level, project, message, meta}
function push(level, message, project = null, meta) {
    const entry = { id: nextId++, ts: Date.now(), level, project, message, meta };
    update(arr => {
        const next = arr.concat(entry);
        if (next.length > MAX_ENTRIES) next.splice(0, next.length - MAX_ENTRIES);
        return next;
    });
}

function clear(project) {
    if (!project) return set([]);
    update(arr => arr.filter(e => e.project !== project));
}

export const logStore = {
    subscribe,
    push,
    info: (msg, project, meta) => push('info', msg, project, meta),
    warn: (msg, project, meta) => push('warn', msg, project, meta),
    error: (msg, project, meta) => push('error', msg, project, meta),
    success: (msg, project, meta) => push('success', msg, project, meta),
    debug: (msg, project, meta) => push('debug', msg, project, meta),
    clear,
};