<script>
  import { createEventDispatcher } from "svelte";

  export let commitMsg = "";
  export let canPush = false;
  export let pending = [];   // can be Array OR Object map { [project]: stats }
  export let pushing = false;

  const dispatch = createEventDispatcher();

  // Normalize `pending` into an array of { name, stats }
  function normalize(p) {
    if (Array.isArray(p)) return p;
    if (p && typeof p === "object") {
      return Object.entries(p).map(([name, stats]) => ({ name, stats }));
    }
    return [];
  }

  // reactive normalized rows
  $: rows = normalize(pending);

  // helpers to read names/stats regardless of key casing
  function projName(p) {
    return p.name ?? p.Name ?? p.project ?? p.Project ?? "";
  }
  function projStats(p) {
    const s = p.stats ?? p.Stats ?? {};
    return {
      added: s.added ?? s.Added ?? 0,
      changed: s.changed ?? s.Changed ?? 0,
      removed: s.removed ?? s.Removed ?? 0
    };
  }

  function doPush() {
    if (!canPush || pushing) return;
    const msg = (commitMsg || "").trim();
    if (msg.length > 500) return;
    dispatch("push", { message: msg || "Update" });
  }
</script>

<div>
  <div class="spread" style="margin-bottom:8px;">
    <div class="badge changed-badge">{rows.length} project(s) changed</div>
  </div>

  <input
    class="input"
    bind:value={commitMsg}
    maxlength="500"
    placeholder="What changed?"
    on:keydown={(e) => (e.ctrlKey || e.metaKey) && e.key === "Enter" && doPush()}
  />
  <div class="label commit-guide">Commit message (≤ 500 chars)</div>

  <div class="row pull-refresh" style="margin-top:10px;">
    <button class="btn pull-refresh-btn" on:click={doPush} disabled={!canPush || pushing}>
      {pushing ? "Pushing…" : "Push"}
    </button>
  </div>

  {#if rows.length > 0}
    <hr />
    <div class="label">Projects with local changes</div>
    <ul class="list" style="max-height:180px; overflow:auto;">
      {#each rows as p, i (projName(p))}
        <li class="item">
          <div class="spread">
            <span>{projName(p)}</span>
            {#if p.stats || p.Stats}
              <span class="badge">
                +{projStats(p).added}
                ~{projStats(p).changed}
                -{projStats(p).removed}
              </span>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</div>
