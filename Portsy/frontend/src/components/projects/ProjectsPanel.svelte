<script>
  import { listRemoteProjects, getCommitHistory } from "../../lib/api.js";
  import { onMount, createEventDispatcher } from "svelte";

  // ✅ receive from parent
  export let autoPush = false;
  // optional callback prop (parent may pass), fallback to event
  export let onToggleAutoPush = (v) => {};

  const dispatch = createEventDispatcher();

  let loading = true, error = "", projects = [], selected = "", commits = [];

  async function load() {
    loading = true;
    error = "";
    commits = [];
    try {
      projects = await listRemoteProjects();
    } catch (e) {
      error = e?.message || String(e);
    } finally {
      loading = false;
    }
  }

  const pick = async (name) => {
    selected = name;
    commits = [];
    try {
      commits = await getCommitHistory(name, 5);
    } catch {}
  };

  function handleAutoPushChange(e) {
    const v = e.currentTarget.checked;
    // let parent know
    try { onToggleAutoPush(v); } catch {}
    dispatch("toggleAutoPush", v);
  }

  // ✅ do initial load once
  onMount(load);
</script>

<div class="panel">
  <div class="spread">
    <h3>Projects</h3>
    <button class="btn" on:click={load}>Refresh</button>
    <label class="row">
      <input type="checkbox" bind:checked={autoPush} on:change={handleAutoPushChange} />
      <span>Auto-push after save</span>
    </label>
  </div>

  {#if loading}<p class="label">Loading…</p>{/if}
  {#if error}<p class="label">Error: {error}</p>{/if}

  <ul class="list">
    {#each projects as p}
      <li class="item">
        <button type="button" class="btn" on:click={() => pick(p.name || p.Name)} style="width:100%; text-align:left;">
          <div class="spread" style="pointer-events:none;">
            <strong>{p.name || p.Name}</strong>
            {#if p.lastCommitId || p.LastCommitID}
              <span class="badge">HEAD: {p.lastCommitId || p.LastCommitID}</span>
            {/if}
          </div>
        </button>
      </li>
    {/each}
  </ul>
</div>
