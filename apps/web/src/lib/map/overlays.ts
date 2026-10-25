import { Popup, type GeoJSONSource, type Map, type MapMouseEvent } from 'maplibre-gl';
import type { MapConfig, Schemas } from '#lib/api/client.ts';
import type { OverlayName } from './style';

type Point = [number, number];
type Feature = { id: string; label: string; detail?: string; color?: string; headway?: number; area?: number; lines?: Point[][]; polygons?: Point[][][] };

const maxTiles = 64;

function tileRange(map: Map, z: number) {
	const b = map.getBounds();
	const n = 2 ** z;
	const x = (lng: number) => Math.floor(((lng + 180) / 360) * n);
	const y = (lat: number) => {
		const r = (Math.max(-85.05, Math.min(85.05, lat)) * Math.PI) / 180;
		return Math.floor(((1 - Math.log(Math.tan(r) + 1 / Math.cos(r)) / Math.PI) / 2) * n);
	};
	const clamp = (v: number) => Math.max(0, Math.min(n - 1, v));
	return { x0: clamp(x(b.getWest())), x1: clamp(x(b.getEast())), y0: clamp(y(b.getNorth())), y1: clamp(y(b.getSouth())) };
}

class Overlay {
	tiles = new globalThis.Map<string, { ctrl: AbortController; features: Feature[] }>();
	enabled = false;

	constructor(
		private map: Map,
		private name: OverlayName,
		private config: Schemas['APIOverlay']
	) {}

	set(on: boolean) {
		this.enabled = on;
		if (on) this.load();
		else this.clear();
	}

	load() {
		if (!this.enabled) return;
		const zoom = Math.round(this.map.getZoom());
		const want = new globalThis.Map<string, { z: number; x: number; y: number }>();
		if (zoom >= this.config.min_zoom) {
			const z = Math.min(zoom, this.config.max_zoom);
			const r = tileRange(this.map, z);
			for (let x = r.x0; x <= r.x1; x++) for (let y = r.y0; y <= r.y1; y++) want.set(`${z}/${x}/${y}`, { z, x, y });
		}
		if (want.size > maxTiles) want.clear();
		let changed = false;
		for (const [key, t] of this.tiles) {
			if (!want.has(key)) {
				t.ctrl.abort();
				this.tiles.delete(key);
				changed = true;
			}
		}
		for (const [key, c] of want) {
			if (this.tiles.has(key)) continue;
			const entry = { ctrl: new AbortController(), features: [] as Feature[] };
			this.tiles.set(key, entry);
			const url = this.config.url.replace('{z}', String(c.z)).replace('{x}', String(c.x)).replace('{y}', String(c.y));
			fetch(url, { signal: entry.ctrl.signal })
				.then((r) => (r.ok ? r.json() : { features: [] }))
				.then((body: { features: Feature[] }) => {
					if (this.tiles.get(key) !== entry) return;
					entry.features = body.features ?? [];
					this.render();
				})
				.catch(() => undefined);
		}
		if (changed) this.render();
	}

	clear() {
		for (const t of this.tiles.values()) t.ctrl.abort();
		this.tiles.clear();
		this.render();
	}

	render() {
		const byId = new globalThis.Map<string, Feature & { all: Point[][] }>();
		for (const t of this.tiles.values()) {
			for (const f of t.features) {
				const prev = byId.get(f.id);
				if (!prev) byId.set(f.id, { ...f, all: [...(f.lines ?? [])] });
				else if (f.lines) prev.all.push(...f.lines);
			}
		}
		const flip = (l: Point[]) => l.map(([lat, lng]) => [lng, lat]);
		const source = this.map.getSource<GeoJSONSource>(`ov-${this.name}`);
		source?.setData({
			type: 'FeatureCollection',
			features: [...byId.values()]
				.sort((a, b) => (b.area ?? 0) - (a.area ?? 0))
				.map((f) => ({
					type: 'Feature',
					properties: { id: f.id, label: f.label, detail: f.detail ?? '', color: f.color ?? null, headway: f.headway ?? 0 },
					geometry: f.polygons
						? { type: 'MultiPolygon', coordinates: f.polygons.map((p) => p.map(flip)) }
						: { type: 'MultiLineString', coordinates: f.all.map(flip) }
				}))
		});
	}
}

function routeLabel(p: Record<string, unknown>): HTMLElement {
	const box = document.createElement('div');
	box.className = 'route-tip';
	const title = document.createElement('strong');
	title.textContent = String(p.label ?? '');
	box.append(title);
	const h = Number(p.headway);
	const every = h ? `every ${Math.max(1, Math.round(h / 60))} min` : '';
	const detail = [p.detail, every].filter(Boolean).join(' · ');
	if (detail) {
		const line = document.createElement('div');
		line.className = 'muted';
		line.textContent = detail;
		box.append(line);
	}
	return box;
}

export function overlays(map: Map, config: MapConfig) {
	const all = new globalThis.Map<OverlayName, Overlay>();
	for (const o of config.overlays ?? []) all.set(o.name, new Overlay(map, o.name, o));
	map.on('moveend', () => all.forEach((o) => o.load()));

	const tip = new Popup({ closeButton: false, closeOnClick: false, offset: 8, className: 'route-popup' });
	let hovered: { source: string; id: string | number } | null = null;
	const unhover = () => {
		if (hovered) map.setFeatureState(hovered, { hover: false });
		hovered = null;
		tip.remove();
	};
	const sources = new globalThis.Map<string, string>();
	for (const name of all.keys()) sources.set(name === 'towns' ? 'ov-towns-fill' : `ov-${name}`, `ov-${name}`);
	map.on('mousemove', (e: MapMouseEvent) => {
		const f = map.queryRenderedFeatures(e.point)[0];
		const source = f && sources.get(f.layer.id);
		if (!source || f.id === undefined) return unhover();
		if (hovered?.source !== source || hovered.id !== f.id) {
			unhover();
			hovered = { source, id: f.id };
			map.setFeatureState(hovered, { hover: true });
		}
		tip.setLngLat(e.lngLat).setDOMContent(routeLabel(f.properties)).addTo(map);
	});
	map.on('mouseout', unhover);

	return {
		set(name: OverlayName, on: boolean) {
			all.get(name)?.set(on);
		},
		has(name: OverlayName) {
			return all.has(name);
		}
	};
}

