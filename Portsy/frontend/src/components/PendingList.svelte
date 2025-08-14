<script>
    export let items = [];
</script>

{#if items?.length}
    <div class="muted">Pending Projects</div>
    {#each items as p}
        <div class="row">• {p.Name} (+{p.Added} ~{p.Modified} -{p.Deleted})</div>
    {/each}
{:else}
    <div class="muted">No Pending Changes.</div>
{/if}