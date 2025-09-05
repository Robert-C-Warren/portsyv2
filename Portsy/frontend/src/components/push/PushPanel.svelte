<script>
	import { createEventDispatcher, onDestroy } from "svelte";
	import { diffs } from "../../stores/diff";

	export let commitMsg = "";
	export let canPush = false;
	export let pending = [];
	export let pushing = false;
	export let projectId;

	const dispatch = createEventDispatcher();

	let _diffMap = new Map();
	const unsub = diffs.subscribe((map) => {
		_diffMap = map || new Map();
		rows = normalize(pending);
		console.log("[PushPanel] subscribe diffs -> keys:", Array.from(_diffMap.keys()));
	});

	function normalizeStats(s = {}) {
		const added = s.added ?? s.Added ?? 0;
		const changed = s.changed ?? s.Changed ?? s.modified ?? s.Modified ?? 0;
		const removed = s.removed ?? s.Removed ?? s.deleted ?? s.Deleted ?? 0;
		return { added, changed, removed };
	}

	function normalize(p) {
		const arr =
			Array.isArray(p) ? p
			: p && typeof p === "object" ? Object.entries(p).map(([name, stats]) => ({ name, stats }))
			: [];

		const rows = arr.map((row) => {
			const name = row.name ?? row.Name ?? row.project ?? row.Project ?? "";
			const baseStats = normalizeStats(row.stats ?? row.Stats);
			// try to enrich from diffs using multiple possible keys
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
			const stats = (baseStats.added | baseStats.changed | baseStats.removed) > 0 ? baseStats : fromDiffs || baseStats;
			return { ...row, name, stats };
		});

		const filtered = rows.filter((r) => (r.stats.added | r.stats.changed | r.stats.removed) > 0);
		console.log("[PushPanel] rows computed:", filtered);
		return filtered;
	}

	$: rows = normalize(pending);

	function projName(p) {
		return p.name ?? p.Name ?? p.project ?? p.Project ?? "";
	}
	function projStats(p) {
		return normalizeStats(p.stats ?? p.Stats);
	}

	function doPush() {
		if (!canPush || pushing) return;
		const msg = (commitMsg || "").trim();
		if (msg.length > 500) return;
		dispatch("push", { message: msg || "Update" });
	}

	onDestroy(() => unsub && unsub());
</script>

<div>
	<div class="spread" style="margin-bottom:8px;">
		<div class="badge changed-badge">{rows.length} project(s) changed</div>
	</div>

	<input class="input" bind:value={commitMsg} maxlength="500" placeholder="What changed?" on:keydown={(e) => (e.ctrlKey || e.metaKey) && e.key === "Enter" && doPush()} />
	<div class="label commit-guide">Commit message (≤ 500 chars)</div>

	<div class="row pull-refresh" style="margin-top:10px;">
		<button class="btn pull-refresh-btn" on:click={doPush} disabled={!canPush || pushing}>
			{pushing ? "Pushing…" : "Push"}
		</button>
	</div>

	{#if rows.length > 0}
		<hr />
		<div class="label">Projects with local changes</div>
		<ul class="list" style="max-height:180px; overflow:auto;">
			{#each rows as p, i (projName(p))}
				<li class="item">
					<div class="spread">
						<span>{projName(p)}</span>
						{#if p.stats || p.Stats}
							<span class="badge">
								+{projStats(p).added}
								~{projStats(p).changed}
								-{projStats(p).removed}
							</span>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
	<div class="debug" style="margin-top:10px; font-family:monospace; font-size:12px; opacity:0.8;">
		<div>diff keys: {JSON.stringify(Array.from(_diffMap.keys()))}</div>
		<div>projectId prop: {projectId}</div>
	</div>
</div>
