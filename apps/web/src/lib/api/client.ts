import { error } from '@sveltejs/kit';
import createClient, { type Middleware } from 'openapi-fetch';
import type { components, paths } from './schema';

export type Schemas = components['schemas'];
export type Collection = Schemas['APICollection'];
export type CollectionListings = Schemas['APICollectionListings'];
export type ListingRow = Schemas['APIListingRow'];
export type Listing = Schemas['APIListing'];
export type ListingDetail = Schemas['APIListingDetail'];
export type Grade = Schemas['APIGrade'];
export type Job = Schemas['APIJob'];
export type JobEvent = Schemas['APIJobEvent'];
export type Meta = Schemas['APIMeta'];
export type MapConfig = Schemas['APIMapConfig'];
export type MapPins = Schemas['APIMapPins'];
export type MapPin = Schemas['APIMapPin'];
export type ProfileItem = Schemas['APIProfileListItem'];
export type ProfileDetail = Schemas['APIProfileDetail'];
export type Criterion = Schemas['Criterion'];
export type Favorite = Schemas['APIFavorite'];
export type User = Schemas['APIUser'];

type Fetch = typeof fetch;

const signIn: Middleware = {
	onResponse({ response }) {
		if (response.status === 401) location.assign(`/auth/login?rd=${encodeURIComponent(location.pathname + location.search)}`);
	}
};

export function api(f: Fetch = fetch) {
	const client = createClient<paths>({ baseUrl: '', fetch: f });
	client.use(signIn);
	return client;
}

export class ApiError extends Error {
	status: number;
	fields: Record<string, string>;
	general: string[];

	constructor(status: number, problem?: Schemas['ErrorModel']) {
		super(problem?.detail || problem?.title || `request failed with status ${status}`);
		this.status = status;
		this.fields = {};
		this.general = [];
		for (const e of problem?.errors ?? []) {
			const field = e.location?.replace(/^body\.?/, '') ?? '';
			if (field) this.fields[field] = e.message ?? 'invalid';
			else if (e.message) this.general.push(e.message);
		}
	}
}

type Result<T> = { data?: T; error?: unknown; response: Response };

export async function call<T>(request: Promise<Result<T>>): Promise<T> {
	const { data, error: problem, response } = await request;
	if (!response.ok) throw new ApiError(response.status, problem as Schemas['ErrorModel'] | undefined);
	return data as T;
}

export async function need<T>(request: Promise<Result<T>>): Promise<T> {
	try {
		return await call(request);
	} catch (e) {
		if (e instanceof ApiError) error(e.status === 404 ? 404 : 502, e.message);
		throw e;
	}
}
