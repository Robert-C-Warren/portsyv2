<script>
	export let tabs = [];
	export let value;
</script>

<div class="tabs">
	{#each tabs as t}
		<button type="button" class="tab {value === t.id ? 'active' : ''}" on:click={() => (value = t.id)}>
			{t.label}
		</button>
	{/each}
</div>
