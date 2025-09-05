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

	import { ScanJSON, PendingJSON, DiffJSON, Push, StartWatcherAll, StopWatcherAll, Pull, GetDiffForProject } from "../wailsjs/go/main/App.js";
	import { EventsOn } from "../wailsjs/runtime/runtime.js";
	import { PickRoot, RootStats } from "../wailsjs/go/main/App.js";
	import { logStore } from "./stores/log";
	import { diffs, initDiffBus } from "./stores/diff";

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

	async function applyAutoPush(v) {
		autoPush = v;
		if (watching && root) {
			// ✅ remove the ':' after await
			await StopWatcherAll();
			await StartWatcherAll(root, autoPush);
		}
	}

	function baseName(s) {
		if (!s) return "";
		const parts = String(s).split(/[/\\]/);
		return parts[parts.length - 1];
	}

	function sameProjectKey(a, b) {
		if (!a || !b) return false;
		a = String(a).trim().toLowerCase();
		b = String(b).trim().toLowerCase();
		return a === b || baseName(a) === baseName(b);
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

		await StartWatcherAll(root, autoPush);
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

	onMount(() => {
		console.log('[App] onMount -> initDiffBus');
		initDiffBus();
		console.log('[App] diff snapshot at mount:', diffs.debugSnapshot());
	});

	async function loadDiff() {
		if (!root || !selectedProject) return;
		const res = await GetDiffForProject(selectedProject);
		diff = JSON.parse(res);
	}

	function projectSliceFromAny(full, name) {
		if (!full) return null;
		if (Array.isArray(full)) {
			return full.find((x) => sameProjectKey(x?.project ?? x?.name ?? x?.projectName, name) || sameProjectKey(x?.projectPath ?? x?.path, name)) || null;
		}

		if (full.projects && typeof full.projects === "object") {
			for (const [k, v] of Object.entries(full.projects)) {
				if (sameProjectKey(k, name) || sameProjectKey(v?.project ?? v?.name, name) || sameProjectKey(v?.projectPath ?? v?.path, name)) return v;
			}
		}

		if (typeof full === "object") {
			for (const [k, v] of Object.entries(full)) {
				if (k === "projects") continue;
				if (sameProjectKey(k, name)) return v;
			}
		}
		return null;
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
		offSaved = EventsOn("alsSaved", async (p) => {
			const proj = p?.project;
			const file = (p?.path || "").split(/[/\\]/).pop() || "set.als";
			if (proj) {
				logStore.info(`Detected save: ${file}`, proj, p);
				if (p?.summary) {
					logStore.info(`Changes: ${p.summary}`, proj);
				} else {
					try {
						const res = await GetDiffForProject(proj);
						const d = JSON.parse(res);
						if (d.changedCount > 0) {
							const head = d.files
								.slice(0, 6)
								.map((f) => `${f.status}: ${f.path}`)
								.join(", ");
							const more = d.files.length > 6 ? ` (+${d.files.length - 6} more)` : "";
							logStore.info(`Changes: ${head}${more}`, proj, d);
						}
					} catch {}
				}
			}
			loadPending();
			if (proj && proj.toLowerCase() === (selectedProject || "").toLowerCase() && (currentTab === "projects" || currentTab === "push")) {
				await loadDiff();
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

	function normalizeProjectDiff(slice, projectName) {
		// Empty/unknown → canonical empty
		if (!slice || typeof slice !== "object") {
			return { project: projectName, changedCount: 0, files: [] };
		}

		// Already in UI shape?
		if (Array.isArray(slice.files) || typeof slice.changedCount === "number") {
			const files = Array.isArray(slice.files) ? slice.files : [];
			const changedCount = typeof slice.changedCount === "number" ? slice.changedCount : files.length;
			return { project: projectName, changedCount, files };
		}

		// Common CLI shapes
		const files = [];
		const pushAll = (list, status) => {
			if (!Array.isArray(list)) return;
			for (const it of list) {
				const p = it?.path ?? it?.Path ?? it;
				if (p) files.push({ path: String(p), status });
			}
		};
		pushAll(slice.Added || slice.added, "added");
		pushAll(slice.Changed || slice.changed, "modified");
		pushAll(slice.Removed || slice.removed, "deleted");

		// Optional ALS logical hint → ensure we at least show the set
		if (files.length === 0 && (slice.Logical?.ALSChanged || slice.logical?.alsChanged)) {
			const alsPath = slice.Logical?.ALSPath || slice.logical?.alsPath || "set.als";
			files.push({ path: alsPath, status: "modified" });
		}

		return { project: projectName, changedCount: files.length, files };
	}
</script>

<div class="shell">
	<HeaderBar {root} onScan={loadProjects} onPending={loadPending} {watching} {toggleWatch} {pickRoot} {autoPush} onToggleAutoPush={applyAutoPush} />

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
				<!-- ✅ use Svelte's on:change -->
				<ProjectSelect {projects} bind:selected={selectedProject} on:change={loadDiff} disabled={!root || projects.length === 0} />
				{#if !root}
					<div class="row projects-row">Choose a root to scan.</div>
				{:else if projects.length === 0}
					<div class="row projects-row">No projects found.</div>
				{:else}
					{#each projects as p}
						<!-- <div class="row projects-row">• {p.name} {p.hasPortsy ? "" : "(no .portsy)"} </div> -->
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
