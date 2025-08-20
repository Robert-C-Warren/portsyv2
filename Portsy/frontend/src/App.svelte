<script>
	import "./style.css";
	import { onMount, onDestroy } from "svelte";

	import Tabs from "./components/Tabs.svelte";
	import HeaderBar from "./components/HeaderBar.svelte";
	import ProjectSelect from "./components/ProjectSelect.svelte";
	import PushPanel from "./components/PushPanel.svelte";
	import DiffSummary from "./components/DiffSummary.svelte";
	import PullPanel from './components/pull/PullPanel.svelte'

	import { ScanJSON, PendingJSON, DiffJSON, Push, StartWatcherAll, StopWatcherAll, Pull } from "../wailsjs/go/main/App.js";
	import { EventsOn } from "../wailsjs/runtime/runtime.js";
	import { PickRoot, RootStats } from "../wailsjs/go/main/App.js";

	const TABS = [
		{ id: "projects", label: "Projects" },
		{ id: "push", label: "Push" },
		{ id: "pull", label: "Pull" },
		{ id: "rollback", label: "Rollback" },
	];

	// nothing is pre-filled or pre-selected
	let root = ""; // chosen by user via picker
	let currentTab = ""; // chosen by user via click
	let log = "";
	let projects = [];
	let pending = [];
	let pushing = false;
	let selectedProject = ""; // chosen by user from dropdown
	let diff = [];
	let watching = false;
	let commitMsg = "";

	function show(s) {
		log = typeof s === "string" ? s : JSON.stringify(s, null, 2);
	}

	async function pickRoot() {
		const dir = await PickRoot();
		if (!dir) return;

		// Preflight: ask before scanning very large roots or drive roots
		try {
			const stats = await RootStats(dir); // { dirCount, isDriveRoot }
			const LIMIT = 800; // tweak to taste; we only scan first level anyway

			if (stats.isDriveRoot || stats.dirCount > LIMIT) {
				const ok = confirm(`That folder looks large (${stats.dirCount} subfolders${stats.isDriveRoot ? ", drive root" : ""}). ` + `Portsy scans only the first level for Ableton projects. Proceed?`);
				if (!ok) return;
			}
		} catch (e) {
			// If stats fails, we can still proceed—the scanner is first-level only.
			console.warn("RootStats failed:", e);
		}

		// Accept the root + auto-scan
		root = dir;
		selectedProject = "";
		diff = [];
		projects = [];
		pending = [];

		await loadProjects();
		await loadPending();

		// If watching was on previously, require the user to toggle it again (stays off for safety)
		if (watching) {
			await StopWatcherAll();
			watching = false;
		}
	}

	function normalizeProjects(raw) {
		const arr = Array.isArray(raw) ? raw : [];
		return arr
			.map((p) => ({
				name: p.name ?? p.Name ?? "",
				path: p.path ?? p.Path ?? "",
				hasPortsy: p.hasPortsy ?? p.HasPortsy ?? false,
			}))
			.filter((p) => p.name || p.path);
	}

	async function loadProjects() {
		if (!root) return;
		try {
			show(`Scanning root: ${root}`);
			const res = await ScanJSON(root); // <- Go method via Wails
			const raw = JSON.parse(res);
			projects = normalizeProjects(raw);
			show(res);
		} catch (e) {
			const msg = e?.message || String(e);
			show(`Scan failed: ${msg}`);
			console.error("ScanJSON error:", e);
		}
	}

	async function loadPending() {
		if (!root) return;
		try {
			show(`Pending for: ${root}`);
			const res = await PendingJSON(root);
			pending = JSON.parse(res);
			show(res);
		} catch (e) {
			show(`Pending failed: ${e?.message || e}`);
			console.error("PendingJSON error:", e);
		}
	}

	async function loadDiff() {
		if (!root || !selectedProject) return;
		try {
			show(`Diff: ${selectedProject}`);
			const res = await DiffJSON(root, selectedProject);
			diff = JSON.parse(res);
			show(res);
		} catch (e) {
			show(`Diff failed: ${e?.message || e}`);
			console.error("DiffJSON error:", e);
		}
	}

	async function doPush() {
		if (!canPush) return;
		const res = await Push(root, selectedProject, commitMsg);
		show(res || "pushed");
		commitMsg = "";
		await loadPending();
		await loadDiff();
	}

	async function toggleWatch() {
		if (!root) {
			watching = false;
			return;
		}
		if (watching) await StartWatcherAll(root, true);
		else await StopWatcherAll();
	}

	// Derived state
	$: canPush = !!root && !!selectedProject && commitMsg.trim().length > 0 && commitMsg.length <= 500;

	// Wire events only; do NOT auto-scan on mount
	let offSaved, offPushed;
	onMount(() => {
		offSaved = EventsOn("alsSaved", (p) => {
			log = `Saved: ${p.project} @ ${p.at}\n` + log;
			loadPending();
		});
		offPushed = EventsOn("pushDone", (p) => {
			log = `Pushed: ${p.project}\n` + log;
			loadPending();
			loadDiff();
		});
	});
	onDestroy(() => {
		offSaved?.();
		offPushed?.();
	});
</script>

<div class="shell">
	<HeaderBar {root} onScan={loadProjects} onPending={loadPending} {watching} {toggleWatch} {pickRoot} />

	<!-- no default tab; user clicks one -->
	<Tabs bind:value={currentTab} tabs={TABS} />

	<div class="content">
		<!-- LEFT: controls -->
		<div class="panel">
			<ProjectSelect {projects} bind:selected={selectedProject} onChange={loadDiff} disabled={!root || projects.length === 0} />

			{#if currentTab === "push"}
				<PushPanel bind:commitMsg {canPush} {pending} {pushing} on:push={(e) => doPush(e.detail.message)} />
			{/if}
			{#if currentTab === "pull"}
				<PullPanel />
			{/if}

			{#if currentTab === "projects"}
				<div class="muted">Projects found:</div>
				{#if !root}
					<div class="row">Choose a root to scan.</div>
				{:else if projects.length === 0}
					<div class="row">No projects found.</div>
				{:else}
					{#each projects as p}
						<div class="row">• {p.name} {p.hasPortsy ? "" : "(no .portsy)"}</div>
						<option value={p.name}>{p.name}</option>
					{/each}
				{/if}
			{/if}

			<!-- TODO: Pull / Rollback panes -->
		</div>

		<!-- RIGHT: diff + log -->
		<div class="panel">
			{#if currentTab === "push" || currentTab === "projects"}
				<DiffSummary projectName={selectedProject} {diff} onRefresh={loadDiff} disabled={!root || !selectedProject} />
			{/if}
			<hr />
			<div class="muted">Event log</div>
			<pre>{log}</pre>
		</div>
	</div>
</div>
