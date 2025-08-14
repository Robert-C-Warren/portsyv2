<script>
  export let root = "";
  export let onScan = () => {};
  export let onPending = () => {};
  export let watching = false;
  export let toggleWatch = () => {};
  export let pickRoot = () => {};
</script>

<div class="header-section">
  <header>
    <div class="projects-root-head">
      <span>Ableton Projects Root:</span>
      <div class="root-display" title={root}>
        {root || "None selected"}
      </div>
      <button class="top-btn" on:click={pickRoot}>Choose…</button>
    </div>

    <div class="scan-pend-btns">
      <button class="top-btn" on:click={onScan} disabled={!root}>Scan Root</button>
      <button class="top-btn" on:click={onPending} disabled={!root}>Pending Sync</button>
    </div>

    <label style="margin-right:auto">
      <input type="checkbox" class="big" bind:checked={watching} on:change={toggleWatch} disabled={!root} />
      Watch + AutoPush
    </label>
  </header>
</div>
