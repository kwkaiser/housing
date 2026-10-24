import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params, url }) => {
	const client = api(fetch);
	const [detail, config] = await Promise.all([
		need(
			client.GET('/api/v1/listings/{source}/{source_id}', {
				params: {
					path: { source: params.source, source_id: params.id },
					query: { collection: url.searchParams.get('collection') ?? undefined, day: url.searchParams.get('day') ?? undefined }
				}
			})
		),
		need(client.GET('/api/v1/map/config'))
	]);
	return { detail, config };
};
