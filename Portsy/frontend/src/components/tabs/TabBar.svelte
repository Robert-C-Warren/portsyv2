<script>
  import { createEventDispatcher, onMount } from "svelte";

  export let active = "projects";
  export let tabs = [
    { id: "projects", label: "Projects" },
    { id: "push",     label: "Push" },
    { id: "pull",     label: "Pull" },
    { id: "rollback", label: "Rollback" },
  ];

  const dispatch = createEventDispatcher();

  function select(id) {
    active = id;
    dispatch("change", { id });
  }

  // Store button nodes by index
  let btnRefs = [];

  // Svelte action to capture/destroy a node reference
  function capture(index) {
    return (node) => {
      btnRefs[index] = node;
      return {
        destroy() {
          if (btnRefs[index] === node) btnRefs[index] = null;
        }
      };
    };
  }

  function focusIndex(i) {
    if (i < 0 || i >= tabs.length) return;
    btnRefs[i]?.focus();
  }

  function onKeyDown(e, index) {
    const left  = e.key === "ArrowLeft"  || e.key.toLowerCase() === "a";
    const right = e.key === "ArrowRight" || e.key.toLowerCase() === "d";
    const home  = e.key === "Home";
    const end   = e.key === "End";
    if (left)  { e.preventDefault(); focusIndex((index - 1 + tabs.length) % tabs.length); }
    if (right) { e.preventDefault(); focusIndex((index + 1) % tabs.length); }
    if (home)  { e.preventDefault(); focusIndex(0); }
    if (end)   { e.preventDefault(); focusIndex(tabs.length - 1); }
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      select(tabs[index].id);
    }
  }

  function tabIndexFor(id) { return active === id ? 0 : -1; }

  onMount(() => {
    if (!tabs.find(t => t.id === active) && tabs.length) active = tabs[0].id;
  });
</script>

<div class="row" role="tablist" aria-label="Portsy sections" style="gap:8px; margin-bottom:12px">
  {#each tabs as t, i (t.id)}
    <button
      class="btn {active === t.id ? 'is-active' : ''}"
      role="tab"
      aria-selected={active === t.id}
      aria-controls={`panel-${t.id}`}
      id={`tab-${t.id}`}
      tabindex={tabIndexFor(t.id)}
      on:click={() => select(t.id)}
      on:keydown={(e) => onKeyDown(e, i)}
      use:capture={i}
    >
      {t.label}
    </button>
  {/each}
</div>

<style>
  .btn.is-active { outline: 2px solid currentColor; }
</style>
