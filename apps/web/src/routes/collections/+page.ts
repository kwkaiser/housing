import { api, need } from '#lib/api/client.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => ({ collections: await need(api(fetch).GET('/api/v1/collections')) });
