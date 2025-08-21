<script>
    import { createEventDispatcher } from "svelte";
    export let value = "";
    export let tabs = [];
    export let onChange = (id)=>{};
    const dispatch = createEventDispatcher();
    function choose(id) {
        value = id;
        onChange?.(id);
        dispatch("change", { value:id });
    }
</script>

<div class="row" style="gap:8px; margin: 8px 0;">
    {#each tabs as t}
        <button class="btn" aria-pressed={t.id===value} on:click={() => choose(t.id)}>{t.label}</button>
    {/each}
</div>