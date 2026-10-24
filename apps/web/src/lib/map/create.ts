import { GeolocateControl, Map, NavigationControl, setWorkerUrl, type LngLatLike } from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url';
import type { MapConfig } from '#lib/api/client.ts';
import { overlays } from './overlays';
import { layerIds, savedLayers, saveLayers, style, type Layers, type OverlayName } from './style';

export type HousingMap = {
	map: Map;
	layers: Layers;
	setLayers(next: Layers): void;
};

setWorkerUrl(workerUrl);

export function createMap(container: HTMLElement, config: MapConfig, view: { center?: LngLatLike; zoom?: number } = {}): HousingMap {
	const layers = savedLayers(config);
	const map = new Map({
		container,
		style: style(config, layers),
		center: view.center ?? [-98.6, 39.8],
		zoom: view.zoom ?? 3,
		attributionControl: { compact: true },
		dragRotate: false,
		pitchWithRotate: false
	});
	map.touchZoomRotate.disableRotation();
	map.addControl(new NavigationControl({ showCompass: false }), 'top-right');
	map.addControl(new GeolocateControl({ trackUserLocation: false }), 'top-right');
	const ov = overlays(map, config);
	map.on('load', () => layers.overlays.forEach((n) => ov.set(n, true)));

	const self: HousingMap = {
		map,
		layers,
		setLayers(next) {
			const vis = (id: string, on: boolean) => map.getLayer(id) && map.setLayoutProperty(id, 'visibility', on ? 'visible' : 'none');
			vis('positron', next.base === 'positron');
			vis('osm', next.base === 'osm');
			for (const name of ['bike', 'rail', 'bus'] as OverlayName[]) {
				const on = next.overlays.includes(name);
				layerIds(name, config).forEach((id) => vis(id, on));
				ov.set(name, on);
			}
			self.layers = next;
			saveLayers(next);
		}
	};
	return self;
}
