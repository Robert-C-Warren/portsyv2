<script>
    // Props:
    //  projects: Array{ name, path?, hasPortsy? }
    //  selected: string (bindable)
    //  disabled: boolean
    // onChange?: (name: string) => void 
    //
    // Events:
    //  "selected" detail = string -> enables `bind:selected`
    //  "change" detail = string -> for conventional listeners

    import { createEventDispatcher } from "svelte";

    export let projects = [];
    export let selected = "";
    export let disabled = false;
    export let onChange = null;

    const dispatch = createEventDispatcher();

    // Normalize once so the template is clean
    function normalize(list) {
        const arr = Array.isArray(list) ? list : [];
        return arr.map(p => ({
            name: p.name ?? p.Name ?? "",
            path: p.path ?? p.Path ?? "",
            hasPortsy: p.hasPortsy ?? p.HasPortsy ?? "",
        })).filter(p => p.name); // must have a name to be selectable
    }

    $: options = normalize(projects);

    function choose(e) {
        const v = e.target.value;
        selected = v;
        // For bind:selected to work, dispatch an event named after the prop with raw value
        dispatch("selected", v);
        // Also provide a conventional change event
        dispatch("change", v);
        // Legacy callback
        try { onChange?.(v); } catch {}
    }

    // If the current selected isn't in the options, clear to avoid "stuck" UI
    $: if (selected && !options.find(o => o.name === selected)) {
        selected = "";
        dispatch("selected", selected);
        dispatch("change", selected);
        try { onChange?.(selected); } catch {}
    }
</script>

<label class="label" for="project-select">Project</label>
<select
  id="project-select"
  class="select"
  bind:value={selected}
  on:change={choose}
  disabled={disabled || options.length === 0}
  aria-disabled={disabled || options.length === 0}
  aria-label="Select a project"
>
  <option value="">{options.length ? "Select a project…" : "No projects found"}</option>

  {#each options as p (p.name)}
    <option
      class="project-highlight"
      value={p.name}
      title={p.path || p.name}
    >
      {p.name}{#if !p.hasPortsy} (no .portsy){/if}
    </option>
  {/each}
</select>