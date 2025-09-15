<script>
	// DiffSummary: compact, shape-agnostic view of a project's local changes.
	// It handles multiple diff formats from the backend and normalizes them for display.

	export let projectName = ""; // readable project label
	export let diff = null; // raw diff payload (various shapes)
	export let disabled = false; // disables Refresh button
	export let onRefresh = () => {}; // callback from parent to re-fetch diff

	/** Return the first present array for any of the provided keys on an object. */
	function pickArray(obj, keys) {
		if (!obj || typeof obj !== "object") return null;
		for (const k of keys) {
			const v = obj[k];
			if (Array.isArray(v)) return v;
		}
		return null;
	}

	/** Classify a diff entry as 'added' | 'changed' | 'removed' (or null if unknown). */
	function classify(entry) {
		// Accept a bunch of synonyms from various emitters.
		const raw = entry?.status ?? entry?.Status ?? entry?.type ?? entry?.Type ?? entry?.action ?? entry?.Action ?? "";

		const v = String(raw).toLowerCase();

		if (["added", "new", "create", "created"].includes(v)) return "added";
		if (["changed", "modify", "modified", "update", "updated"].includes(v)) return "changed";
		if (["removed", "remove", "delete", "deleted"].includes(v)) return "removed";

		return null;
	}

	/** Normalize any "file-ish" value into { path, status? } */
	function toFile(entry, fallbackStatus) {
		if (typeof entry === "string") {
			return { path: entry, status: fallbackStatus || null };
		}
		const path = entry?.path ?? entry?.Path ?? "";
		const status = entry?.status ?? entry?.Status ?? entry?.type ?? entry?.Type ?? entry?.action ?? entry?.Action ?? fallbackStatus ?? null;
		return { path, status };
	}

	/** From a list of entries, build grouped buckets by classification. */
	function groupFromFiles(files) {
		const groups = { added: [], changed: [], removed: [] };
		for (const e of files) {
			const k = classify(e);
			if (k) groups[k].push(toFile(e, k));
			else {
				// Unknown verb → treat as "changed" so it still surfaces
				groups.changed.push(toFile(e, "changed"));
			}
		}
		return groups;
	}

	/** Main normalization: accept many shapes and return { added, changed, removed } arrays. */
	function normalizeDiff(d) {
		if (!d) return { added: [], changed: [], removed: [] };

		// Shape A: Already grouped object: { added, changed, removed }
		const a = pickArray(d, ["added", "Added"]);
		const c = pickArray(d, ["changed", "Changed", "modified", "Modified"]);
		const r = pickArray(d, ["removed", "Removed", "deleted", "Deleted"]);
		if (a || c || r) {
			return {
				added: (a || []).map((x) => toFile(x, "added")),
				changed: (c || []).map((x) => toFile(x, "changed")),
				removed: (r || []).map((x) => toFile(x, "removed")),
			};
		}

		// Shape B: Flat array of entries/events
		if (Array.isArray(d)) {
			return groupFromFiles(d);
		}

		// Shape C: { files: [...] } or { Files: [...] }
		const files = pickArray(d, ["files", "Files"]);
		if (files) return groupFromFiles(files);

		// Shape D: { changedCount, files } with counts only (still use files)
		if (typeof d === "object" && "changedCount" in d) {
			const files2 = Array.isArray(d.files) ? d.files : [];
			return groupFromFiles(files2);
		}

		// Shape E: Event array under various keys (fallback)
		const events = pickArray(d, ["events", "Events"]);
		if (events) return groupFromFiles(events);

		// Unknown shape → show nothing rather than crash
		return { added: [], changed: [], removed: [] };
	}

	// Reactive derivations
	$: groups = normalizeDiff(diff);
	$: added = groups.added;
	$: changed = groups.changed;
	$: removed = groups.removed;

	// Optional: stable keys for list rendering
	function keyFor(item, idx) {
		// Prefer a path if present; fall back to index.
		return (item?.path ?? "") + ":" + idx;
	}
</script>

<div>
	<div class="spread">
		<div class="label">
			Diff for <strong>{projectName || "(none)"}</strong>
		</div>
		<button class="btn" on:click={() => onRefresh?.()} {disabled}>Refresh</button>
	</div>

	{#if added.length + changed.length + removed.length > 0}
		<div class="row" style="margin-top:8px;">
			<span class="badge">Added: {added.length}</span>
			<span class="badge">Changed: {changed.length}</span>
			<span class="badge">Removed: {removed.length}</span>
		</div>

		<ul class="list" style="margin-top:8px; max-height:240px; overflow:auto;">
			{#each added as f, i (keyFor(f, i))}
				<li class="item item-added">+ {f.path ?? f.Path}</li>
			{/each}
			{#each changed as f, i (keyFor(f, i))}
				<li class="item item-changed">~ {f.path ?? f.Path}</li>
			{/each}
			{#each removed as f, i (keyFor(f, i))}
				<li class="item item-removed">- {f.path ?? f.Path}</li>
			{/each}
		</ul>
	{:else}
		<div class="label">No changes detected.</div>
	{/if}
</div>
