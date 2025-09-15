<script>
	// PullPanel: fetch remote projects, let the user pull one into the chosen local root.
	// Safety-first: destructive pulls require an explicit confirmation checkbox.
	import { onMount } from "svelte";
	import { ListRemoteProjects, Pull } from "../../../wailsjs/go/main/App.js";

	// Parent passes the local root folder; pulling is disabled until this is set.
	export let root = "";

	// Local UI state
	let projects = []; // [{ name, last}]
	let selected = ""; // Selected project name
	let commitId = ""; // optional commit override (blank = HEAD)
	let allowDelete = false; // "force" flag; allows deletions when applying pull
	let pulling = false;
	let loading = false;
	let error = "";
	let notice = "";

	// Async token guards prevent stale responses from overwriting newer state
	let loadToken = 0;

	// Normalize remote project shape (handles mixed casing from Go/Firestore)
	const normalize = (list) => (Array.isArray(list) ? list : []).map((p) => ({ name: p.name || p.Name || "", last: p.lastCommitId || p.LastCommitID || "" })).filter((p) => p.name);

	// Cross-platform join:
	// - Prefer '/' for UI display; Windows accepts '/' as well as '\'
	// - Avoid duplicating separators
	function joinPath(a, b) {
		const A = String(a || "");
		const B = String(b || "");
		if (!A) return B;
		if (!B) return A;
		const aTrim = A.replace(/[\\/]+$/g, "");
		const bTrim = B.replace(/[\\/]+$/g, "");
		return `${aTrim}/${bTrim}`;
	}

	async function loadProjects() {
		loading = true;
		error = "";
		notice = "";
		error = "";
		notice = "";
		const token = ++loadToken;
		try {
			const res = await ListRemoteProjects();
			if (token !== loadToken) return; // stale response; user reloaded again
			projects = normalize(res);
			// Auto-select first project if nothing chosen yet
			if (!selected && projects.length) selected = projects[0].name;
		} catch (e) {
			if (token !== loadToken) return;
			error = e?.message || String(e);
			projects = [];
		} finally {
			if (token === loadToken) loading = false;
		}
	}

	// Very light validation: allow empty (means HEAD), otherwise expect hex-ish id
	function isValidCommitId(s) {
		if (!s) return true; // HEAD
		const id = String(s).trim();
		return /^[a-fA-F0-9]{6,64}$/.test(id); // tolerate short shas
	}

	async function doPull() {
		if (!selected || !root || pulling) return;
		// Guard: if detructive, require explicit confirmation
		if (allowDelete) {
			const ok = confirm(`This pull may delete local files in:\n\n${joinPath(root, selected)}\n\n` + `Check your backups and ensure you selected the right project.\n\nProceed?`);
			if (!ok) return;
		}

		const dest = joinPath(root, selected); // Destination under chosen root
		pulling = true;
		error = "";
		notice = "";

		try {
			// Wails signature: Pull(project, dest, commit, force)
			// Empty commitId means "pull HEAD"
			const commitArg = commitId.trim() || "";
			if (!isValidCommitId(commitArg)) {
				throw new Error("Commit ID looks invalid. Use a hex hash or leave blank for HEAD.");
			}

			await Pull(selected, dest, commitArg, allowDelete);
			notice = `Pulled ${selected} -> ${dest} ✓`;
		} catch (e) {
			error = e?.message || String(e);
		} finally {
			pulling = false;
		}
	}

	onMount(loadProjects);

	// Derived flags for button disabled states
	$: canPull = !!root && !!selected && !pulling && isValidCommitId(commitId);
</script>

<div class="panel">
	<div class="spread">
		<h3>Pull</h3>
	</div>

	<!-- Error / notice banners -->
	{#if error}<p class="label">Error: {error}</p>{/if}
	{#if notice}<p class="label">{notice}</p>{/if}

	<!-- Project chooser -->
	<div class="row">
		<select class="select" bind:value={selected} disabled={loading || pulling || projects.length === 0} aria-label="Select remote project to pull">
			<option value="">
				{projects.length ? "Select remote project…"
				: loading ? "Loading…"
				: "No remote projects found"}
			</option>
			{#each projects as p (p.name)}
				<option value={p.name}>
					{p.name}{#if p.last}
						— HEAD: {p.last}{/if}
				</option>
			{/each}
		</select>
	</div>

	<!-- Commit override input (blank = HEAD) -->
	<div class="row" style="margin-top:8px;">
		<input
			class="input"
			type="text"
			placeholder="Commit (leave blank for HEAD)"
			bind:value={commitId}
			disabled={pulling || !selected}
			aria-invalid={!isValidCommitId(commitId)}
			title="Optional: specify a commit hash to pull; leave blank to pull HEAD"
			style="width:100%;"
		/>
	</div>
	{#if commitId && !isValidCommitId(commitId)}
		<p class="label">Commit ID should be hex (6–64 chars), or leave blank for HEAD.</p>
	{/if}

	<!-- Destination preview -->
	<p class="label" style="margin-top:8px;">
		Destination: {root ? `${joinPath(root, selected || "(project)")}` : "(choose a root in the header first)"}
	</p>

	<!-- Destructive toggle -->
	<label class="row" style="margin-top:4px;">
		<input type="checkbox" bind:checked={allowDelete} disabled={pulling} aria-label="Allow deletions when pulling" />
		<span>I understand this may delete local files (force pull)</span>
	</label>

	<!-- Actions -->
	<div class="row pull-refresh" style="margin-top:8px;">
		<button class="btn pull-refresh-btn" on:click={doPull} disabled={!canPull}>
			{pulling ? "Pulling…" : "Pull"}
		</button>
		<button class="btn pull-refresh-btn" on:click={loadProjects} disabled={loading || pulling}>
			{loading ? "Loading…" : "Refresh"}
		</button>
	</div>
</div>
