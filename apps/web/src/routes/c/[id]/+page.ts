import { api, need } from '#lib/api/client.ts';
import { shiftDay } from '#lib/format.ts';
import { ALL_DATES } from '#lib/ranges.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params, url }) => {
	const q = url.searchParams;
	const single = q.has('day');
	const from = single ? '' : q.has('from') ? (q.get('from') ?? '') : shiftDay(new Date(), { days: -1 });
	const to = single ? '' : (q.get('to') ?? '');
	const client = api(fetch);
	const [view, collections] = await Promise.all([
		need(
			client.GET('/api/v1/collections/{id}/listings', {
				params: {
					path: { id: params.id },
					query: {
						day: single ? q.get('day') || 'latest' : undefined,
						from: single ? undefined : from || ALL_DATES,
						to: to || undefined,
						dealbreakers: q.get('dealbreakers') === '1',
						new_only: q.get('new') === '1',
						hide_repeats: q.get('repeats') === '0',
						min_match: Number(q.get('min')) || undefined
					}
				}
			})
		),
		need(client.GET('/api/v1/collections'))
	]);
	return { view, collections, single, from, to };
};
