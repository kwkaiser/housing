import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const client = api(fetch);
	const [meta, profiles, collection] = await Promise.all([
		need(client.GET('/api/v1/meta')),
		need(client.GET('/api/v1/profiles')),
		need(client.GET('/api/v1/collections/{id}', { params: { path: { id: params.id } } }))
	]);
	return { meta, profiles, collection };
};
