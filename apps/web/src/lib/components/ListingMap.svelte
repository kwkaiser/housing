<script lang="ts">
	import { onMount } from 'svelte';
	import type { MapConfig, Schemas } from '#lib/api/client.ts';
	import type { HousingMap } from '#lib/map/create.ts';
	import LayerPanel from './LayerPanel.svelte';

	let { config, location, title }: { config: MapConfig; location: Schemas['APILocation']; title: string } = $props();

	let el: HTMLDivElement;
	let hm = $state.raw<HousingMap>();

	function mount(createMap: typeof import('#lib/map/create.ts').createMap) {
		const { lat, lng } = location;
		hm = createMap(el, config, { center: [lng, lat], zoom: 15.5 });
		const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#2563eb';
		hm.map.on('load', () => {
			const map = hm!.map;
			map.addSource('pin', { type: 'geojson', data: { type: 'Feature', properties: { title }, geometry: { type: 'Point', coordinates: [lng, lat] } } });
			map.addLayer({
				id: 'pin',
				type: 'circle',
				source: 'pin',
				paint: { 'circle-radius': 8, 'circle-color': accent, 'circle-opacity': 0.6, 'circle-stroke-color': accent, 'circle-stroke-width': 3 }
			});
		});
	}

	onMount(() => {
		let gone = false;
		import('#lib/map/create.ts').then((m) => {
			if (!gone) mount(m.createMap);
		});
		return () => {
			gone = true;
			hm?.map.remove();
		};
	});
</script>

<div class="map-frame">
	<div class="map" bind:this={el}></div>
	{#if hm}<div class="layers"><LayerPanel map={hm} {config} /></div>{/if}
</div>

<style>
	.map-frame {
		height: 22rem;
		max-height: 60vh;
	}

	.layers {
		position: absolute;
		top: 0.6rem;
		left: 0.6rem;
		z-index: 3;
	}
</style>
