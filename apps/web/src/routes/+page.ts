import { redirect } from '@sveltejs/kit';
import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const cs = await need(api(fetch).GET('/api/v1/collections'));
	if (cs.length > 0) redirect(307, `/c/${encodeURIComponent(cs[0].id)}`);
	return {};
};
