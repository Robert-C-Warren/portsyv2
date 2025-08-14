<script>
    export let diff = [];
    export let onRefresh = () => {};
    export let disabled = false;
    export let selectedProject = "";
</script>

<div class="row">
    <button on:click={onRefresh} disabled={disabled}>Refresh Diff</button>
    {#if selectedProject}<span class="muted">({selectedProject})</span>{/if}
</div>

{#if diff?.length === 0}
    <div class="muted">No differences vs. local cache.</div>
{:else}
    <pre>{JSON.stringify(diff, null, 2)}</pre>
{/if}