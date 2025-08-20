<script>
    import { listPushableProjects, getDiffForProject, pushProject, onEvent } from "src/lib/api";
	import { onMount } from "svelte";
    let loading = true, error = '', projects = [], selected = '', diff = null, msg = '', pushing = false, notice = '';
    const load = async () => {
        loading = true; error = '', projects = [], selected = '', diff = null;
        try { projects = await listPushableProjects(); }
        catch (e){ error = e.message; }
        finally { loading = false; }
    };
    async function select(name){
        selected = name; diff = null; notice = '';
        try { diff = await getDiffForProject(name); } catch(e){ error = e.message; }
    }
    async function doPush(){
        if (!selected) return;
        if ((msg||'').length > 500) { error = 'Commit message must be ≤ 500 chars'; return; }
        pushing = true; error = ''; notice = '';
        try { await pushProject(selected, msg || 'Update'); msg = ''; notice = 'Pushed ✓'; await load(); }
        catch(e){ error = e.message; }
        finally{ pushing = false; }
    }
    onMount(load);
    onEvent?.('portsy:push:success', (e)=>{ if(e?.project===selected) notice='Pushed ✓'; });
</script>

<div class="panel">
  <div class="spread"><h3>Push</h3><button class="btn" on:click={load} disabled={pushing}>Refresh</button></div>
  {#if error}<p class="label">Error: {error}</p>{/if}
  <div class="row">
    <select class="select" on:change={(e)=>select(e.target.value)}>
      <option value="">Select a project…</option>
      {#each projects as p}
        <option value={p.name || p.Name}>
          {(p.name || p.Name)} {#if p.stats}— +{p.stats?.added||p.Stats?.Added} ~{p.stats?.changed||p.Stats?.Changed} -{p.stats?.removed||p.Stats?.Removed}{/if}
        </option>
      {/each}
    </select>
    <button class="btn" on:click={doPush} disabled={!selected || pushing}>Push</button>
  </div>

  {#if diff}
    <hr />
    <div class="label">Diff for <strong>{selected}</strong></div>
    <div class="row">
      <span class="badge">Added: {diff.added?.length ?? diff.Added?.length ?? 0}</span>
      <span class="badge">Changed: {diff.changed?.length ?? diff.Changed?.length ?? 0}</span>
      <span class="badge">Removed: {diff.removed?.length ?? diff.Removed?.length ?? 0}</span>
    </div>
    <div style="margin-top:10px;">
      <input class="input" placeholder="Commit message (≤500 chars)" bind:value={msg} maxlength="500" />
    </div>
    {#if notice}<p class="label">{notice}</p>{/if}
  {/if}
</div>