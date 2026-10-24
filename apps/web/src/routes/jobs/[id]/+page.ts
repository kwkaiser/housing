import { error } from '@sveltejs/kit';
import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0) error(404, 'no such job');
	const client = api(fetch);
	const [job, events] = await Promise.all([
		need(client.GET('/api/v1/jobs/{id}', { params: { path: { id } } })),
		need(client.GET('/api/v1/jobs/{id}/events', { params: { path: { id } } }))
	]);
	return { job, events };
};
