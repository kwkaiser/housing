import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const client = api(fetch);
	const [meta, profiles] = await Promise.all([need(client.GET('/api/v1/meta')), need(client.GET('/api/v1/profiles'))]);
	return { meta, profiles };
};
