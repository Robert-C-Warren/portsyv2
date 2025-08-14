<script>
    import PendingList from "./PendingList.svelte";
    export let commitMsg = "";
    export let onPush = () => {};
    export let canPush = false;
    export let pending = [];
</script>

<div class="row">
    <input placeholder="Commit Message..." bind:value={commitMsg} />
    <button disabled={!canPush} on:click={onPush}>Push</button>
</div>

<PendingList items={pending} />