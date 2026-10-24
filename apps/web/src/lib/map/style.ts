import type { LayerSpecification, StyleSpecification } from 'maplibre-gl';
import type { MapConfig } from '#lib/api/client.ts';
import { load, save } from '#lib/storage.ts';

export type Base = 'positron' | 'osm';
export type OverlayName = 'terrain' | 'bike' | 'rail' | 'bus';
export type Layers = { base: Base; overlays: OverlayName[] };

export const BIKE = '#009400';
export const BUS = '#a16207';
export const RAIL = '#475569';

const osm = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
const transitland = '<a href="https://www.transit.land/terms">Transitland</a>';

export const overlayLabels: Record<OverlayName, string> = { terrain: 'Terrain', bike: 'Bike lanes', rail: 'Rail transit', bus: 'Bus routes' };
export const overlayNames: OverlayName[] = ['terrain', 'bike', 'rail', 'bus'];

export function savedLayers(config: MapConfig): Layers {
	const saved = load<Partial<Layers>>('mapLayers', {});
	const base: Base = saved.base === 'osm' || !config.positron ? 'osm' : 'positron';
	const known = new Set<OverlayName>(availableOverlays(config));
	return { base, overlays: (saved.overlays ?? []).filter((o) => known.has(o)) };
}

export function saveLayers(layers: Layers): void {
	save('mapLayers', layers);
}

export function availableOverlays(config: MapConfig): OverlayName[] {
	const names = new Set((config.overlays ?? []).map((o) => o.name));
	return overlayNames.filter((n) => (n === 'terrain' ? !!config.terrain : n !== 'bus' || names.has(n)));
}

export function hasOverlay(config: MapConfig, name: OverlayName): boolean {
	return (config.overlays ?? []).some((o) => o.name === name);
}

const headway = ['coalesce', ['get', 'headway'], 0];
const busWeight = ['case', ['all', ['>', headway, 0], ['<=', headway, 600]], 4, ['all', ['>', headway, 0], ['<=', headway, 1200]], 3, 2];
const busOpacity = ['case', ['all', ['>', headway, 0], ['<=', headway, 600]], 0.9, ['all', ['>', headway, 0], ['<=', headway, 1200]], 0.65, 0.4];
const hover = ['boolean', ['feature-state', 'hover'], false];

export function style(config: MapConfig, layers: Layers): StyleSpecification {
	const retina = typeof devicePixelRatio !== 'undefined' && devicePixelRatio > 1 ? '@2x' : '';
	const vis = (on: boolean) => ({ visibility: on ? ('visible' as const) : ('none' as const) });
	const empty = { type: 'geojson' as const, data: { type: 'FeatureCollection' as const, features: [] }, promoteId: 'id' };
	const style: StyleSpecification = {
		version: 8,
		sources: {
			osm: { type: 'raster', tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'], tileSize: 256, maxzoom: 19, attribution: osm },
			cyclosm: {
				type: 'raster',
				tiles: ['a', 'b', 'c'].map((s) => `https://${s}.tile-cyclosm.openstreetmap.fr/cyclosm-lite/{z}/{x}/{y}.png`),
				tileSize: 256,
				maxzoom: 20,
				attribution: '<a href="https://github.com/cyclosm/cyclosm-cartocss-style/releases">CyclOSM</a>'
			},
			'ov-bike': { ...empty, attribution: '<a href="https://openfreemap.org">OpenFreeMap</a> &copy; <a href="https://www.openmaptiles.org/">OpenMapTiles</a>' },
			'ov-rail': { ...empty, attribution: transitland },
			'ov-bus': { ...empty, attribution: transitland }
		},
		layers: [
			{ id: 'osm', type: 'raster', source: 'osm', layout: vis(layers.base === 'osm') },
			{ id: 'cyclosm', type: 'raster', source: 'cyclosm', layout: vis(layers.overlays.includes('bike')), paint: { 'raster-hue-rotate': 255, 'raster-brightness-min': 0.35 } },
			{
				id: 'ov-bike',
				type: 'line',
				source: 'ov-bike',
				layout: vis(layers.overlays.includes('bike')),
				paint: { 'line-color': BIKE, 'line-width': ['case', hover, 6, 12], 'line-opacity': ['case', hover, 0.95, 0] }
			},
			{
				id: 'ov-rail',
				type: 'line',
				source: 'ov-rail',
				layout: vis(layers.overlays.includes('rail')),
				paint: { 'line-color': ['coalesce', ['get', 'color'], RAIL], 'line-width': ['case', hover, 8, 4], 'line-opacity': ['case', hover, 1, 0.85] }
			},
			{
				id: 'ov-bus',
				type: 'line',
				source: 'ov-bus',
				layout: vis(layers.overlays.includes('bus')),
				paint: { 'line-color': BUS, 'line-width': ['case', hover, ['+', busWeight, 4], busWeight], 'line-opacity': ['case', hover, 1, busOpacity] }
			}
		] as LayerSpecification[]
	};
	if (config.positron) {
		style.sources.positron = {
			type: 'raster',
			tiles: [config.positron.url.replace('{r}', retina)],
			tileSize: 256,
			maxzoom: config.positron.max_zoom,
			attribution: `${osm} &copy; <a href="https://carto.com/attributions">CARTO</a>`
		};
		style.layers.unshift({ id: 'positron', type: 'raster', source: 'positron', layout: vis(layers.base === 'positron') });
	}
	if (config.terrain) {
		style.sources.terrain = {
			type: 'raster-dem',
			tiles: [config.terrain.url],
			tileSize: config.terrain.tile_size,
			maxzoom: config.terrain.max_zoom,
			encoding: config.terrain.encoding,
			attribution: '<a href="https://mapterhorn.com/attribution">&copy; Mapterhorn</a>'
		};
		const at = style.layers.findIndex((l) => l.id === 'cyclosm');
		style.layers.splice(at, 0, {
			id: 'terrain',
			type: 'hillshade',
			source: 'terrain',
			layout: vis(layers.overlays.includes('terrain')),
			paint: {
				'hillshade-method': 'multidirectional',
				'hillshade-illumination-direction': [270, 315, 0, 45],
				'hillshade-illumination-altitude': [30, 30, 30, 30],
				'hillshade-highlight-color': ['#ffffff00', '#ffffff00', '#ffffff00', '#ffffff00'],
				'hillshade-shadow-color': ['#5a4a3140', '#5a4a3160', '#5a4a3140', '#5a4a3120'],
				'hillshade-exaggeration': 0.6
			}
		});
	}
	if (!hasOverlay(config, 'rail')) {
		style.sources.openrailwaymap = {
			type: 'raster',
			tiles: ['a', 'b', 'c'].map((s) => `https://${s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png`),
			tileSize: 256,
			maxzoom: 19,
			attribution: '<a href="https://www.openrailwaymap.org">OpenRailwayMap</a>'
		};
		const at = style.layers.findIndex((l) => l.id === 'ov-bike');
		style.layers.splice(at, 0, { id: 'openrailwaymap', type: 'raster', source: 'openrailwaymap', layout: vis(layers.overlays.includes('rail')) });
	}
	return style;
}

export function layerIds(name: OverlayName, config: MapConfig): string[] {
	switch (name) {
		case 'terrain':
			return ['terrain'];
		case 'bike':
			return ['cyclosm', 'ov-bike'];
		case 'rail':
			return hasOverlay(config, 'rail') ? ['ov-rail'] : ['openrailwaymap'];
		case 'bus':
			return ['ov-bus'];
	}
}
