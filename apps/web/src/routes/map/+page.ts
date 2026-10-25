import { api, need, type MapPins } from '#lib/api/client.ts';
import { shiftDay } from '#lib/format.ts';
import { mapConfig } from '#lib/map/config.ts';
import type { PageLoad } from './$types';

const cache = new Map<string, MapPins>();

export const load: PageLoad = async ({ fetch, url }) => {
	const q = url.searchParams;
	let from = q.get('from') ?? '';
	const to = q.get('to') ?? '';
	if (!q.has('from') && !q.has('to')) from = shiftDay(new Date(), { days: -1 });
	const key = `${from}|${to}`;
	const request = need(api(fetch).GET('/api/v1/map/pins', { params: { query: { from: from || undefined, to: to || undefined } } })).then((pins) => {
		cache.set(key, pins);
		return pins;
	});
	const hit = cache.get(key);
	const [config, pins] = await Promise.all([mapConfig(fetch), hit ?? request, import('#lib/map/create.ts')]);
	return { config, pins, fresh: hit ? request : undefined, from, to };
};
