<script>
	export let projectName = "";
	export let diff = [];
	export let onRefresh = () => {};
	export let disabled = false;

	// Normalize mixed-case fields from the CLI and bucket them.
	function normalize(c) {
		const path = c.path ?? c.Path ?? "";
		const raw = (c.kind ?? c.Kind ?? c.status ?? c.Status ?? "").toLowerCase();
		let kind = "other";
		if (raw.startsWith("a")) kind = "added";
		else if (raw.startsWith("m")) kind = "modified";
		else if (raw.startsWith("d") || raw.startsWith("r")) kind = "deleted";
		return {
			kind,
			path,
			sizeDelta: c.sizeDelta ?? c.SizeDelta ?? null,
		};
	}

	$: items = Array.isArray(diff) ? diff.map(normalize) : [];
	$: counts = {
		added: items.filter((x) => x.kind === "added").length,
		modified: items.filter((x) => x.kind === "modified").length,
		deleted: items.filter((x) => x.kind === "deleted").length,
		other: items.filter((x) => x.kind === "other").length,
	};
	$: byKind = {
		added: items.filter((x) => x.kind === "added"),
		modified: items.filter((x) => x.kind === "modified"),
		deleted: items.filter((x) => x.kind === "deleted"),
		other: items.filter((x) => x.kind === "other"),
	};

	let open = { added: true, modified: true, deleted: true, other: false };

	async function copySummary() {
		const lines = [
			`Project: ${projectName || "(none)"}`,
			`+${counts.added}  ~${counts.modified}  -${counts.deleted}`,
			...["added", "modified", "deleted", "other"].flatMap((k) => (byKind[k].length ? [``, `${k.toLocaleUpperCase()} (${byKind[k].length})`, ...byKind[k].map((i) => ` • ${i.path}`)] : [])),
		].join("\n");
		try {
			await navigator.clipboard.writeText(lines);
		} catch {}
	}
</script>

<div class="summary-head">
	<div class="left">
		<div class="title">
			Changes &nbsp;{#if projectName}<span class="muted">for&nbsp;</span> <strong>{projectName}</strong>{/if}
		</div>
		<div class="badges">
			<span class="badge ok">+{counts.added}</span>
			<span class="badge warn">+{counts.modified}</span>
			<span class="badge err">+{counts.deleted}</span>
			{#if counts.other}<span class="badge">{counts.other}</span>{/if}
		</div>
	</div>
	<div class="right">
		<button on:click={onRefresh} {disabled}>Refresh</button>
		<button on:click={copySummary} disabled={items.length === 0}>Copy Summary</button>
	</div>
</div>

{#if items.length === 0}
	<div class="muted">No differences vs. local cache.</div>
{:else}
	{#each ["added", "modified", "deleted", "other"] as k}
		{#if byKind[k].length}
			<details class="group" bind:open={open[k]}>
				<summary>
					<span class="gicon">
						{#if k === "added"}➕{:else if k === "modified"}✎{:else if k === "deleted"}🗑{:else}…{/if}
					</span>
					<span class="glabel">{k.charAt(0).toUpperCase() + k.slice(1)}</span>
					<span class="count">{byKind[k].length}</span>
				</summary>
				<ul class="filelist">
					{#each byKind[k] as i}
						<li>
							<code>{i.path}</code>
							{#if i.sizeDelta !== null}<span class="muted"> ({i.sizeDelta} B)</span>{/if}
						</li>
					{/each}
				</ul>
			</details>
		{/if}
	{/each}
{/if}

<style>
	.summary-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		margin-bottom: 8px;
	}
	.summary-head .title {
		font-weight: 600;
	}
	.badges {
		display: flex;
		gap: 6px;
	}

	.badge {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 2px 8px;
		border-radius: 999px;
		font-size: 12px;
		background: rgba(255, 255, 255, 0.15);
	}
	.badge.ok {
		background: #c8f7d0;
		color: #0f4d2e;
	}
	.badge.warn {
		background: #fff2c2;
		color: #7a5a00;
	}
	.badge.err {
		background: #ffd1d1;
		color: #7a0610;
	}

	.group {
		margin: 8px 0;
	}
	.group summary {
		display: flex;
		align-items: center;
		gap: 8px;
		cursor: pointer;
		padding: 4px 0;
	}
	.gicon {
		width: 1.2em;
		text-align: center;
	}
	.count {
		margin-left: 6px;
		font-size: 12px;
		background: rgba(255, 255, 255, 0.15);
		padding: 1px 6px;
		border-radius: 999px;
	}
	.filelist {
		list-style: none;
		padding-left: 0;
		margin: 6px 0 0;
	}
	.filelist li {
		padding: 4px 6px;
		border-radius: 6px;
	}
	.filelist li:hover {
		background: rgba(255, 255, 255, 0.08);
	}
	code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
	}
	.muted {
		color: #ffffff;
		opacity: 0.85;
	}
</style>
