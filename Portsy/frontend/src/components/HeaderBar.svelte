<script>
	// - choose a root folder
	// - toggle auto-push after .als saves
	// - scan & fetch pending
	// - start/stop watcher
	//
	// Design notes:
	// - Exposes both callback props *and* Svelte events for parent control.
	// - Shows the chosen root, truncated for readability but full value in title.
	import { createEventDispatcher } from "svelte";

	// ---- Props controlled by parent ----
	export let root = ""; // Chosen root path
	export let watching = false; // Watcher state
	export let autoPush = false;

	// Callback props
	export let pickRoot = () => {}; // Auto-push toggle state
	export let onScan = () => {};
	export let onPending = () => {};
	export let toggleWatch = () => {};
	export let onToggleAutoPush = (v) => {};

	const dispatch = createEventDispatcher();

	// Emit events so parent can manage truth; callbacks still called if provided
	function handlePickRoot() {
		// parent opens a native picker via Wails
		try {
			pickRoot();
		} finally {
			dispatch("pickRoot");
		}
	}

	function handleScan() {
		if (!root) return; // no root; no scan
		try {
			onScan();
		} finally {
			dispatch("scan");
		}
	}

	function handlePending() {
		if (!root) return;
		try {
			onPending();
		} finally {
			dispatch("pending");
		}
	}

	function handleToggleWatch() {
		if (!root) return;
		try {
			toggleWatch();
		} finally {
			dispatch("toggleWatch");
		}
	}

	// Auto-push toggling:
	// - Optimistically update local
	// - Emit an event so the parent can accept/veto and (if needed) re-bind a different value.
	function handleAutoPushChange(e) {
		const v = e.currentTarget.checked;
		try {
			onToggleAutoPush?.(v);
		} catch {}
		dispatch("toggleAutoPush", v);
	}

	// Tiny helper: keep the badge readable while preserving full path in title
	function shortenPath(p, max = 48) {
		if (!p) return "";
		const s = String(p);
		if (s.length <= max) return s;
		// middle-ellipsize
		const head = s.slice(0, Math.ceil((max - 1) / 2));
		const tail = s.slice(-Math.floor((max - 1) / 2));
		return `${head}…${tail}`;
	}
</script>

<div class="spread" style="margin-bottom:12px">
	<!-- Left chunk: root picker -->
	<div class="row" style="gap:8px; align-items:center;">
		<button class="btn" type="button" on:click={handlePickRoot} aria-label="Choose root folder"> Choose Root </button>

		{#if root}
			<span class="badge" title={root} aria-live="polite" aria-label={"Current root: " + root}>
				{shortenPath(root)}
			</span>
		{:else}
			<span class="label" aria-live="polite">No root selected</span>
		{/if}
	</div>

	<!-- Right chunk: auto-push + scan/pending + watch -->
	<div class="row" style="gap:8px; align-items:center;">
		<!-- Auto-push toggle (bindable + evented) -->
		<label class="row" style="gap:6px; align-items:center;">
			<input type="checkbox" bind:checked={autoPush} on:change={handleAutoPushChange} aria-label="Enable auto-push after save" />
			<span>Auto-push after save</span>
		</label>

		<button class="btn" type="button" on:click={handleScan} disabled={!root} aria-disabled={!root} title={!root ? "Choose a root first" : "Scan for projects"}> Scan </button>

		<button class="btn" type="button" on:click={handlePending} disabled={!root} aria-disabled={!root} title={!root ? "Choose a root first" : "Show pending changes"}> Pending </button>

		<button
			class="btn"
			type="button"
			on:click={handleToggleWatch}
			disabled={!root}
			aria-pressed={watching}
			aria-disabled={!root}
			title={!root ? "Choose a root first"
			: watching ? "Stop watching for saves"
			: "Start watching for saves"}
		>
			{watching ? "Stop Watch" : "Start Watch"}
		</button>
	</div>
</div>
