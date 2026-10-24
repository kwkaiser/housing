<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { LngLatBounds, Popup, type GeoJSONSource, type MapLayerMouseEvent } from 'maplibre-gl';
	import { goto } from '$app/navigation';
	import type { MapPin } from '#lib/api/client.ts';
	import LayerPanel from '#lib/components/LayerPanel.svelte';
	import { createMap, type HousingMap } from '#lib/map/create.ts';
	import { beds, match, money, sized } from '#lib/format.ts';
	import RangeSelect from '#lib/components/RangeSelect.svelte';
	import { load, save } from '#lib/storage.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const UNGRADED = '#78716c';
	type Fading = '' | 'age' | 'match';
	type Placed = { pin: MapPin; lat: number; lng: number };

	let collections = $derived(data.pins.collections ?? []);
	let colors = $derived(Object.fromEntries(collections.map((c) => [c.id, c.color])));
	let chosen = $state<string>(load('mapCollection', ''));
	let selected = $derived(collections.some((c) => c.id === chosen) ? chosen : (collections[0]?.id ?? ''));
	let fading = $state<Fading>(load<Fading>('mapFading', 'age'));
	let contrast = $state<number>(Number(load('mapContrast', 50)));
	let show = $state<{ dealbreakers?: boolean; ungraded?: boolean }>(load('mapShow', {}));
	let cutoff = $state<number>(Number(load('mapCutoff', 0)) || 0);

	let placed = $derived.by((): Placed[] => {
		const out = (data.pins.pins ?? []).map((pin) => ({ pin, lat: pin.lat, lng: pin.lng }));
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
		if (!show.dealbreakers && g?.dealbreakers?.length) return null;
		if (g && cutoff > 0 && g.match < cutoff) return null;
		return c;
	}

	let visible = $derived(placed.map((p) => ({ ...p, lead: lead(p.pin) })).filter((p) => p.lead !== null) as (Placed & { lead: string })[]);

	const FLOOR = 0.2;
	const clamp = (n: number) => Math.max(0, Math.min(1, n));

	let features = $derived.by(() => {
		const start = Date.parse(data.from || data.pins.first_day || '');
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
			const s = !fading ? 1 : l === null ? 0.5 : FLOOR + (1 - FLOOR) * l ** gamma;
			const g = p.pin.grades[p.lead];
			const hollow = Boolean(g?.dealbreakers?.length);
			return {
				type: 'Feature' as const,
				id: i,
				properties: {
					i: data.pins.pins!.indexOf(p.pin),
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
		const pins = (data.pins.pins ?? []).filter((p) => (p.collections ?? []).includes(selected));
		const parts = [`${pins.length} listings placed`];
		const approx = pins.filter((p) => p.approx).length;
		const ungraded = pins.filter((p) => p.ungraded).length;
		const dealbreakers = pins.filter((p) => p.grades[selected]?.dealbreakers?.length).length;
		if (approx) parts.push(`${approx} approximate from zip code`);
		if (dealbreakers) parts.push(`${dealbreakers} with dealbreakers`);
		if (ungraded) parts.push(`${ungraded} ungraded`);
		if (data.pins.unplaced) parts.push(`${data.pins.unplaced} without a location across all collections`);
		return parts.join(' · ');
	});


	function range(from: string, to: string) {
		const q = new URLSearchParams({ from });
		if (to) q.set('to', to);
		goto(`/map?${q}`);
	}

	let el: HTMLDivElement;
	let hm = $state<HousingMap>();

	function fit() {
		const map = hm?.map;
		if (!map) return;
		const bounds = new LngLatBounds();
		for (const p of visible) bounds.extend([p.lng, p.lat]);
		if (bounds.isEmpty()) {
			const on = collections.filter((c) => c.id === selected && c.area);
			for (const c of on.length ? on : collections.filter((c) => c.area)) {
				bounds.extend([c.area!.west, c.area!.south]);
				bounds.extend([c.area!.east, c.area!.north]);
			}
		}
		if (bounds.isEmpty()) map.jumpTo({ center: [-98.6, 39.8], zoom: 3 });
		else map.fitBounds(bounds, { padding: 40, maxZoom: 15, duration: 0 });
	}

	function popup(pin: MapPin, leadId: string): HTMLElement {
		const box = document.createElement('div');
		const href = `/listing/${encodeURIComponent(pin.source)}/${encodeURIComponent(pin.source_id)}?collection=${encodeURIComponent(leadId)}&day=${pin.last_seen}`;
		if (pin.preview) {
			const a = document.createElement('a');
			a.href = href;
			const img = document.createElement('img');
			img.className = 'preview';
			img.src = sized(pin.preview, 640);
			img.alt = '';
			a.append(img);
			box.append(a);
		}
		const title = document.createElement('a');
		title.href = href;
		title.textContent = pin.address;
		const g = pin.grades[leadId];
		const meta = document.createElement('div');
		const bd = pin.beds === undefined ? null : pin.beds === 0 ? 'studio' : `${beds(pin.beds)} bd`;
		meta.textContent = [g ? match(g.match, g.calibrated, g.stale) : 'Not graded yet', money(pin.price_cents, pin.offer), bd].filter(Boolean).join(' · ');
		const where = document.createElement('div');
		where.className = 'muted';
		const retrieved = pin.last_seen !== pin.first_seen ? ` · last retrieved ${pin.last_seen}` : '';
		where.textContent = `First seen ${pin.first_seen}${retrieved} · ${(pin.collections ?? []).join(', ')}`;
		box.append(title, meta, where);
		if (g?.dealbreakers?.length) {
			const bad = document.createElement('div');
			bad.className = 'bad';
			bad.textContent = `Dealbreakers: ${g.dealbreakers.join(', ')}`;
			box.append(bad);
		}
		if (pin.approx) {
			const note = document.createElement('div');
			note.className = 'muted';
			note.textContent = "Approximate: placed at the zip code's center";
			box.append(note);
		}
		return box;
	}

	onMount(() => {
		hm = createMap(el, data.config);
		const map = hm.map;
		const tip = new Popup({ closeButton: false, closeOnClick: false, offset: 10 });
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
			untrack(fit);
		});
		const pinAt = (e: MapLayerMouseEvent) => {
			const f = e.features?.[0];
			if (!f) return null;
			const p = f.properties as { i: number; lead: string };
			return { pin: data.pins.pins![p.i], lead: p.lead };
		};
		map.on('mousemove', 'pins', (e) => {
			const hit = pinAt(e);
			if (!hit) return;
			map.getCanvas().style.cursor = 'pointer';
			const g = hit.pin.grades[hit.lead];
			tip
				.setLngLat(e.lngLat)
				.setText([money(hit.pin.price_cents, hit.pin.offer), g ? match(g.match, g.calibrated, g.stale) : 'ungraded', g?.dealbreakers?.length ? 'dealbreaker' : null, hit.pin.first_seen].filter(Boolean).join(' · '))
				.addTo(map);
		});
		map.on('mouseleave', 'pins', () => {
			map.getCanvas().style.cursor = '';
			tip.remove();
		});
		map.on('click', 'pins', (e) => {
			const hit = pinAt(e);
			if (!hit) return;
			tip.remove();
			new Popup({ maxWidth: '280px', className: 'pin-popup' }).setLngLat(e.lngLat).setDOMContent(popup(hit.pin, hit.lead)).addTo(map);
		});
		return () => map.remove();
	});

	$effect(() => {
		const fc = { type: 'FeatureCollection' as const, features };
		hm?.map.getSource<GeoJSONSource>('pins')?.setData(fc);
	});

	function choose(id: string) {
		chosen = id;
		save('mapCollection', id);
		queueMicrotask(fit);
	}
</script>

<svelte:head><title>Map · housing</title></svelte:head>

<div class="map-page">
	<h1>Map</h1>
	{#if !collections.length}
		<p>There are no collections yet. <a href="/collections/new">Create one</a>, then run it.</p>
	{:else}
		<div class="controls">
			<details class="picker collections">
				<summary><span class="swatch" style="background: {colors[selected]}"></span> {selected}</summary>
				<div class="menu">
					{#each collections as c (c.id)}
						<label>
							<input type="radio" name="map-collection" checked={c.id === selected} onchange={() => choose(c.id)} />
							<span class="swatch" style="background: {c.color}"></span>
							{c.id} <span class="muted count">{c.graded}{#if c.latest_day} · {c.latest_day}{/if}</span>
						</label>
					{/each}
				</div>
			</details>
			<details class="picker filters">
				<summary>{cutoff ? `Filters · ≥${cutoff}%` : 'Filters'}</summary>
				<div class="menu">
					<label>Match cutoff <input type="number" min="0" step="5" inputmode="numeric" placeholder="0" bind:value={cutoff} oninput={() => save('mapCutoff', cutoff || 0)} /> %</label>
					<label><input type="checkbox" bind:checked={show.dealbreakers} onchange={() => save('mapShow', show)} /> Show dealbreakers</label>
					<label><input type="checkbox" bind:checked={show.ungraded} onchange={() => save('mapShow', show)} /> Show ungraded</label>
				</div>
			</details>
			<RangeSelect from={data.from} to={data.to} onrange={range} />
			<select aria-label="Fading" bind:value={fading} onchange={() => save('mapFading', fading)}>
				<option value="">No fading</option>
				<option value="age">Fade by age</option>
				<option value="match">Fade by match</option>
			</select>
			<label class="toggle contrast">Contrast <input type="range" min="0" max="100" bind:value={contrast} disabled={!fading} oninput={() => save('mapContrast', contrast)} /></label>
			<span class="muted summary">{summary}</span>
		</div>
	{/if}
	<div class="map-frame">
		<div class="map" bind:this={el}></div>
		{#if hm}<LayerPanel map={hm} config={data.config} />{/if}
	</div>
</div>
