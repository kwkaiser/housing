<script lang="ts" module>
	let camera: { center: [number, number]; zoom: number } | undefined;
</script>

<script lang="ts">
	import { mount as attach, onMount, unmount, untrack } from 'svelte';
	import type { GeoJSONSource, MapLayerMouseEvent, Popup } from 'maplibre-gl';
	import SlidersHorizontalIcon from '@lucide/svelte/icons/sliders-horizontal';
	import { goto } from '$app/navigation';
	import { api, call, type MapPin, type PlaceMatch } from '#lib/api/client.ts';
	import LayerPanel from '#lib/components/LayerPanel.svelte';
	import PinPreview, { gallery } from '#lib/components/PinPreview.svelte';
	import PlaceConfirm from '#lib/components/PlaceConfirm.svelte';
	import PlaceSearch from '#lib/components/PlaceSearch.svelte';
	import Sheet from '#lib/components/Sheet.svelte';
	import type { HousingMap } from '#lib/map/create.ts';
	import { match, money } from '#lib/format.ts';
	import RangeSelect from '#lib/components/RangeSelect.svelte';
	import { load, save } from '#lib/storage.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let pins = $derived(data.pins);

	$effect(() => {
		const fresh = data.fresh;
		if (!fresh) return;
		let live = true;
		fresh.then((p) => live && (pins = p)).catch(() => undefined);
		return () => {
			live = false;
		};
	});

	const UNGRADED = '#78716c';
	type Fading = '' | 'age' | 'match';
	type Placed = { pin: MapPin; lat: number; lng: number };

	let collections = $derived(pins.collections ?? []);
	let colors = $derived(Object.fromEntries(collections.map((c) => [c.id, c.color])));
	let chosen = $state<string>(load('mapCollection', ''));
	let selected = $derived(collections.some((c) => c.id === chosen) ? chosen : (collections[0]?.id ?? ''));
	let fading = $state<Fading>(load<Fading>('mapFading', 'age'));
	let contrast = $state<number>(Number(load('mapContrast', 50)));
	let show = $state<{ dealbreakers?: boolean; ungraded?: boolean; hidden?: boolean }>(load('mapShow', {}));
	let cutoff = $state<number>(Number(load('mapCutoff', 0)) || 0);
	let filtersOpen = $state(false);

	let placed = $derived.by((): Placed[] => {
		const out = (pins.pins ?? []).map((pin) => ({ pin, lat: pin.lat, lng: pin.lng }));
		const groups = new Map<string, Placed[]>();
		for (const p of out) {
			const key = `${p.lat},${p.lng}`;
			groups.set(key, [...(groups.get(key) ?? []), p]);
		}
		for (const group of groups.values()) {
			if (group.length < 2) continue;
			const r = 0.0012 * Math.sqrt(group.length);
			group.forEach((p, i) => {
				const a = (2 * Math.PI * i) / group.length;
				p.lat += r * Math.sin(a);
				p.lng += (r * Math.cos(a)) / Math.cos((p.lat * Math.PI) / 180);
			});
		}
		return out;
	});

	function lead(pin: MapPin): string | null {
		const c = selected;
		if (!(pin.collections ?? []).includes(c)) return null;
		const g = pin.grades[c];
		if (!show.ungraded && !g) return null;
		if (!show.hidden && pin.hidden) return null;
		if (!show.dealbreakers && g?.dealbreakers?.length) return null;
		if (g && cutoff > 0 && g.match < cutoff) return null;
		return c;
	}

	let visible = $derived(placed.map((p) => ({ ...p, lead: lead(p.pin) })).filter((p) => p.lead !== null) as (Placed & { lead: string })[]);

	const FLOOR = 0.2;
	const clamp = (n: number) => Math.max(0, Math.min(1, n));

	let features = $derived.by(() => {
		const start = Date.parse(data.from || pins.first_day || '');
		const end = data.to ? Date.parse(data.to) : Date.now();
		const level = (p: (typeof visible)[number]): number | null => {
			if (fading === 'match') {
				const g = p.pin.grades[p.lead];
				return g ? clamp(g.match / 100) : null;
			}
			if (fading === 'age') {
				if (!Number.isFinite(start) || end <= start) return 1;
				return clamp((Date.parse(p.pin.first_seen) - start) / (end - start));
			}
			return 1;
		};
		const gamma = 0.5 + 3.5 * (contrast / 100);
		return visible.map((p, i) => {
			const l = level(p);
			const s = (!fading ? 1 : l === null ? 0.5 : FLOOR + (1 - FLOOR) * l ** gamma) * (p.pin.hidden ? 0.4 : 1);
			const g = p.pin.grades[p.lead];
			const hollow = Boolean(g?.dealbreakers?.length);
			return {
				type: 'Feature' as const,
				id: i,
				properties: {
					i: pins.pins!.indexOf(p.pin),
					lng: p.lng,
					lat: p.lat,
					lead: p.lead,
					color: g ? colors[p.lead] : UNGRADED,
					opacity: s,
					fill: hollow ? 0 : (p.pin.approx ? 0.2 : 0.6) * s,
					radius: fading ? 4 + 5 * s : 7,
					sort: s
				},
				geometry: { type: 'Point' as const, coordinates: [p.lng, p.lat] }
			};
		});
	});

	let summary = $derived.by(() => {
		const shown = (pins.pins ?? []).filter((p) => (p.collections ?? []).includes(selected));
		const parts = [`${shown.length} listings placed`];
		const approx = shown.filter((p) => p.approx).length;
		const ungraded = shown.filter((p) => p.ungraded).length;
		const dealbreakers = shown.filter((p) => p.grades[selected]?.dealbreakers?.length).length;
		const hidden = shown.filter((p) => p.hidden).length;
		if (approx) parts.push(`${approx} approximate from zip code`);
		if (dealbreakers) parts.push(`${dealbreakers} with dealbreakers`);
		if (hidden) parts.push(`${hidden} hidden`);
		if (ungraded) parts.push(`${ungraded} ungraded`);
		if (pins.unplaced) parts.push(`${pins.unplaced} without a location across all collections`);
		return parts.join(' · ');
	});


	function range(from: string, to: string) {
		const q = new URLSearchParams({ from });
		if (to) q.set('to', to);
		goto(`/map?${q}`);
	}

	let el: HTMLDivElement;
	let hm = $state.raw<HousingMap>();
	let lib: typeof import('#lib/map/create.ts');

	function fit() {
		const map = hm?.map;
		if (!map) return;
		const bounds = new lib.LngLatBounds();
		for (const p of visible) bounds.extend([p.lng, p.lat]);
		if (bounds.isEmpty()) {
			const on = collections.filter((c) => c.id === selected && c.area);
			for (const c of on.length ? on : collections.filter((c) => c.area)) {
				bounds.extend([c.area!.west, c.area!.south]);
				bounds.extend([c.area!.east, c.area!.north]);
			}
		}
		if (bounds.isEmpty()) map.jumpTo({ center: [-98.6, 39.8], zoom: 3 });
		else map.fitBounds(bounds, { padding: { top: 80, bottom: 40, left: 40, right: 60 }, maxZoom: 15, duration: 0 });
	}

	function mount() {
		hm = lib.createMap(el, data.config, camera ?? {});
		const map = hm.map;
		const tip = new lib.Popup({ closeButton: false, closeOnClick: false, offset: 10 });
		map.on('load', () => {
			map.addSource('pins', { type: 'geojson', data: { type: 'FeatureCollection', features: untrack(() => features) } });
			map.addLayer({
				id: 'pins',
				type: 'circle',
				source: 'pins',
				layout: { 'circle-sort-key': ['get', 'sort'] },
				paint: {
					'circle-radius': ['get', 'radius'],
					'circle-color': ['get', 'color'],
					'circle-opacity': ['get', 'fill'],
					'circle-stroke-color': ['get', 'color'],
					'circle-stroke-opacity': ['get', 'opacity'],
					'circle-stroke-width': 2
				}
			});
			map.addLayer({
				id: 'pins-hit',
				type: 'circle',
				source: 'pins',
				paint: { 'circle-radius': matchMedia('(pointer: coarse)').matches ? 22 : 12, 'circle-opacity': 0 }
			});
			if (!camera) untrack(fit);
			map.on('moveend', () => (camera = { center: map.getCenter().toArray(), zoom: map.getZoom() }));
		});
		const pinAt = (e: MapLayerMouseEvent) => {
			const hits = (e.features ?? []).map((f) => {
				const p = f.properties as { i: number; lead: string; lng: number; lat: number };
				const at: [number, number] = [p.lng, p.lat];
				return { pin: pins.pins![p.i], lead: p.lead, at, dist: map.project(at).dist(e.point) };
			});
			return hits.length ? hits.reduce((a, b) => (b.dist < a.dist ? b : a)) : null;
		};
		map.on('mousemove', 'pins-hit', (e) => {
			const hit = pinAt(e);
			if (!hit) return;
			gallery(hit.pin);
			map.getCanvas().style.cursor = 'pointer';
			const g = hit.pin.grades[hit.lead];
			tip
				.setLngLat(e.lngLat)
				.setText([money(hit.pin.price_cents, hit.pin.offer), g ? match(g.match, g.calibrated, g.stale) : 'ungraded', g?.dealbreakers?.length ? 'dealbreaker' : null, hit.pin.hidden ? 'hidden' : null, hit.pin.first_seen].filter(Boolean).join(' · '))
				.addTo(map);
		});
		map.on('mouseleave', 'pins-hit', () => {
			map.getCanvas().style.cursor = '';
			tip.remove();
		});
		map.on('click', 'pins-hit', (e) => {
			const hit = pinAt(e);
			if (!hit) return;
			tip.remove();
			const box = document.createElement('div');
			const pop = new lib.Popup({ maxWidth: '280px', className: 'pin-popup' }).setLngLat(hit.at).setDOMContent(box);
			const onhidden = (on: boolean) => {
				hit.pin.hidden = on;
				pins = { ...pins };
				if (on && !show.hidden) pop.remove();
			};
			const preview = attach(PinPreview, { target: box, props: { pin: hit.pin, lead: hit.lead, onhidden } });
			pop.on('close', () => unmount(preview));
			pop.addTo(map);
		});
	}

	onMount(() => {
		let gone = false;
		import('#lib/map/create.ts').then((m) => {
			if (gone) return;
			lib = m;
			mount();
		});
		return () => {
			gone = true;
			hm?.map.remove();
		};
	});

	$effect(() => {
		const fc = { type: 'FeatureCollection' as const, features };
		hm?.map.getSource<GeoJSONSource>('pins')?.setData(fc);
	});

	let proposal: Popup | undefined;

	function near() {
		const c = hm?.map.getCenter();
		return c && { lat: c.lat, lng: c.lng };
	}

	async function addPlace(m: PlaceMatch, name: string) {
		await call(api().POST('/api/v1/places', { body: { name, address: m.address, lat: m.lat, lng: m.lng } }));
		if (!hm) return;
		const on = hm.layers.overlays;
		if (on.includes('places')) hm.places.refresh();
		else hm.setLayers({ ...hm.layers, overlays: [...on, 'places'] });
	}

	function propose(m: PlaceMatch) {
		const map = hm?.map;
		if (!map) return;
		proposal?.remove();
		const at: [number, number] = [m.lng, m.lat];
		map.flyTo({ center: at, zoom: Math.max(map.getZoom(), 16) });
		hm!.places.draft(at);
		const box = document.createElement('div');
		const pop = new lib.Popup({ maxWidth: '300px', className: 'place-popup', offset: 12 }).setLngLat(at).setDOMContent(box);
		const view = attach(PlaceConfirm, {
			target: box,
			props: {
				match: m,
				oncancel: () => pop.remove(),
				onsave: async (name: string) => {
					await addPlace(m, name);
					pop.remove();
				}
			}
		});
		pop.on('close', () => {
			unmount(view);
			if (proposal !== pop) return;
			proposal = undefined;
			hm?.places.draft(null);
		});
		proposal = pop.addTo(map);
	}

	function choose(id: string) {
		chosen = id;
		save('mapCollection', id);
		queueMicrotask(fit);
	}
</script>

<svelte:head><title>Map · housing</title></svelte:head>

<div class="map-page">
	<h1 class="sr-only">Map</h1>
	<div class="map-frame">
		<div class="map" bind:this={el}></div>
	</div>
	<div class="overlay">
		{#if collections.length}
			<details class="picker">
				<summary><span class="swatch" style:background={colors[selected]}></span> {selected} <span class="muted">{visible.length}</span></summary>
				<div class="menu">
					{#each collections as c (c.id)}
						<label>
							<input type="radio" name="map-collection" checked={c.id === selected} onchange={() => choose(c.id)} />
							<span class="swatch" style:background={c.color}></span>
							{c.id} <span class="muted">{c.graded}{c.latest_day ? ` · ${c.latest_day}` : ''}</span>
						</label>
					{/each}
				</div>
			</details>
			<button type="button" onclick={() => (filtersOpen = true)}><SlidersHorizontalIcon size={16} /> {cutoff ? `≥${cutoff}%` : 'Filters'}</button>
		{/if}
		{#if hm}
			<PlaceSearch {near} onpick={propose} />
			<LayerPanel map={hm} config={data.config} />
		{/if}
	</div>
	{#if !collections.length}
		<div class="card empty">
			<p>There are no collections yet.</p>
			<a class="button primary" href="/collections/new">Create a collection</a>
		</div>
	{/if}
</div>

<Sheet bind:open={filtersOpen} title="Map filters" done>
	<p class="summary-line">{summary}</p>
	<h3 class="label">Dates</h3>
	<div class="option-list"><div><RangeSelect from={data.from} to={data.to} onrange={range} /></div></div>
	<h3 class="label">Listings</h3>
	<div class="option-list">
		<label>Match cutoff <span><input type="number" min="0" step="5" inputmode="numeric" placeholder="0" bind:value={cutoff} oninput={() => save('mapCutoff', cutoff || 0)} /> %</span></label>
		<label>Show dealbreakers <input type="checkbox" bind:checked={show.dealbreakers} onchange={() => save('mapShow', show)} /></label>
		<label>Show ungraded <input type="checkbox" bind:checked={show.ungraded} onchange={() => save('mapShow', show)} /></label>
		<label>Show hidden <input type="checkbox" bind:checked={show.hidden} onchange={() => save('mapShow', show)} /></label>
	</div>
	<h3 class="label">Appearance</h3>
	<div class="option-list">
		<label>
			Fading
			<select bind:value={fading} onchange={() => save('mapFading', fading)}>
				<option value="">None</option>
				<option value="age">By age</option>
				<option value="match">By match</option>
			</select>
		</label>
		<label>Contrast <input type="range" min="0" max="100" bind:value={contrast} disabled={!fading} oninput={() => save('mapContrast', contrast)} /></label>
	</div>
</Sheet>

<style>
	:global(body:has(.map-page)) {
		height: 100dvh;
		overflow: hidden;
		display: flex;
		flex-direction: column;
	}

	:global(body:has(.map-page) main) {
		flex: 1;
		min-height: 0;
		width: 100%;
		max-width: none;
		padding: 0 0 var(--bottom-chrome);
	}

	.map-page {
		position: relative;
		height: 100%;
	}

	.map-frame {
		position: absolute;
		inset: 0;
		border: 0;
		border-radius: 0;
	}

	.overlay {
		position: absolute;
		z-index: 4;
		top: calc(0.75rem + var(--safe-top));
		left: calc(0.75rem + var(--safe-left));
		right: 4rem;
		display: flex;
		flex-wrap: wrap;
		align-items: flex-start;
		gap: 0.5rem;
		pointer-events: none;
	}

	.overlay > :global(*) {
		pointer-events: auto;
	}

	.overlay summary,
	.overlay > button {
		border-color: transparent;
		box-shadow: var(--shadow-lg);
	}

	.overlay summary .muted {
		font-weight: 400;
	}

	.swatch {
		display: inline-block;
		width: 0.7rem;
		height: 0.7rem;
		border-radius: 50%;
		flex: none;
	}

	.empty {
		position: absolute;
		z-index: 4;
		inset: auto 1rem 2.5rem;
		max-width: 26rem;
		margin: 0 auto;
	}

	.label {
		margin: 0.5rem 0 0.25rem;
	}

	.map-page :global(.maplibregl-ctrl-top-right) {
		top: var(--safe-top);
	}
</style>
