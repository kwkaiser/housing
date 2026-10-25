import { api, need } from '#lib/api/client.ts';
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;
export const trailingSlash = 'never';

export const load: LayoutLoad = async ({ fetch }) => ({ me: await need(api(fetch).GET('/api/v1/me')) });
