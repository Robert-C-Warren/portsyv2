<!-- frontend/src/App.svelte -->
<script>
	import "./style.css";
	import { ScanJSON, PendingJSON, DiffJSON, Push, StartWatcherAll, StopWatcherAll } from "../wailsjs/go/main/App.js";
	import { EventsOn } from "../wailsjs/runtime/runtime.js";

	let root = "C:\\Projects";
	let currentTab = "projects";
	let log = "";
	let projects = [];
	let pending = [];
	let selectedProject = "";
	let diff = [];
	let watching = false;
	let commitMsg = "";

	function show(s) {
		log = typeof s === "string" ? s : JSON.stringify(s, null, 2);
	}

	async function loadProjects() {
		const res = await ScanJSON(root);
		projects = JSON.parse(res);
		if (!selectedProject && projects.length) selectedProject = projects[0].name;
		show(res);
	}

	async function loadPending() {
		const res = await PendingJSON(root);
		pending = JSON.parse(res);
		show(res);
	}

	async function loadDiff() {
		if (!selectedProject) return;
		const res = await DiffJSON(root, selectedProject);
		diff = JSON.parse(res);
		show(res);
	}

	async function doPush() {
		const res = await Push(root, selectedProject, commitMsg);
		show(res || "pushed");
		await loadPending();
		await loadDiff();
	}

	async function toggleWatch(e) {
		if (e?.target?.checked) {
			await StartWatcherAll(root, true);
			watching = true;
		} else {
			await StopWatcherAll();
			watching = false;
		}
	}

	// live events from watcher
	EventsOn("alsSaved", (p) => {
		log = `Saved: ${p.project} @ ${p.at}\n` + log;
		loadPending();
	});
	EventsOn("pushDone", (p) => {
		log = `Pushed: ${p.project}\n` + log;
		loadPending();
		loadDiff();
	});

	// Initial load
	loadProjects().then(loadPending);
</script>

<div class="shell">
	<!-- -----------HEADER SECTION----------- -->
	<div class="header-section">
		<header>
			<div class="projects-root-head">
				<label>
					Ableton Projects Root:
					<input class="root-input" bind:value={root} placeholder="C:\Projects" />
				</label>
			</div>

			<div class="scan-pend-btns">
				<button class="top-btn" on:click={loadProjects}>Scan Root</button>
				<button class="top-btn" on:click={loadPending}>Pending Sync</button>
			</div>

			<label style="margin-right:auto">
				<input type="checkbox" class="big" bind:checked={watching} on:change={toggleWatch} />
				Watch + AutoPush
			</label>
		</header>
	</div>

	<div class="tabs">
		{#each ["projects", "push", "pull", "rollback"] as t}
			<button type="button" class="tab {currentTab === t ? 'active' : ''}" on:click={() => (currentTab = t)}>
				{t}
			</button>
		{/each}
	</div>

	<div class="content">
		<!-- left pane: context controls -->
		<div class="panel">
			<div class="row">
				<span class="muted">Selected project:</span>
				<select bind:value={selectedProject} on:change={loadDiff}>
					{#each projects as p}
						<option value={p.name}>{p.name}</option>
					{/each}
				</select>
			</div>

			{#if currentTab === "push"}
				<div class="row">
					<input placeholder="Commit message…" bind:value={commitMsg} />
					<button disabled={!selectedProject} on:click={doPush}>Push</button>
				</div>
				<div class="muted">Pending projects:</div>
				{#each pending as p}
					<div class="row">• {p.Name} (+{p.Added} ~{p.Modified} -{p.Deleted})</div>
				{/each}
			{/if}

			{#if currentTab === "projects"}
				<div class="muted">Projects found:</div>
				{#each projects as p}
					<div class="row">• {p.name} {p.hasPortsy ? "" : "(no .portsy)"}</div>
				{/each}
			{/if}

			<!-- TODO: pull/rollback panes can go here -->
		</div>

		<!-- right pane: log/diff -->
		<div class="panel">
			{#if currentTab === "push" || currentTab === "projects"}
				<div class="row">
					<button on:click={loadDiff} disabled={!selectedProject}>Refresh Diff</button>
				</div>
				{#if diff.length === 0}
					<div class="muted">No differences vs. local cache.</div>
				{:else}
					<pre>{JSON.stringify(diff, null, 2)}</pre>
				{/if}
			{/if}

			<hr />
			<div class="muted">Event log / JSON output</div>
			<pre>{log}</pre>
		</div>
	</div>
</div>

<style>
	.shell {
		display: flex;
		flex-direction: column;
		height: 100vh;
		background-color: #7a7a7a;
	}
	.header-section {
		display: inline;
		flex-direction: row;
		align-items: center;
		padding: 10px 12px;
		border-bottom: 1px solid #ffffff;
	}
	.root-input {
		min-height: 30px;
		border-radius: 6px;
		border: none;
		text-align: center;
	}
	.projects-root-head input {
		width: 360px;
	}
	.scan-pend-btns {
		margin: 15px;
	}
	/* make the actual box bigger */
	input[type="checkbox"].big {
		transform: scale(1.6); /* size */
		transform-origin: left center; /* keep it aligned */
		margin-right: 8px; /* compensate */
		accent-color: #111; /* optional: color in Chromium/WebKit */
	}
	.top-btn {
		border-radius: 6px;
		padding: 10px;
		margin: auto 10px;
		min-width: 130px;
		background: none;
		background-color: #91dcff;
		cursor: pointer;
		border: none;
	}
	.tabs {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		border: none;
		border-bottom: 1px solid #ffffff;
	}
	.tab {
		padding: 6px 10px;
		border-radius: 6px;
		cursor: pointer;
		border: none;
		font-size: 14px;
	}
	.tab.active {
		background: #91dcff;
	}
	.content {
		display: grid;
		grid-template-columns: 360px 1fr;
		gap: 12px;
		padding: 12px;
		height: 100%;
	}
	.panel {
		border: 1px solid #ffffff;
		border-radius: 8px;
		padding: 10px;
		overflow: auto;
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
		margin: 6px 0;
	}
	.muted {
		color: #ffffff;
		font-size: 14px;
	}
	button.tab {
		background: transparent;
		border: 1px solid #000000;
	}
	button.tab.active {
		border-color: #111;
	}
</style>
