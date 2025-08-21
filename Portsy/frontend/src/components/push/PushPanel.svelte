<script>
	import { createEventDispatcher } from "svelte";
	export let commitMsg = "";
	export let canPush = false;
	export let pending = [];
	export let pushing = false;
	const dispatch = createEventDispatcher();
	const count = (arr) => (Array.isArray(arr) ? arr.length : 0);
	function doPush() {
		if (!canPush || pushing) return;
		if ((commitMsg || "").length > 500) return;
		dispatch("push", { message: commitMsg.trim() || "Update" });
	}
</script>

<div>
	<div class="spread" style="margin-bottom:8px;">
		<div class="badge changed-badge">{count(pending)} project(s) changed</div>
	</div>
	<input class="input" bind:value={commitMsg} maxlength="500" placeholder="What changed?" on:keydown={(e) => (e.ctrlKey || e.metaKey) && e.key === "Enter" && doPush()} />
	<div class="label commit-guide">Commit message (≤ 500 chars)</div>
	<div class="row pull-refresh" style="margin-top:10px;">
		<button class="btn pull-refresh-btn" on:click={doPush} disabled={!canPush || pushing}>
			{pushing ? "Pushing…" : "Push"}
		</button>
	</div>
	{#if count(pending) > 0}
		<hr />
		<div class="label">Projects with local changes</div>
		<ul class="list" style="max-height:180px; overflow:auto;">
			{#each pending as p}
				<li class="item">
					<div class="spread">
						<span>{p.name || p.Name}</span>
						{#if p.stats || p.Stats}
							<span class="badge">
								+{(p.stats?.added ?? p.Stats?.Added) || 0}
								~{(p.stats?.changed ?? p.Stats?.Changed) || 0}
								-{(p.stats?.removed ?? p.Stats?.Removed) || 0}
							</span>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
