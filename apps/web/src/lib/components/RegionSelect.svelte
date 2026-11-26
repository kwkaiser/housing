<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import XIcon from '@lucide/svelte/icons/x';

	type Region = Schemas['Region'];

	let { regions, selected = $bindable(), label }: { regions: Region[]; selected: string[]; label: string } = $props();

	let filter = $state('');
	let details = $state<HTMLDetailsElement>();
	let search = $state<HTMLInputElement>();

	let byID = $derived(new Map(regions.map((r) => [r.id, r])));
	let groups = $derived.by(() => {
		const q = filter.trim().toLowerCase();
		const out = new Map<string, Region[]>();
		for (const r of regions) {
			const group = r.group ?? '';
			if (q && !r.name.toLowerCase().includes(q) && !group.toLowerCase().includes(q)) continue;
			out.set(group, [...(out.get(group) ?? []), r]);
		}
		return [...out];
	});
	let summary = $derived.by(() => {
		const names = selected.map((id) => byID.get(id)?.name ?? id);
		if (names.length === 0) return 'Choose regions';
		if (names.length <= 2) return names.join(', ');
		return `${names.slice(0, 2).join(', ')} +${names.length - 2}`;
	});

	function toggle(id: string, on: boolean) {
		selected = on ? [...selected.filter((v) => v !== id), id] : selected.filter((v) => v !== id);
	}

	function toggleGroup(items: Region[], on: boolean) {
		const ids = items.map((r) => r.id);
		selected = on ? [...selected, ...ids.filter((id) => !selected.includes(id))] : selected.filter((id) => !ids.includes(id));
	}
</script>

<div class="region-select">
	<details class="picker" bind:this={details} ontoggle={() => details?.open && search?.focus()}>
		<summary aria-label="{label}: {summary}"><span class="text">{summary}</span></summary>
		<div class="menu">
			<input class="filter" type="search" bind:this={search} bind:value={filter} placeholder="Filter regions" aria-label="Filter regions" />
			{#each groups as [group, items] (group)}
				{@const all = items.every((r) => selected.includes(r.id))}
				<div class="group" role="group" aria-label={group}>
					{#if group}
						<label class="head">
							<input type="checkbox" checked={all} indeterminate={!all && items.some((r) => selected.includes(r.id))} onchange={(e) => toggleGroup(items, e.currentTarget.checked)} />
							{group}
						</label>
					{/if}
					{#each items as r (r.id)}
						<label class="item"><input type="checkbox" checked={selected.includes(r.id)} onchange={(e) => toggle(r.id, e.currentTarget.checked)} /> {r.name}</label>
					{/each}
				</div>
			{:else}
				<p class="empty muted">No regions match “{filter}”.</p>
			{/each}
		</div>
	</details>
	{#if selected.length}
		<ul class="picked">
			{#each selected as id (id)}
				{@const r = byID.get(id)}
				<li>
					<span>{r?.name ?? id}{#if r?.group}<span class="muted"> · {r.group}</span>{/if}</span>
					<button type="button" class="ghost" aria-label="Remove {r?.name ?? id}" onclick={() => toggle(id, false)}><XIcon size={14} /></button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.region-select {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		min-width: 0;
	}

	.picker {
		max-width: 100%;
	}

	summary {
		max-width: 100%;
	}

	summary .text {
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.menu {
		width: min(22rem, calc(100vw - 2rem));
		padding-top: 0;
	}

	.filter {
		position: sticky;
		top: 0;
		z-index: 2;
		width: 100%;
		margin: 0 0 0.25rem;
		border-radius: 0;
		border-width: 0 0 1px;
		background: var(--surface);
	}

	.group + .group {
		border-top: 1px solid var(--line);
		margin-top: 0.25rem;
		padding-top: 0.25rem;
	}

	.head {
		position: sticky;
		top: var(--control);
		z-index: 1;
		background: var(--surface);
		font: var(--label);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
	}

	.item {
		padding-left: 1.75rem !important;
	}

	.empty {
		padding: 0.5rem 0.6rem;
	}

	.picked {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.picked li {
		display: inline-flex;
		align-items: center;
		gap: 0.15rem;
		padding: 0.15rem 0.15rem 0.15rem 0.65rem;
		border: 1px solid var(--accent);
		border-radius: 999px;
		background: var(--accent-soft);
		font-size: 0.85rem;
	}

	.picked button {
		display: inline-grid;
		place-items: center;
		width: 1.6rem;
		min-height: 1.6rem;
		padding: 0;
		border-radius: 50%;
	}
</style>
