import { api, need, type MapConfig } from '#lib/api/client.ts';

let cached: Promise<MapConfig> | undefined;

export function mapConfig(f: typeof fetch): Promise<MapConfig> {
	cached ??= need(api(f).GET('/api/v1/map/config')).catch((e) => {
		cached = undefined;
		throw e;
	});
	return cached;
}
