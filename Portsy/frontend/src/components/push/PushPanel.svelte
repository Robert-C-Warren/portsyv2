<script>
	// PushPanel: shows projects with local changes, collects a commit message,
	// and emits a "push" event. It cross-referneces a live `diffs` store to
	// enrich Pending stats when the backend's summary is sparse

	import { createEventDispatcher, onDestroy } from "svelte";
	import { diffs } from "../../stores/diff";

	// Props (controlled by parent App.svelte)
	export let commitMsg = ""; // two-way bound text input
	export let canPush = false; // parent-derived guard (root+project+msg+hasChanges)
	export let pending = []; // pending changes summary from backend
	export let pushing = false; // UI spinner/disabled while a push is running
	export let projectId = null; // hint to map diffs by id

	const dispatch = createEventDispatcher();

	// Live cache of diffs keyed by project id/name/path (store-defined)
	let _diffMap = new Map();

	// Subscribe once; recompute rows whenever diffs change
	const unsub = diffs.subscribe((map) => {
		_diffMap = map || new Map();
		rows = normalize(pending); // re-drive rows with fresh diffs
		console.log("[PushPanel] diff keys:", Array.from(_diffMap.keys()));
	});

	// Normalize a stats object; tolerate various casings and synonyms
	function normalizeStats(s = {}) {
		const added = s.added ?? s.Added ?? 0;
		const changed = s.changed ?? s.Changed ?? s.modified ?? s.Modified ?? 0;
		const removed = s.removed ?? s.Removed ?? s.deleted ?? s.Deleted ?? 0;
		return { added, changed, removed };
	}

	// Produce UI rows from `pending`. If the backend didn't provide per-op counts,
	// enrich from `_diffmap` (which carries detailed changed file lists).
	function normalize(p) {
		const arr =
			Array.isArray(p) ? p
			: p && typeof p === "object" ? Object.entries(p).map(([name, stats]) => ({ name, stats }))
			: [];

		const rows = arr.map((row) => {
			const name = row.name ?? row.Name ?? row.project ?? row.Project ?? "";

			// Base stats fro backend (may be zeros/omitted)
			const baseStats = normalizeStats(row.stats ?? row.Stats);

			// Try multiple keys to find this project's diff details
			const candidates = [row.projectId, row.id, projectId, name, row.absolutePath, row.Path].filter(Boolean);
			let fromDiffs = null;
			for (const k of candidates) {
				const d = _diffMap.get(k);
				if (d) {
					fromDiffs = {
						added: d.added?.length ?? 0,
						changed: d.modified?.length ?? d.changed?.length ?? 0,
						removed: d.deleted?.length ?? d.removed?.length ?? 0,
					};
					break;
				}
			}
			// Prefer nonzero backend counts; otherwise use diff-dreived numbers
			const hasBase = baseStats.added + baseStats.changed + baseStats.removed > 0 ? baseStats : fromDiffs || baseStats;
			const stats = hasBase ? baseStats : fromDiffs || baseStats;
			return { ...row, name, stats };
		});

		// Only show projects with any changes at all
		const filtered = rows.filter((r) => r.stats.added + r.stats.changed + r.stats.removed > 0);
		console.log("[PushPanel] rows:", filtered);
		return filtered;
	}

	// Receive derivation: recompute rows when `pending` changes
	$: rows = normalize(pending);

	// Small helpers for template readability
	function projName(p) {
		return p.name ?? p.Name ?? p.project ?? p.Project ?? "";
	}
	function projStats(p) {
		return normalizeStats(p.stats ?? p.Stats);
	}

	// Emit a push request with a trimmed, bounded message
	function doPush() {
		if (!canPush || pushing) return;
		const msg = (commitMsg || "").trim();
		if (msg.length > 500) return; // Guard (UI enforces via maxlength too)
		dispatch("push", { message: msg || "Update" });
	}

	onDestroy(() => unsub && unsub());
</script>

<div>
	<div class="spread" style="margin-bottom:8px;">
		<!-- Always-visible counter; helps the user see why Push may be disabled -->
		<div class="badge changed-badge" aria-live="polite">
			{rows.length} project(s) changed
		</div>
	</div>

	<!-- Commit message input; supports Ctrl/Cmd+Enter to submit -->
	<input
		class="input"
		bind:value={commitMsg}
		maxlength="500"
		placeholder="What changed?"
		aria-label="Commit message (max 500 characters)"
		on:keydown={(e) => (e.ctrlKey || e.metaKey) && e.key === "Enter" && doPush()}
	/>
	<div class="label commit-guide">Commit message (≤ 500 chars)</div>

	<div class="row pull-refresh" style="margin-top:10px;">
		<button class="btn pull-refresh-btn" on:click={doPush} disabled={!canPush || pushing} aria-disabled={!canPush || pushing}>
			{pushing ? "Pushing…" : "Push"}
		</button>
	</div>

	{#if rows.length > 0}
		<hr />
		<div class="label">Projects with local changes</div>
		<ul class="list" style="max-height:180px; overflow:auto;">
			{#each rows as p (projName(p))}
				{#key projName(p)}
					<li class="item">
						<div class="spread">
							<span>{projName(p)}</span>
							{#if p.stats || p.Stats}
								{#await Promise.resolve(projStats(p)) then s}
									<span class="badge">+{s.added} ~{s.changed} -{s.removed}</span>
								{/await}
							{/if}
						</div>
					</li>
				{/key}
			{/each}
		</ul>
	{/if}
</div>
