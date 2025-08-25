<script>
  import { tick, onDestroy } from 'svelte';
  import { logStore } from '../stores/log.js';

  export let selectedProject = null; // when null/empty: show nothing
  export let follow = true;          // auto-scroll to newest

  let all = [];       // all log entries from store
  let filtered = [];  // entries for the selected project
  let scroller;

  // subscribe once and keep the unsubscribe
  const unsubscribe = logStore.subscribe((entries) => {
    all = Array.isArray(entries) ? entries : [];
    updateFiltered();
  });

  // recompute whenever selectedProject or all changes
  $: updateFiltered();

  function updateFiltered() {
    filtered = selectedProject ? all.filter(e => e.project === selectedProject) : [];
    if (follow && scroller) {
      tick().then(() => {
        try { scroller.scrollTop = scroller.scrollHeight; } catch {}
      });
    }
  }

  const levelIcon = {
    info: 'ℹ️',
    warn: '⚠️',
    error: '⛔',
    success: '✅',
    debug: '🐛'
  };

  function fmt(ts) {
    const d = new Date(ts);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  onDestroy(() => {
    try { unsubscribe && unsubscribe(); } catch {}
  });
</script>

{#if selectedProject}
  <div class="log" bind:this={scroller} aria-label="Event log">
    {#if filtered.length === 0}
      <div class="empty">No events yet for <span class="project">{selectedProject}</span>.</div>
    {:else}
      {#each filtered as e (e.id)}
        <div class="row {e.level}">
          <span class="time">[{fmt(e.ts)}]</span>
          <span class="badge" title={e.level}>{levelIcon[e.level] || '•'}</span>
          <span class="msg">{e.message}</span>
        </div>
      {/each}
    {/if}
  </div>
{/if}

<style>
  .log {
    height: 220px;
    overflow: auto;
    border: 1px solid var(--border, rgba(0,0,0,.1));
    border-radius: 6px;
    padding: .25rem .5rem;
    background: var(--panel, rgba(0,0,0,.03));
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
    font-size: 12.5px;
  }

  .row {
    display: flex;
    gap: .5rem;
    align-items: baseline;
    padding: .125rem .25rem;
    border-left: 3px solid transparent;
    word-break: break-word;
    white-space: break-spaces;
  }

  .time { opacity: .65; }
  .badge { width: 1.25rem; text-align: center; flex: 0 0 auto; }
  .msg { flex: 1 1 auto; }

  .row.info    { border-left-color: #4da3ff40; }
  .row.warn    { border-left-color: #ffb02066; }
  .row.error   { border-left-color: #ff5a5f80; }
  .row.success { border-left-color: #28a74566; }
  .row.debug   { border-left-color: #9996; }

  .empty { opacity: .6; padding: .25rem; }
  .project { font-weight: 600; }
</style>
