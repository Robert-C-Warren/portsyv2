<script>
    export let projects = [];
    export let selected = "";
    export let disabled = false;
    export let onChange = () => {};
    function handle(e){
        selected = e.target.value;
        onChange?.(selected)
    }
</script>

<label class="label" for="project-select">Project</label>
<select id="project-select" class="select" bind:value={selected} on:change={handle} {disabled}>
    <option value="">Select a project…</option>
    {#each projects as p}
        <option value={p.name}>{p.name}</option>
    {/each}
</select>