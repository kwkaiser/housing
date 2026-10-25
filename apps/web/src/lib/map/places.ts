import { Popup, type GeoJSONSource, type Map, type MapLayerMouseEvent } from 'maplibre-gl';
import { api, call, type Place } from '#lib/api/client.ts';

export type Places = ReturnType<typeof places>;

function details(place: Place, onremove: () => void): HTMLElement {
	const box = document.createElement('div');
	box.className = 'place-tip';
	const title = document.createElement('strong');
	title.textContent = place.name;
	const address = document.createElement('div');
	address.className = 'muted';
	address.textContent = place.address;
	const remove = document.createElement('button');
	remove.type = 'button';
	remove.className = 'ghost';
	remove.textContent = 'Remove';
	remove.onclick = onremove;
	box.append(title, address, remove);
	return box;
}

export function places(map: Map) {
	let enabled = false;
	let loaded: Place[] = [];
	let request = 0;

	const render = () =>
		map.getSource<GeoJSONSource>('places')?.setData({
			type: 'FeatureCollection',
			features: (enabled ? loaded : []).map((p) => ({
				type: 'Feature',
				properties: { id: p.id, label: p.name, detail: p.address },
				geometry: { type: 'Point', coordinates: [p.lng, p.lat] }
			}))
		});

	async function refresh() {
		const n = ++request;
		const list = await call(api().GET('/api/v1/places')).catch(() => null);
		if (n !== request || !list) return;
		loaded = list;
		render();
	}

	map.on('click', 'places-hit', (e: MapLayerMouseEvent) => {
		const id = e.features?.[0]?.properties?.id;
		const place = loaded.find((p) => p.id === id);
		if (!place) return;
		const pop = new Popup({ maxWidth: '260px', className: 'place-popup', offset: 10 }).setLngLat([place.lng, place.lat]);
		pop.setDOMContent(
			details(place, async () => {
				if (!confirm(`Remove ${place.name} from your places?`)) return;
				await call(api().DELETE('/api/v1/places/{id}', { params: { path: { id: place.id } } })).catch(() => undefined);
				pop.remove();
				refresh();
			})
		);
		pop.addTo(map);
	});
	map.on('mouseenter', 'places-hit', () => (map.getCanvas().style.cursor = 'pointer'));
	map.on('mouseleave', 'places-hit', () => (map.getCanvas().style.cursor = ''));

	return {
		set(on: boolean) {
			if (on === enabled) return;
			enabled = on;
			if (on) refresh();
			else render();
		},
		refresh,
		draft(at: [number, number] | null) {
			map.getSource<GeoJSONSource>('place-draft')?.setData({
				type: 'FeatureCollection',
				features: at ? [{ type: 'Feature', properties: {}, geometry: { type: 'Point', coordinates: at } }] : []
			});
		}
	};
}
