<script>
  import { onMount } from "svelte";
  import { ListRemoteProjects, Pull } from "../../../wailsjs/go/main/App.js";

  export let root = "";   // <-- receive root from App.svelte

  let projects = [];
  let selected = "";
  let commitId = "";
  let allowDelete = false;
  let pulling = false;
  let loading = false;
  let error = "";
  let notice = "";

  const normalize = (list) =>
    (Array.isArray(list) ? list : [])
      .map(p => ({ name: p.name || p.Name || "", last: p.lastCommitId || p.LastCommitID || "" }))
      .filter(p => p.name);

  function joinWin(a, b) {
    if (!a) return b || "";
    const needsSep = !a.endsWith("\\") && !a.endsWith("/");
    return a + (needsSep ? "\\" : "") + (b || "");
  }

  async function loadProjects() {
    loading = true; error = ""; notice = "";
    try {
      const res = await ListRemoteProjects();
      projects = normalize(res);
      if (!selected && projects.length) selected = projects[0].name;
    } catch (e) {
      error = e?.message || String(e);
      projects = [];
    } finally {
      loading = false;
    }
  }

  async function doPull() {
    if (!selected) return;
    const dest = joinWin(root, selected);        // <-- build DEST under chosen root
    pulling = true; error = ""; notice = "";
    try {
      // Your Wails method signature: Pull(project, dest, commit, force)
      await Pull(selected, dest, commitId, allowDelete);
      notice = `Pulled to ${dest} ✓`;
    } catch (e) {
      error = e?.message || String(e);
    } finally {
      pulling = false;
    }
  }

  onMount(loadProjects);
</script>

<div class="panel">
  <div class="spread">
    <h3>Pull</h3>
</div>

{#if error}<p class="label">Error: {error}</p>{/if}

<div class="row">
    <select class="select" bind:value={selected} disabled={loading || pulling || projects.length===0}>
        <option value="">{projects.length ? "Select remote project…" : "No remote projects found"}</option>
        {#each projects as p}
        <option value={p.name}>{p.name}{#if p.last} — HEAD: {p.last}{/if}</option>
        {/each}
    </select>
  </div>

  <p class="label" style="margin-top:8px;">
    Destination: {root ? `${root}\\${selected || "(project)"}` : "(choose a root in the header first)"}
  </p>

  <div class="row pull-refresh">
    <button class="btn pull-refresh-btn" on:click={doPull} disabled={!selected || pulling || !root}>
        {pulling ? "Pulling…" : "Pull"}
    </button>
    <button class="btn pull-refresh-btn" on:click={loadProjects} disabled={loading || pulling}>
      {loading ? "Loading…" : "Refresh"}
    </button>
  </div>

  {#if notice}<p class="label">{notice}</p>{/if}
</div>
