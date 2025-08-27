<script>
  export let root = "";
  export let watching = false;
  export let pickRoot = () => {};
  export let onScan = () => {};
  export let onPending = () => {};
  export let toggleWatch = () => {};
  export let autoPush = false;
  export let onToggleAutoPush = (v) => {};
</script>

<div class="spread" style="margin-bottom:12px">
  <div class="row" style="gap:8px; align-items:center;">
    <button class="btn" on:click={pickRoot}>Choose Root</button>
    {#if root}<span class="badge">{root}</span>{/if}
  </div>

  <div class="row" style="gap:8px; align-items:center;">
    <!--- auto-push toggle -->
    <label class="row" style="gap:6px; align-items:center;">
      <input
        type="checkbox"
        bind:checked={autoPush}
        on:change={(e) => onToggleAutoPush(e.currentTarget.checked)}
      />
      <span>Auto-push after save</span>
    </label>

    <button class="btn" on:click={onScan}>Scan</button>
    <button class="btn" on:click={onPending}>Pending</button>
    <button class="btn" on:click={toggleWatch} disabled={!root}>
      {watching ? "Stop Watch" : "Start Watch"}
    </button>
  </div>
</div>
