<script>
  // Shows remote projects from Firebase / API
  // Lets users select one to view recent commits
  // and toggles whether or not auto-push on save is enabled

  import { listRemoteProjects, getCommitHistory } from "../../lib/api.js";
  import { onMount, createEventDispatcher } from "svelte";

  // Props controlled by parent App.svelte
  export let autoPush = false; // whether auto-push is currently on
  export let onToggleAutoPush = null; // legacy callback

  // Sveltes built in event dispatcher for child -> parent events
  const dispatch = createEventDispatcher();

  // Local reactive state
  let loading = true, error = "", projects = [], selected = "", commits = [];

  // Token guards prevent stale async repsonses from overwriting newer state
  let loadToken = 0;
  let pickToken = 0;

  // Normalize raw project data into a consistent shape
  // Handles both lower- and upper-case field names from API
  function normalizeProjects(raw) {
    const arr = Array.isArray(raw) ? raw : [];
    return arr.map(p => ({
      name: p.name ?? p.Name ?? "",
      head: p.lastCommitId ?? p.LastCommitID ?? null,
    })).filter(p => p.name);
  }

  // Normalize commit history into a consistent shape
  function normalizeCommits(raw) {
    const arr = Array.isArray(raw) ? raw : [];
    return arr.map(c => ({
      id: c.commitId ?? c.CommitId ?? c.id ?? "",
      message: c.message ?? c.Message ?? "",
      timestamp: c.timestamp ?? c.Timestamp ?? "",
      author: c.author ?? c.Author ?? "",
    }));
  }

  // Load the list of remote projects
  async function load() {
    loading = true;
    error = "";
    commits = [];
    const token = ++loadToken; // mark this request
    try {
      const res = await listRemoteProjects();
      if (token !== loadToken) return; // stale response, ignore
      projects = normalizeProjects(res);
    } catch (e) {
      if (token !== loadToken) return;
      error = e?.message || String(e);
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  // Select a projcet -> feth recent commit history
  const pick = async (name) => {
    selected = name;
    dispatch("select", { name }); // bubble selection event to parent
    commits = [];
    const token = ++pickToken;
    try {
      const res = await getCommitHistory(name, 5); // API limited to 5 commits
      if (token !== pickToken) return;
      commits = normalizeCommits(res);
    } catch {/* ignore */}
  };

  // Handle auto-push checkbox changes
  function handleAutoPushChange(e) {
    const v = e.currentTarget.checked;
    autoPush = v; // local immediate UI feedback
    dispatch("toggleAutoPush", v); // notify parent via event
    if (typeof onToggleAutoPush === "function") {
      try { onToggleAutoPush(v); } catch {}
    }
  }

  // Utility: truncate a commit hash for display
  function truncate(id, n = 7) {
    if (!id) return "";
    return String(id).slice(0, n);
  }

  // Utility: format a timestamp nicely
  function formatTime(ts) {
    if (!ts) return "";
    const d = new Date(ts);
    return isNaN(d) ? String(ts) : d.toLocaleString();
  }

  // Kick off the initial project load when component mounts
  onMount(load);
</script>

<!-- === Template === -->
<div class="panel">
  <div class="spread">
    <h3>Projects</h3>
    <div class="row" style="gap:.5rem; align-items:center;">
      <!-- Refresh button reloads the remote project list -->
      <button class="btn" type="button" on:click={load} disabled={loading}>
        {loading ? "Loading…" : "Refresh"}
      </button>
      <!-- Auto-push toggle: bindts to autoPush prop -->
      <label class="row">
        <input
          type="checkbox"
          checked={autoPush}
          on:change={handleAutoPushChange}
          aria-label="Enable auto-push after save"
        />
        <span>Auto-push after save</span>
      </label>
    </div>
  </div>

  <!-- Error / empty states -->
  {#if error}<p class="label">Error: {error}</p>{/if}
  {#if !loading && !error && projects.length === 0}
    <p class="label">No remote projects yet.</p>
  {/if}

  <!-- Project list -->
  <ul class="list">
    {#each projects as p (p.name)}
      <li class="item">
        <button
          type="button"
          class="btn project-row {selected === p.name ? 'is-selected' : ''}"
          on:click={() => pick(p.name)}
          aria-pressed={selected === p.name}
          style="width:100%; text-align:left;"
        >
          <div class="spread">
            <strong>{p.name}</strong>
            {#if p.head}
              <span class="badge">HEAD: {truncate(p.head)}</span>
            {/if}
          </div>
        </button>
      </li>
    {/each}
  </ul>

  <!-- Commit history for selected project  -->
  {#if selected}
    <div class="muted" style="margin-top:.75rem;">Recent commits for {selected}</div>
    {#if commits.length === 0}
      <p class="label">No commits yet.</p>
    {:else}
      <ul class="list">
        {#each commits as c (c.id)}
          <li class="item">
            <div class="row">
              <span class="badge">{truncate(c.id)}</span>
              <span style="margin-left:.5rem; flex:1;">{c.message}</span>
            </div>
            <div class="muted" style="font-size:.85em;">
              {formatTime(c.timestamp)}{#if c.author} • {c.author}{/if}
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}
</div>

<style>
  /* Highlight the selected project row */
  .project-row.is-selected { outline: 2px solid currentColor; }
</style>
