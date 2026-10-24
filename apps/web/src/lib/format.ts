export function thousands(n: number): string {
	return Math.round(n).toLocaleString('en-US');
}

export function money(cents: number | undefined, offer?: string): string {
	if (!cents) return '-';
	const sign = cents < 0 ? '-' : '';
	const s = `${sign}$${thousands(Math.abs(cents) / 100)}`;
	return offer === 'rent' ? `${s}/mo` : s;
}

export function beds(n: number | undefined): string {
	if (n === undefined || n === null) return '-';
	return n === 0 ? 'studio' : String(n);
}

export function baths(n: number | undefined): string {
	return n === undefined || n === null ? '-' : String(n);
}

export function sqft(n: number | undefined): string {
	return n ? `${thousands(n)} sqft` : '-';
}

export function match(m: number, calibrated: boolean, stale?: boolean): string {
	const s = calibrated ? `${m.toFixed(0)}%` : m.toFixed(1);
	return stale ? `${s}*` : s;
}

export function percent(n: number): string {
	return `${n.toFixed(0)}%`;
}

export function shortModel(m: string): string {
	return m.slice(m.lastIndexOf('/') + 1);
}

export function cost(usd: number): string {
	if (!usd) return '-';
	return usd < 1 ? `$${usd.toFixed(3)}` : `$${usd.toFixed(2)}`;
}

export function ago(iso: string | undefined, now = Date.now()): string {
	if (!iso) return '-';
	const d = now - Date.parse(iso);
	const min = 60_000;
	const hour = 60 * min;
	if (d < min) return 'just now';
	if (d < hour) return `${Math.floor(d / min)}m ago`;
	if (d < 48 * hour) return `${Math.floor(d / hour)}h ago`;
	return `${Math.floor(d / (24 * hour))}d ago`;
}

export function elapsed(ms: number): string {
	const s = Math.max(0, Math.round(ms / 1000));
	if (s < 60) return `${s}s`;
	if (s < 3600) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, '0')}s`;
	return `${Math.floor(s / 3600)}h ${String(Math.floor((s % 3600) / 60)).padStart(2, '0')}m`;
}

export function dateTime(iso: string | undefined): string {
	if (!iso) return '-';
	const d = new Date(iso);
	return `${localDay(d)} ${d.toTimeString().slice(0, 5)}`;
}

export function time(iso: string): string {
	return new Date(iso).toTimeString().slice(0, 8);
}

export function localDay(d: Date): string {
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function shiftDay(d: Date, { days = 0, months = 0 }: { days?: number; months?: number }): string {
	const out = new Date(d);
	out.setMonth(out.getMonth() + months);
	out.setDate(out.getDate() + days);
	return localDay(out);
}

const kinds: Record<string, string> = {
	run_collection: 'Collection run',
	create_profile: 'Create profile',
	draft_profile: 'Draft profile'
};

export function kindLabel(kind: string): string {
	return kinds[kind] ?? kind.replaceAll('_', ' ');
}

export function label(s: string): string {
	return s.replaceAll('_', ' ');
}

export function address(a: { formatted: string; street?: string; unit?: string; city?: string }): string {
	return a.formatted || [a.street, a.unit, a.city].filter(Boolean).join(', ');
}

export function lines(s: string): string[] {
	return s
		.split('\n')
		.map((l) => l.trim())
		.filter(Boolean);
}

export function sized(path: string, size: 320 | 640 | 1280): string {
	return `${path}?size=${size}`;
}
