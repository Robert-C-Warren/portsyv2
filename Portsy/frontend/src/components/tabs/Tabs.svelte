<script>
	// - Props:
	//    value: string        -> active tab id (bindable)
	//    tabs:  {id,label}[]  -> list of tabs
	//    onChange?: (id)=>{}  -> optional callback (legacy; events are preferred)
	//
	// - Events:
	//    change: detail = string (the new value)  -> enables `bind:value`
	//    select: detail = { id }                  -> optional, explicit API
	import { createEventDispatcher, onMount } from "svelte";

	export let value = "";
	export let tabs = []; // e.g., [{ id:'projects', label:'Projects' }, ...]
	export let onChange = null; // callback prop

	const dispatch = createEventDispatcher();

	function choose(id) {
		value = id; // update local so the UI reflects immediately
		// Important: for `bind:value`, the 'change' event's detail must be the raw value
		dispatch("change", id);
		// Extra semantic event for `on:select`
		dispatch("select", { id });
		// Legacy callback
		try {
			onChange?.(id);
		} catch {}
	}

	// Keyboard nav: Left/Right move, Home/End jump, Space/Enter activates
	function onKeyDown(e, index) {
		const len = tabs.length;
		if (!len) return;

		const left = e.key === "ArrowLeft";
		const right = e.key === "ArrowRight";
		const home = e.key === "Home";
		const end = e.key === "End";
		const activate = e.key === "Enter" || e.key === " ";

		const focusIndex = (i) => {
			const t = tabs[(i + len) % len];
			if (!t) return;
			const el = document.getElementById(`tab-${t.id}`);
			el?.focus();
		};

		if (left) {
			e.preventDefault();
			focusIndex(index - 1);
		}
		if (right) {
			e.preventDefault();
			focusIndex(index + 1);
		}
		if (home) {
			e.preventDefault();
			focusIndex(0);
		}
		if (end) {
			e.preventDefault();
			focusIndex(len - 1);
		}
		if (activate) {
			e.preventDefault();
			choose(tabs[index].id);
		}
	}

    function tabIndexFor(id) {
        return value === id ? 0 : -1; // roving tabindex pattern
    }

    // If parent passes an invalid value, default to first tab for a sane initial state
    onMount(() => {
        if (!tabs.find(t => t.id === value) && tabs.length) {
            value = tabs[0].id;
            dispatch("change", value);
            dispatch("select", { id: value });
            try { onChange?.(value); } catch {}
        }
    });
</script>

<div class="row" role="tablist" aria-label="Tabs" style="gap:8px; margin: 8px 0;">
  {#each tabs as t, i (t.id)}
    <button
      class="btn {t.id === value ? 'is-active' : ''}"
      role="tab"
      id={`tab-${t.id}`}
      aria-selected={t.id === value}
      aria-controls={`panel-${t.id}`}   
      tabindex={tabIndexFor(t.id)}
      on:click={() => choose(t.id)}
      on:keydown={(e) => onKeyDown(e, i)}
      type="button"
    >
      {t.label}
    </button>
  {/each}
</div>

<style>
  /* Simple active affordance; keeps your existing .btn styling */
  .btn.is-active {
    outline: 2px solid currentColor;
  }
</style>
