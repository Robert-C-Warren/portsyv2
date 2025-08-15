<script>
    import { createEventDispatcher } from "svelte";
    import PendingList from "./PendingList.svelte";

    export let commitMsg = "";
    export let canPush = false;
    export let pending = [];
    export let pushing = false;
    export let maxLen = 500;
    export let onPush = null;

    const dispatch = createEventDispatcher();

    $: trimmed = (commitMsg || '').trim();
    $: validMsg = trimmed.length > 0 && trimmed.length <= maxLen;
    $: disabled = !canPush || !validMsg || pushing;

    function doPush() {
        if (disabled) return;
        // Prefer events; fallback to function prop for compatability
        if (typeof onPush === 'function') onPush(trimmed);
        else dispatch('push', { message: trimmed });
    }

    function onKeydown(e) {
        // Ctrl/Cmd + Enter to push
        if ((e.key === 'Enter') && (e.ctrlKey || e.metaKey)) {
            e.preventDefault();
            doPush();
        }
    }
</script>

<div class="row" style="gap:8px">
    <input class="root-input" placeholder="Commit Message…" bind:value={commitMsg} maxlength={maxLen} on:keydown={onKeydown} />
    <span class="muted">{commitMsg.length}/{maxLen}</span>
    <button class="top-btn" disabled={disabled} on:click={doPush}>
        {pushing ? 'Pushing…' : 'Push'}
    </button>
</div>

<PendingList items={pending} />