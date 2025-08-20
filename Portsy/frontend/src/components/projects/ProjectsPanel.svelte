<script>
	import { listRemoteProjects, getCommitHistory } from "src/lib/api";
	let loading = true,
		error = "",
		projects = [],
		selected = "",
		commits = [];
	const load = async () => {
		loading = true;
		error = "";
		commits = [];
		try {
			projects = await listRemoteProjects();
		} catch (e) {
			error = e.message;
		} finally {
			loading = false;
		}
	};
	const pick = async (name) => {
		selected = name;
		commits = [];
		try {
			commits = await getCommitHistory(name, 5);
		} catch {}
	};
	$: load(); // initial
</script>

<div class="panel">
	<div class="spread">
		<h3>Projects</h3>
		<button class="btn" on:click={load}>Refresh</button>
	</div>
	{#if loading}<p class="label">Loading…</p>{/if}
	{#if error}<p class="label">Error: {error}</p>{/if}
	<ul class="list">
		{#each projects as p}
			<li class="item" on:click={() => pick(p.name || p.Name)}>
				<div class="spread">
					<strong>{p.name || p.Name}</strong>
					{#if p.lastCommitId || p.LastCommitID}<span class="badge">HEAD: {p.lastCommitId || p.LastCommitID}</span>{/if}
				</div>
			</li>
		{/each}
	</ul>
</div>
