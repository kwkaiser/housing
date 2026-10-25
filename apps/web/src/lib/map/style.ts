import type { LayerSpecification, StyleSpecification } from 'maplibre-gl';
import type { MapConfig } from '#lib/api/client.ts';
import { load, save } from '#lib/storage.ts';

export type Base = 'positron' | 'osm';
export type OverlayName = 'places' | 'terrain' | 'noise' | 'towns' | 'bike' | 'rail' | 'bus';
export type Layers = { base: Base; overlays: OverlayName[] };

export const BIKE = '#009400';
export const BUS = '#a16207';
export const RAIL = '#475569';
export const TOWN = '#86198f';
export const PLACE = '#1c1917';
export const noiseLegend: [string, string][] = [
	['#ffc107', '45'],
	['#ff8000', '50'],
	['#ff0000', '55'],
	['#ff3399', '60'],
	['#a300cc', '70'],
	['#5200cc', '80'],
	['#0000ff', '90+']
];

const osm = '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors';
const transitland = '<a href="https://www.transit.land/terms">Transitland</a>';
const census = '<a href="https://www.census.gov/data/developers/data-sets/TIGERweb-map-service.html">U.S. Census Bureau</a>';

export const overlayLabels: Record<OverlayName, string> = { places: 'Places', terrain: 'Terrain', noise: 'Noise', towns: 'Town lines', bike: 'Bike lanes', rail: 'Rail transit', bus: 'Bus routes' };
export const overlayNames: OverlayName[] = ['places', 'terrain', 'noise', 'towns', 'bike', 'rail', 'bus'];

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
	return overlayNames.filter((n) => {
		if (n === 'terrain') return !!config.terrain;
		if (n === 'noise') return !!config.noise;
		return n === 'places' || n === 'bike' || n === 'rail' || names.has(n);
	});
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
				tiles: config.cyclosm ? [config.cyclosm.url] : ['a', 'b', 'c'].map((s) => `https://${s}.tile-cyclosm.openstreetmap.fr/cyclosm-lite/{z}/{x}/{y}.png`),
				tileSize: retina ? 128 : 256,
				maxzoom: config.cyclosm?.max_zoom ?? 20,
				attribution: '<a href="https://github.com/cyclosm/cyclosm-cartocss-style/releases">CyclOSM</a>'
			},
			'ov-towns': { ...empty, attribution: census },
			'ov-bike': { ...empty, attribution: '<a href="https://openfreemap.org">OpenFreeMap</a> &copy; <a href="https://www.openmaptiles.org/">OpenMapTiles</a>' },
			'ov-rail': { ...empty, attribution: transitland },
			'ov-bus': { ...empty, attribution: transitland },
			places: empty,
			'place-draft': empty
		},
		layers: [
			{ id: 'osm', type: 'raster', source: 'osm', layout: vis(layers.base === 'osm') },
			{ id: 'cyclosm', type: 'raster', source: 'cyclosm', layout: vis(layers.overlays.includes('bike')), paint: { 'raster-hue-rotate': 255, 'raster-brightness-min': 0.35 } },
			{
				id: 'ov-towns-fill',
				type: 'fill',
				source: 'ov-towns',
				layout: vis(layers.overlays.includes('towns')),
				paint: { 'fill-color': TOWN, 'fill-opacity': ['case', hover, 0.08, 0] }
			},
			{
				id: 'ov-towns',
				type: 'line',
				source: 'ov-towns',
				layout: vis(layers.overlays.includes('towns')),
				paint: { 'line-color': TOWN, 'line-width': ['case', hover, 4, 1.5], 'line-opacity': ['case', hover, 1, 0.6] }
			},
			{
				id: 'ov-bike-paths',
				type: 'line',
				source: 'ov-bike',
				layout: { ...vis(layers.overlays.includes('bike')), 'line-cap': 'round', 'line-join': 'round' },
				paint: { 'line-color': BIKE, 'line-width': ['interpolate', ['linear'], ['zoom'], 13, 1.5, 16, 3, 18, 5], 'line-opacity': 0.85 }
			},
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
			},
			{
				id: 'places',
				type: 'circle',
				source: 'places',
				layout: vis(layers.overlays.includes('places')),
				paint: { 'circle-radius': ['case', hover, 9, 7], 'circle-color': PLACE, 'circle-stroke-color': '#ffffff', 'circle-stroke-width': 2.5 }
			},
			{
				id: 'places-hit',
				type: 'circle',
				source: 'places',
				layout: vis(layers.overlays.includes('places')),
				paint: { 'circle-radius': typeof matchMedia !== 'undefined' && matchMedia('(pointer: coarse)').matches ? 20 : 12, 'circle-opacity': 0 }
			},
			{
				id: 'place-draft',
				type: 'circle',
				source: 'place-draft',
				paint: { 'circle-radius': 9, 'circle-color': PLACE, 'circle-opacity': 0.35, 'circle-stroke-color': PLACE, 'circle-stroke-width': 2.5 }
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
	if (config.noise) {
		style.sources.noise = {
			type: 'raster',
			tiles: [config.noise.url],
			tileSize: 256,
			maxzoom: config.noise.max_zoom,
			attribution: '<a href="https://www.bts.gov/geospatial/national-transportation-noise-map">BTS</a>'
		};
		const at = style.layers.findIndex((l) => l.id === 'cyclosm');
		style.layers.splice(at, 0, { id: 'noise', type: 'raster', source: 'noise', layout: vis(layers.overlays.includes('noise')), paint: { 'raster-opacity': 0.45 } });
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
		case 'places':
			return ['places', 'places-hit'];
		case 'terrain':
			return ['terrain'];
		case 'noise':
			return ['noise'];
		case 'towns':
			return ['ov-towns-fill', 'ov-towns'];
		case 'bike':
			return ['cyclosm', 'ov-bike-paths', 'ov-bike'];
		case 'rail':
			return hasOverlay(config, 'rail') ? ['ov-rail'] : ['openrailwaymap'];
		case 'bus':
			return ['ov-bus'];
	}
}
