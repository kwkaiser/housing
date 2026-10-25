import { api, need } from '#lib/api/client.ts';
import { mapConfig } from '#lib/map/config.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params, url }) => {
	const [detail, config] = await Promise.all([
		need(
			api(fetch).GET('/api/v1/listings/{source}/{source_id}', {
				params: {
					path: { source: params.source, source_id: params.id },
					query: { collection: url.searchParams.get('collection') ?? undefined, day: url.searchParams.get('day') ?? undefined }
				}
			})
		),
		mapConfig(fetch)
	]);
	return { detail, config };
};
