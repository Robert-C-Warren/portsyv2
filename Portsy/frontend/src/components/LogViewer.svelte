<script>
  // LogViewer - shows per-project log entries from a central store
  // Features:
  // - Filters by SelectedProject
  // - Auto-Scrolls to bottom while `follow` is true
  // - Pauses follow when the user scrolls up, with a one-click "Jump to latest"
  import { tick, onDestroy } from 'svelte';
  import { logStore } from '../stores/log.js';

  // Props
  export let selectedProject = null; // when null/empty: show nothing
  export let follow = true;          // auto-scroll to newest

  // Local State
  let all = [];       // all log entries from store
  let filtered = [];  // entries for the selected project
  let scroller;       // <div> node ref (scroll container)
  let showJump = false; // show "Jump to latest" CTA when user scrolls up

  // subscribe once and keep the unsubscribe
  const unsubscribe = logStore.subscribe((entries) => {
    all = Array.isArray(entries) ? entries : [];
    updateFiltered();
  });

  // recompute whenever dependencies change
  $: updateFiltered();

  function updateFiltered() {
    // Filter by project; tolerate different shapes
    filtered = selectedProject ? all.filter(e => (e.project ?? e.Project) === selectedProject) : [];
    
    // if following, scroll to bottom on the next microtask
    if (follow && scroller) {
      tick().then(() => {
        try { scroller.scrollTop = scroller.scrollHeight; } catch {}
        showJump = false; // at the bottom already
      })
    }
  }

  // Pause follow when user scrolls up; resume when at bottom
  function onScroll() {
    if (!scroller) return;
    const atBottom = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 4;
    if (!atBottom) {
      // User scrolled up -> pause follow and offer a jump
      if (follow) follow = false;
      showJump = true;
    } else {
      // At bottom again -> we can hide the CTA
      showJump = false;
    }
  }

  function jumpToLatest() {
    if (!scroller) return;
    try {scroller.scrollTop = scroller.scrollHeight; } catch {}
    follow = true;
    showJump = false;
  }

  // Emoji per level
  const levelIcon = {
    info: 'ℹ️',
    warn: '⚠️',
    error: '⛔',
    success: '✅',
    debug: '🐛'
  };

  function fmt(ts) {
    const d = ts ? new Date(ts) : null;
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  onDestroy(() => {
    try { unsubscribe && unsubscribe(); } catch {}
  });

  // Generate a stable key if e.id is missing: time + message hash + index
  function keyFor(e, i) {
    return e.id ?? `${e.ts ?? ''}:${(e.message ?? '').slice(0,24)}:${i}`;
  }
</script>

{#if selectedProject}
  <div class="log-shell">
    <!-- Top controls: follow toggle + Jump button (appears when user scrolls up) -->
    <div class="log-controls">
      <label class="row" style="gap:.5rem; align-items:center;">
        <input type="checkbox" bind:checked={follow} aria-label="Auto-scroll to latest" />
        <span class="muted">Follow</span>
      </label>

      {#if showJump}
        <button class="btn jump" on:click={jumpToLatest} title="Scroll to latest entry">Jump to latest</button>
      {/if}
    </div>

    <div
      class="log"
      bind:this={scroller}
      on:scroll={onScroll}
      aria-label="Event log"
      role="log"
      aria-live="polite"
    >
      {#if filtered.length === 0}
        <div class="empty">
          No events yet for <span class="project">{selectedProject}</span>.
        </div>
      {:else}
        {#each filtered as e, i (keyFor(e, i))}
          <div class="row {e.level}">
            <span class="time">[{fmt(e.ts)}]</span>
            <span class="badge" title={e.level}>{levelIcon[e.level] || '•'}</span>
            <span class="msg">{e.message}</span>
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}

<style>
  .log-shell { display: flex; flex-direction: column; gap: .25rem; }

  .log-controls {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }
  .muted { opacity: .7; }

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

  .time  { opacity: .65; }
  .badge { width: 1.25rem; text-align: center; flex: 0 0 auto; }
  .msg   { flex: 1 1 auto; }

  .row.info    { border-left-color: #4da3ff40; }
  .row.warn    { border-left-color: #ffb02066; }
  .row.error   { border-left-color: #ff5a5f80; }
  .row.success { border-left-color: #28a74566; }
  .row.debug   { border-left-color: #9996; }

  .empty   { opacity: .6; padding: .25rem; }
  .project { font-weight: 600; }

  .jump { padding: .125rem .5rem; font-size: 12px; }
</style>
