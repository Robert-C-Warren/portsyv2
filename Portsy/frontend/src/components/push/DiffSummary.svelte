<script>
    export let projectName = "";
    export let diff = null;
    export let disabled = false;
    export let onRefresh = () => {};
    
    // Return the first present array for any of the provided keys
    function pickArray(obj, keys) {
        if (!obj || typeof obj !== "object") return null;
        for (const k of keys) {
            const v = obj[k];
            if (Array.isArray(v)) return v;
        }
        return null;
    }

    // Find an event array if diff is event-shaped
    function getEventArray(d) {
        if (Array.isArray(d)) return d;
        return pickArray(d, ["events", "Events", "files", "Files"]) || null;
    }

    // Normalize one category into a file-array for display/counting
    function normalizeList(d, arrayKeys, eventMatches) {
        // 1) Object-with-array case
        const arr = pickArray(d, arrayKeys);
        if (arr) return arr;

        // 2) Event array case (filter by Type)
        const evs = getEventArray(d) || d; // d might already be the event array
        if (!Array.isArray(evs)) return [];

        return evs.filter((e) => {
            const t = String(e.Type ?? e.type ?? e.Action ?? e.action ?? "").toLowerCase();
            return eventMatches.includes(t);
        })
        .map((e) => ({
            path: e.path ?? e.Path,
            type: e.Type ?? e.type
        }));
    }

    // Map "modified" -> changed, accedpt multiple verbs
    $: added = normalizeList(diff, ["added","Added"], ["added","new","create","created"]);
    $: changed = normalizeList(diff, ["changed","Changed","modified","Modified"], ["changed","modify","modified","update","updated"]);
    $: removed = normalizeList(diff, ["removed","Removed","deleted","Deleted"],["removed","delete","deleted"]);
</script>

<div>
    <div class="spread">
        <div class="label">Diff for <strong>{projectName || "(none)"}</strong></div>
        <button class="btn" on:click={() => onRefresh?.()} disabled={disabled}>Refresh</button>
    </div>
    
    {#if diff}
        <div class="row" style="margin-top:8px;">
            <span class="badge">Added: {added.length}</span>
            <span class="badge">Changed: {changed.length}</span>
            <span class="badge">Removed: {removed.length}</span>
        </div>
        <ul class="list" style="margin-top:8px; max-height:240px; overflow:auto;">
            {#each added as f}<li class="item item-added">+  {f.path ?? f.Path}</li>{/each}
            {#each changed as f}<li class="item item-changed">~  {f.path ?? f.Path}</li>{/each}
            {#each removed as f}<li class="item item-removed">-  {f.path ?? f.Path}</li>{/each}
        </ul>
    {:else}
        <div class="label">No diff loaded.</div>
    {/if}
</div>