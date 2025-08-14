<script>
  export let projects = [];
  export let selected = "";
  export let onChange = () => {};
  export let disabled = false;
</script>

<div class="row">
  <span class="muted">Selected Project:</span>
  <select bind:value={selected} on:change={onChange} {disabled}>
    <option value="" disabled selected>{projects.length ? "Select a project…" : "No projects"}</option>
    {#each projects as p}
      <option value={p.name}>{p.name}</option>
    {/each}
  </select>
</div>
