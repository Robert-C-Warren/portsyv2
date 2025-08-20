<script>
    export let active = 'projects';
    const tabs = [
        { id: 'projects', label: 'Projects'},
        { id: 'push', label: 'Push'},
        { id: 'pull', label: 'Pull'},
        { id: 'rollback', label: 'Rollback'},
    ];
    export let onChange = (id)=>{};
</script>

<div class="row" style="gap:8px; margin-bottom:12px">
    {#each tabs as t}
        <button class="btn" on:click={() => onChange(t.id)} aria-pressed={active===t.id}>
            {t.label}
        </button> 
    {/each}
</div>