<script lang="ts">
	import { untrack } from 'svelte';
	import type { MapConfig } from '#lib/api/client.ts';
	import type { HousingMap } from '#lib/map/create.ts';
	import { availableOverlays, overlayLabels, type Base, type OverlayName } from '#lib/map/style.ts';

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

<details class="picker layer-panel">
	<summary>Layers</summary>
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
		{/each}
	</div>
</details>
