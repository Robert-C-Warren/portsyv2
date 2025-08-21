<script>
    export let root = "";
    export let watching = false;
    export let pickRoot = () => {};
    export let onScan = () => {};
    export let onPending = () => {};
    export let toggleWatch = () => {};
</script>

<div class="spread" style="margin-bottom:12px">
    <div>
        <button class="btn" on:click={pickRoot}>Choose Root</button>
        {#if root}<span class="badge" style="margin-left:8px;">{root}</span>{/if}
    </div>
    <div class="row">
        <button class="btn" on:click={onScan}>Scan</button>
        <button class="btn" on:click={onPending}>Pending</button>
        <button class="btn" on:click={toggleWatch}>{watching ? "Stop Watch" : "Start Watch"}</button>
    </div>
</div>