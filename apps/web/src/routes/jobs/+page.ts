import { api, call, need, type Job } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

type Status = Job['status'];
type Kind = 'run_collection' | 'create_profile' | 'draft_profile' | 'prune_media';

export const load: PageLoad = async ({ fetch, url }) => {
	const q = url.searchParams;
	const limit = Number(q.get('limit')) || 50;
	const query = {
		collection: q.get('collection') || undefined,
		status: (q.get('status') || undefined) as Status | undefined,
		kind: (q.get('kind') || undefined) as Kind | undefined,
		before: Number(q.get('before')) || undefined,
		limit
	};
	const client = api(fetch);
	const res = await client.GET('/api/v1/jobs', { params: { query } });
	const jobs = await call(Promise.resolve(res));
	const next = res.response.headers.get('Link')?.match(/<[^>]*[?&]before=(\d+)[^>]*>;\s*rel="next"/)?.[1];
	const collections = await need(client.GET('/api/v1/collections'));
	return { jobs, next, query, collections };
};
