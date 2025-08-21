<script>
    export let projectName = "";
    export let diff = null;
    export let disabled = false;
    export let onRefresh = () => {};
    const count = (arr) => Array.isArray(arr) ? arr.length : (arr?.length ?? 0);
</script>

<div>
    <div class="spread">
        <div class="label">Diff for <strong>{projectName || "(none)"}</strong></div>
        <button class="btn" on:click={() => onRefresh?.()} {disabled}>Refresh</button>
    </div>
    {#if diff}
        <div class="row" style="margin-top:8px;">
            <span class="badge">Added: {count(diff.added ?? diff.Added)}</span>
            <span class="badge">Changed: {count(diff.changed ?? diff.changed)}</span>
            <span class="badge">Removed: {count(diff.removed ?? diff.removed)}</span>
        </div>
        <ul class="list" style="margin-top:8px; max-height:240px; overflow:auto;">
            {#each (diff.added ?? diff.Added ?? []) as f}<li class="item">+  {f.path ?? f.Path}</li>{/each}
            {#each (diff.changed ?? diff.Changed ?? []) as f}<li class="item">~  {f.path ?? f.Path}</li>{/each}
            {#each (diff.removed ?? diff.Removed ?? []) as f}<li class="item">-  {f.path ?? f.Path}</li>{/each}
        </ul>
    {:else}
        <div class="label">No diff loaded.</div>
    {/if}
</div>