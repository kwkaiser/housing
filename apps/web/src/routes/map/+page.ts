import { api, need } from '#lib/api/client.ts';
import { shiftDay } from '#lib/format.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
	const q = url.searchParams;
	let from = q.get('from') ?? '';
	const to = q.get('to') ?? '';
	if (!q.has('from') && !q.has('to')) from = shiftDay(new Date(), { months: -1 });
	const client = api(fetch);
	const [config, pins] = await Promise.all([
		need(client.GET('/api/v1/map/config')),
		need(client.GET('/api/v1/map/pins', { params: { query: { from: from || undefined, to: to || undefined } } })),
		import('#lib/map/create.ts')
	]);
	return { config, pins, from, to };
};
