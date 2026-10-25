<script lang="ts">
	import { untrack } from 'svelte';
	import type { MapConfig } from '#lib/api/client.ts';
	import type { HousingMap } from '#lib/map/create.ts';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import { availableOverlays, noiseLegend, overlayLabels, type Base, type OverlayName } from '#lib/map/style.ts';

	let { map, config }: { map: HousingMap; config: MapConfig } = $props();

	let base = $state<Base>(untrack(() => map.layers.base));
	let on = $state<OverlayName[]>(untrack(() => [...map.layers.overlays]));

	function update() {
		map.setLayers({ base, overlays: [...on] });
	}

	function toggle(name: OverlayName, checked: boolean) {
		on = checked ? [...on, name] : on.filter((n) => n !== name);
		update();
	}
</script>

<details class="picker">
	<summary><LayersIcon size={16} /> <span class="text">Layers</span></summary>
	<div class="menu">
		{#if config.positron}
			<label><input type="radio" name="base" value="positron" bind:group={base} onchange={update} /> Positron</label>
		{/if}
		<label><input type="radio" name="base" value="osm" bind:group={base} onchange={update} /> OpenStreetMap</label>
		<hr />
		{#each availableOverlays(config) as name (name)}
			<label>
				<input type="checkbox" checked={on.includes(name)} onchange={(e) => toggle(name, e.currentTarget.checked)} />
				{overlayLabels[name]}
			</label>
			{#if name === 'noise' && on.includes('noise')}
				<div class="legend" aria-label="Noise level in dBA">
					{#each noiseLegend as [color, db] (db)}
						<span><i style:background={color}></i>{db}</span>
					{/each}
					<span class="unit">dBA</span>
				</div>
			{/if}
		{/each}
	</div>
</details>

<style>
	summary {
		box-shadow: var(--shadow-lg);
	}

	.legend {
		display: flex;
		flex-wrap: wrap;
		gap: 0.25rem 0.5rem;
		padding: 0 0 0.25rem 1.5rem;
		font-size: 0.75rem;
	}

	.legend span {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
	}

	.legend i {
		width: 0.6rem;
		height: 0.6rem;
		border-radius: 2px;
		opacity: 0.8;
	}

	.unit {
		color: var(--muted);
	}

	@media (width < 640px) {
		.text {
			display: none;
		}
	}
</style>
