import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => ({
	detail: await need(api(fetch).GET('/api/v1/profiles/{id}', { params: { path: { id: params.id } } })),
	meta: await need(api(fetch).GET('/api/v1/meta'))
});
