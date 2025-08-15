<script>
  export let projects = [];
  export let selected = "";
  export let onChange = () => {};
  export let disabled = false;
</script>

<div class="row">
  <span class="muted">Selected Project:</span>
  <select class="dropdown" bind:value={selected} on:change={onChange} {disabled}>
    <option class="dropdown" value="" disabled selected>{projects.length ? "Select a project…" : "No projects"}</option>
    {#each projects as p}
      <option class="dropdown" value={p.name}>{p.name}</option>
    {/each}
  </select>
</div>
