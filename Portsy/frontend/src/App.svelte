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

	import { ScanJSON, PendingJSON, Push, StartWatcherAll, StopWatcherAll, GetDiffForProject } from "../wailsjs/go/main/App.js";
	import { EventsOn } from "../wailsjs/runtime/runtime.js";
	import { PickRoot, RootStats } from "../wailsjs/go/main/App.js";
	import { logStore } from "./stores/log";
	import { diffs, initDiffBus } from "./stores/diff";
	import { stopWatcherAll } from "./lib/api";
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
	let diff = { changedCount: 0, files: [] };
	let watching = false;
	let commitMsg = "";
	let autoPush = false;
	let scanToken = 0, pendingToken = 0, diffToken = 0;
	$: selectedProjectKey = (selectedProject || "").trim().toLowerCase();
	$: trimmedMsg = (commitMsg || "").trim();

	async function applyAutoPush(v) {
		autoPush = v;
		if (watching && root) {
			try {
				await StopWatcherAll();
				await StartWatcherAll(root, autoPush);
				watching = true;
			} catch (e) {
				watching = false;
				autoPush = !v;
				logStore.error(`Watcher restart failed: ${e.message || e}`);
			}
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
		const text = typeof s === "string" ? s : JSON.stringify(s, null, 2);
		logStore.debug(text, selectedProject || root || "app");
	}

	async function pickRoot() {
		const dir = await PickRoot();
		if (!dir) return;

		// stats check elided
		const wasWatching = watching;
		if (watching) {
			try { await stopWatcherAll(); } catch {}
			watching = false;
		}
		root = dir;
		selectedProject = "";
		diff = [];
		projects = [];
		pending = [];

		await loadProjects();
		await loadPending();

		if (wasWatching) {
			await StartWatcherAll(root, autoPush);
			watching = true;
			currentTab = "projects";
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

	async function safeJSON(res, context) {
		try {
			return JSON.parse(res);
		} catch (e) {
			const msg = `Malformed ${context} JSON`;
			logStore.error(`${msg}: ${e.message || e}`);
			throw new Error(msg);
		}
	}

	async function loadProjects() {
		if (!root) return;
		try {
			show(`Scanning root: ${root}`);
			const token = ++scanToken;
			const raw = await safeJSON(await ScanJSON(root), "scan");
			if (token !== scanToken) return;
			projects = normalizeProjects(raw);
		} catch (e) {
			show(`Scan failed: ${e?.message || e}`);
		}
	}

	async function loadPending() {
		if (!root) return;
		try {
			show(`Pending for: ${root}`);
			const token = ++pendingToken;
			const res = await PendingJSON(root);
			const data = typeof res === 'string' ? JSON.parse(res) : res;
			if (token !== pendingToken) return;
			pending = Array.isArray(data) ? data : [];
			show(data)
		} catch (e) {
			show(`Pending failed: ${e?.message || e}`);
			console.error("PendingJSON error:", e);
		}
	}

	onMount(() => {
		console.log("[App] onMount -> initDiffBus");
		initDiffBus();
		console.log("[App] diff snapshot at mount:", diffs.debugSnapshot());
	});

	async function loadDiff() {
		if (!root || !selectedProject) return;
		try {
			const token = ++diffToken;
			const res = await GetDiffForProject(selectedProject)
			const data = typeof res === 'string' ? JSON.parse(res) : res;
			if (token !== diffToken) return;
			diff = data || { changedCount: 0, files: [] };
		} catch (e) {
			logStore.warn(`Failed to load diff: ${e?.message || e}`, selectedProject);
			diff = { changedCount: 0, files: [] };
		}
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

	async function doPush(message) {
		const msg = message ?? trimmedMsg;
		if (!root || !selectedProject || msg.length === 0 || msg.length > 500) return;

		try {
			const res = await Push(root, selectedProject, msg);
			logStore.success(`Pushed ${selectedProject}`, selectedProject, { message: msg });
			show(res || "pushed");
		} catch (e) {
			logStore.error(`Push failed: ${e.message || e}`, selectedProject);
		} finally {
			commitMsg = "";
			await loadPending();
			await loadDiff();
		}
	}

	async function toggleWatch() {
		if (!root) {
			watching = false;
			return;
		}
		try {
			if (!watching) {
				await StartWatcherAll(root, autoPush);
				watching = true;
			} else {
				await StopWatcherAll();
				watching = false;
			}
		} catch (e) {
			watching = false;
			logStore.error(`Watcher toggle failed: ${e?.message || e}`);
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

	onDestroy(async () => {
		offSaved?.();
		offPushed?.();
		if (watching) {
			try {
				await StopWatcherAll();
			} catch {}
		}
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

	let diffTimer;
	function scheduleLoadDiff() {
		clearTimeout(diffTimer);
		diffTimer = setTimeout(loadDiff, 150);
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
				{#if $selectedProject}
					<PushPanel bind:commitMsg {canPush} {pending} {pushing} on:push={(e) => doPush(e.detail.message)} projectId={$selectedProject.projectId}/>
				{:else}
					<PushPanel bind:commitMsg {canPush} {pending} {pushing} on:push={(e) => doPush(e.detail.message)} />
				{/if}
			{/if}
			{#if currentTab === "pull"}
				<PullPanel {root} />
			{/if}

			{#if currentTab === "projects"}
				<div class="muted projects-row">Projects found:</div>
				<!-- ✅ use Svelte's on:change -->
				<ProjectSelect {projects} bind:selected={selectedProject} on:change={scheduleLoadDiff} disabled={!root || projects.length === 0} />
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
