import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const client = api(fetch);
	const [detail, jobs, meta] = await Promise.all([
		need(client.GET('/api/v1/profiles/{id}', { params: { path: { id: params.id } } })),
		need(client.GET('/api/v1/jobs', { params: { query: { profile: params.id, limit: 1 } } })),
		need(client.GET('/api/v1/meta'))
	]);
	return { detail, job: jobs[0], meta };
};
