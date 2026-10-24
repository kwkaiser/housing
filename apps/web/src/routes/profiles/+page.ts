import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => ({ profiles: await need(api(fetch).GET('/api/v1/profiles')) });
