<script>
	import "./style.css";
	import { onMount, onDestroy } from "svelte";

	import Tabs from "./components/tabs/Tabs.svelte";
	import HeaderBar from "./components/HeaderBar.svelte";
	import ProjectSelect from "./components/ProjectSelect.svelte";
	import DiffSummary from "./components/push/DiffSummary.svelte";
	import PushPanel from "./components/push/PushPanel.svelte";
	import PullPanel from "./components/pull/PullPanel.svelte";
	import LogViewer from "./components/LogViewer.svelte";

	import { ScanJSON, PendingJSON, DiffJSON, Push, StartWatcherAll, StopWatcherAll, Pull } from "../wailsjs/go/main/App.js";
	import { EventsOn } from "../wailsjs/runtime/runtime.js";
	import { PickRoot, RootStats } from "../wailsjs/go/main/App.js";
	import { logStore } from "./stores/log";

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
	let autoPush = false; 

	function projectSliceFromDiff(full, name) {
		if (!full || !name) return null;

		// Case A: array of per-project entries
		if (Array.isArray(full)) {
			const hit = full.find((x) => (x?.project || x?.name || x?.projectName) === name || x?.project?.name === name);
			return hit || null;
		}

		// Case B: object keyed by name or nested under "projects"
		if (full && typeof full === "object") {
			if (full.projects && full.projects[name]) return full.projects[name];
			if (full[name]) return full[name];
		}

		return null;
	}

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

		await StartWatcherAll(root, autoPush)
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
			// Backend DiffJSON actually takes only (root) and returns all projects
			const res = await DiffJSON(root);
			const full = JSON.parse(res);
			const slice = projectSliceFromDiff(full, selectedProject) || { project: selectedProject, changedCount: 0, files: [] };
			diff = slice;

			// Update pending badge based on slice
			try {
				const changed = !!(slice?.changedCount || slice?.files?.length);
				pending = Array.isArray(pending) ? pending : [];
			} catch {}
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
		if (!watching) {
			await StartWatcherAll(root, autoPush);
			watching = true;
		} else {
			await StopWatcherAll();
			watching = false;
		}
	}

	// Derived state
	$: canPush = !!root && !!selectedProject && commitMsg.trim().length > 0 && commitMsg.length <= 500;

	// Wire events only; do NOT auto-scan on mount
	let offSaved, offPushed;
	onMount(() => {
		offSaved = EventsOn("alsSaved", (p) => {
			const proj = p?.project;
			const file = (p?.path || "").split(/[/\\]/).pop() || "set.als";

			// ➊ Write to the per‑project store that <LogViewer> uses
			if (proj) {
				logStore.info(`Detected save: ${file}`, proj, p);
			}

			// refresh badges
			loadPending();

			// refresh diff if the user is currently looking at this project
			const norm = (s) => (s || "").trim().toLowerCase();
			if (proj && norm(proj) === norm(selectedProject) && (currentTab === "projects" || currentTab === "push")) {
				loadDiff();
			}
		});

		offPushed = EventsOn("pushDone", (p) => {
			const proj = p?.project;
			if (proj) {
				logStore.success(`Auto-pushed ${proj}`, proj);
			}
			log = `Pushed: ${proj}\n` + log;
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
		<div class="panel projects-panel">
			{#if currentTab === "push"}
				<PushPanel bind:commitMsg {canPush} {pending} {pushing} on:push={(e) => doPush(e.detail.message)} />
			{/if}
			{#if currentTab === "pull"}
				<PullPanel {root} />
			{/if}

			{#if currentTab === "projects"}
				<div class="muted projects-row">Projects found:</div>
				<ProjectSelect {projects} bind:selected={selectedProject} onChange={loadDiff} disabled={!root || projects.length === 0} />
				{#if !root}
					<div class="row projects-row">Choose a root to scan.</div>
				{:else if projects.length === 0}
					<div class="row projects-row">No projects found.</div>
				{:else}
					{#each projects as p}
						<!-- <div class="row projects-row">• {p.name} {p.hasPortsy ? "" : "(no .portsy)"}</div> -->
					{/each}
				{/if}
			{/if}
		</div>

		<!-- RIGHT: diff + log -->
		<div class="panel diff-panel">
			{#if currentTab === "push" || currentTab === "projects"}
				<DiffSummary projectName={selectedProject} {diff} onRefresh={loadDiff} disabled={!root || !selectedProject} />
			{/if}
			{#if selectedProject}
				<hr />
				<div class="muted">Event Log</div>
				<LogViewer {selectedProject} />
			{/if}
		</div>
	</div>
</div>
