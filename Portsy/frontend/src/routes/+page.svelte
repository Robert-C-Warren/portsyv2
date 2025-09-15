<script>
  // Minimal scanner page: query backend for Ableton projects under a root
  // and render basic metadata + HEAD commit preview.

  // Wails/Vite prefers the explicit .js extension
  import { ScanProjects } from '../../wailsjs/go/main/App.js';
  import { tick } from 'svelte';

  /** @typedef {{ id:string; message:string; timestamp:number|string }} CommitMeta */
  /** @typedef {{ name:string; path:string; alsFile:string; hasPortsy:boolean; lastCommit?: CommitMeta|null }} AbletonProject */

  /** @type {AbletonProject[]} */
  let projects = [];
  let rootPath = '';

  let loading = false;
  let error = '';

  // Token guard: if multiple scans fire quickly, ignore stale responses
  let scanToken = 0;

  function normalize(list) {
    const arr = Array.isArray(list) ? list : [];
    return arr.map(p => ({
      name: p.name ?? p.Name ?? '',
      path: p.path ?? p.Path ?? '',
      alsFile: p.alsFile ?? p.ALSFile ?? p.als ?? '',
      hasPortsy: p.hasPortsy ?? p.HasPortsy ?? false,
      lastCommit: p.lastCommit ?? p.LastCommit ?? null
    })).filter(p => p.name);
  }

  function fmtCommit(c) {
    if (!c) return '—';
    const t = new Date(c.timestamp);
    const when = isNaN(+t) ? String(c.timestamp) : t.toLocaleString();
    const idShort = (c.id || '').slice(0, 7);
    return `${idShort ? idShort + ' • ' : ''}${c.message || 'no message'} • ${when}`;
  }

  async function scan() {
    if (!rootPath.trim()) {
      error = 'Please enter a root path first.';
      projects = [];
      return;
    }

    const token = ++scanToken;
    loading = true;
    error = '';
    try {
      /** @type {AbletonProject[] | string} */
      let res = await ScanProjects(rootPath);

      // Some Wails bindings return JSON strings; accept both
      if (typeof res === 'string') {
        try {
          res = JSON.parse(res);
        } catch {
          throw new Error('ScanProjects returned non-JSON string.');
        }
      }

      if (token !== scanToken) return; // stale response; user triggered another scan
      projects = normalize(res);
    } catch (e) {
      if (token !== scanToken) return;
      error = e?.message || String(e);
      projects = [];
      console.error('ScanProjects failed:', e);
    } finally {
      if (token === scanToken) loading = false;
      await tick(); // let DOM flush if you add follow-up logic
    }
  }
</script>

<div class="row" style="gap:.5rem; align-items:center;">
  <input
    type="text"
    class="input"
    bind:value={rootPath}
    placeholder="Enter Ableton Projects Root"
    spellcheck="false"
    on:keydown={(e) => e.key === 'Enter' && !loading && scan()}
    style="min-width:24rem;"
    aria-label="Ableton projects root path"
  />
  <button class="btn" on:click={scan} disabled={loading}>
    {loading ? 'Scanning…' : 'Scan Projects'}
  </button>
</div>

{#if error}
  <p class="label" role="alert">Error: {error}</p>
{/if}

{#if !loading && projects.length === 0 && !error}
  <p class="label">No projects found. Each project folder should contain a .als.</p>
{/if}

<ul class="list" style="margin-top:.5rem;">
  {#each projects as p (p.path || p.name)}
    <li class="item">
      <div class="spread">
        <div>
          <b>{p.name}</b>
          <span class="badge" title={p.path}>{p.hasPortsy ? '• .portsy' : '• no .portsy'}</span>
          <div class="muted">.als: {p.alsFile || '—'}</div>
        </div>
        <div class="muted" style="text-align:right; max-width:50%;">
          <div>HEAD: {fmtCommit(p.lastCommit)}</div>
        </div>
      </div>
    </li>
  {/each}
</ul>

<style>
  .list { padding-left: 0; }
  .item { list-style: none; padding: .5rem; border-bottom: 1px solid rgba(0,0,0,.06); }
  .spread { display: flex; justify-content: space-between; gap: 1rem; align-items: baseline; }
  .badge { margin-left: .5rem; opacity: .7; }
  .muted { opacity: .75; }
</style>
