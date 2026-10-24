<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import { api, call, type ListingRow } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import Star from '#lib/components/Star.svelte';
	import Status from '#lib/components/Status.svelte';
	import { address, ago, match, money, beds, percent } from '#lib/format.ts';
	import { load, save } from '#lib/storage.ts';
	import RangeSelect from '#lib/components/RangeSelect.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let view = $derived(data.view);
	let c = $derived(view.collection);
	let profiles = $derived(c.profiles ?? []);
	let multi = $derived(profiles.length > 1);
	let withDealbreakers = $derived(page.url.searchParams.get('dealbreakers') === '1');
	let pinned = $derived(data.single && page.url.searchParams.get('day') !== 'latest' && view.day ? view.day : '');
	let preset = $derived(data.single ? (pinned ? 'day' : 'latest') : undefined);
	let extras = $derived([{ value: 'latest', label: 'Latest run' }, ...(pinned ? [{ value: 'day', label: `Run on ${pinned}` }] : [])]);
	let runError = $state<unknown>();

	type Column = { key: string; label: string; cls?: string; off?: boolean; desc?: boolean; value: (r: ListingRow) => number | string };

	let columns = $derived.by((): Column[] => {
		const grade = (r: ListingRow, id: string) => (r.grades ?? []).find((g) => g.profile === id);
		const cols: Column[] = [
			{ key: 'rank', label: '#', cls: 'num', value: (r) => r.rank },
			{ key: 'match', label: 'Match', cls: 'num score', desc: true, value: (r) => r.best.match },
			{ key: 'score', label: 'Score', cls: 'num', desc: true, value: (r) => r.best.score }
		];
		if (multi) {
			cols.push({ key: 'profile', label: 'Profile', value: (r) => r.best.profile });
			for (const id of profiles) cols.push({ key: `p:${id}`, label: id, cls: 'num', desc: true, value: (r) => grade(r, id)?.match ?? -1 });
		}
		cols.push(
			{ key: 'coverage', label: 'Coverage', cls: 'num', desc: true, value: (r) => r.best.coverage },
			{ key: 'price', label: 'Price', cls: 'num', value: (r) => r.listing.price.cents },
			{ key: 'beds', label: 'Beds', cls: 'num', value: (r) => r.listing.beds ?? -1 },
			{ key: 'address', label: 'Address', value: (r) => address(r.listing.address).toLowerCase() },
			{ key: 'sources', label: 'Sources', value: (r) => r.listing.source },
			{ key: 'dealbreakers', label: 'Dealbreakers', cls: 'tags bad', off: true, value: (r) => (r.best.dealbreakers ?? []).join(',') },
			{ key: 'missing', label: 'Missing', cls: 'tags', off: true, value: (r) => (r.best.missing_essentials ?? []).join(',') },
			{ key: 'avoids', label: 'Avoids', cls: 'tags', off: true, value: (r) => (r.best.avoids_hit ?? []).join(',') },
			{ key: 'summary', label: 'Summary', cls: 'summary', off: true, value: (r) => r.best.summary }
		);
		return cols;
	});

	let shown = $state<Record<string, boolean>>(load('columns', {}));
	const visible = (col: Column) => shown[col.key] ?? !col.off;

	let sortKey = $derived(page.url.searchParams.get('sort') ?? '');
	let sortCol = $derived(columns.find((col) => col.key === sortKey));
	let desc = $derived.by(() => {
		const dir = page.url.searchParams.get('dir');
		return dir ? dir === 'desc' : (sortCol?.desc ?? false);
	});
	let rows = $derived.by(() => {
		const out = [...(view.rows ?? [])];
		const col = sortCol;
		if (!col) return out;
		return out.sort((a, b) => {
			const x = col.value(a);
			const y = col.value(b);
			const d = x < y ? -1 : x > y ? 1 : 0;
			return desc ? -d : d;
		});
	});

	function href(changes: Record<string, string | null>): string {
		const q = new URLSearchParams(page.url.search);
		for (const [k, v] of Object.entries(changes)) {
			if (v === null) q.delete(k);
			else q.set(k, v);
		}
		const s = q.toString();
		return `/c/${encodeURIComponent(c.id)}${s ? `?${s}` : ''}`;
	}

	function rangeHref(from: string, to: string): string {
		return href({ day: null, from, to: to || null });
	}


	function sortHref(col: Column): string {
		if (col.key !== sortKey) return href({ sort: col.key, dir: col.desc ? 'desc' : 'asc' });
		if (col.key === 'rank' && desc) return href({ sort: null, dir: null });
		return href({ sort: col.key, dir: desc ? 'asc' : 'desc' });
	}

	function toggleColumn(key: string, on: boolean) {
		shown[key] = on;
		save('columns', shown);
	}

	async function run() {
		runError = undefined;
		try {
			const job = await call(api().POST('/api/v1/collections/{id}/runs', { params: { path: { id: c.id } } }));
			goto(`/jobs/${job.id}`);
		} catch (e) {
			runError = e;
		}
	}

	function change(r: ListingRow): { text: string; cls: string } | null {
		if (r.previous_price_cents !== undefined) {
			const diff = r.listing.price.cents - r.previous_price_cents;
			if (diff > 0) return { text: `↑ ${money(diff)}`, cls: 'up' };
			if (diff < 0) return { text: `↓ ${money(-diff)}`, cls: 'down' };
		}
		return r.status === 'changed' ? { text: 'updated', cls: 'changed' } : null;
	}

	function listingHref(r: ListingRow): string {
		const q = new URLSearchParams({ collection: c.id });
		if (r.day) q.set('day', r.day);
		return `/listing/${encodeURIComponent(r.listing.source)}/${encodeURIComponent(r.listing.source_id)}?${q}`;
	}
</script>

<svelte:head><title>{c.id} · housing</title></svelte:head>

<h1>{c.id}</h1>

<div class="controls">
	{#if data.collections.length > 1}
		<select aria-label="Collection" value={c.id} onchange={(e) => goto(`/c/${encodeURIComponent(e.currentTarget.value)}`)}>
			{#each data.collections as x (x.id)}<option value={x.id}>{x.id}</option>{/each}
		</select>
	{/if}
	<RangeSelect
		from={data.from}
		to={data.to}
		{extras}
		active={preset}
		onrange={(from, to) => goto(rangeHref(from, to))}
		onextra={(value) => value === 'latest' && goto(href({ day: 'latest', from: null, to: null }))}
	/>
	{#if data.single}
		{#if view.previous_day}<a class="button" href={href({ day: view.previous_day })}>← {view.previous_day}</a>{:else}<span class="button" aria-disabled="true">←</span>{/if}
		<span class="muted">{view.day ?? 'never run'}</span>
		{#if view.next_day}<a class="button" href={href({ day: view.next_day })}>{view.next_day} →</a>{:else}<span class="button" aria-disabled="true">→</span>{/if}
	{/if}
	<details class="picker">
		<summary>Columns</summary>
		<div class="menu">
			{#each columns as col (col.key)}
				<label><input type="checkbox" checked={visible(col)} onchange={(e) => toggleColumn(col.key, e.currentTarget.checked)} /> {col.label}</label>
			{/each}
		</div>
	</details>
	<a class="button" href={href({ dealbreakers: withDealbreakers ? null : '1' })}>{withDealbreakers ? 'Hide' : 'Show'} dealbreakers</a>
	<label class="toggle"><input type="checkbox" checked={page.url.searchParams.get('new') === '1'} onchange={(e) => goto(href({ new: e.currentTarget.checked ? '1' : null }))} /> New only</label>
	<label class="toggle"><input type="checkbox" checked={page.url.searchParams.get('repeats') === '0'} onchange={(e) => goto(href({ repeats: e.currentTarget.checked ? '0' : null }))} /> Hide repeats</label>
</div>

<div class="controls run">
	<span class="muted">
		{#if !data.single}
			{view.listings} listings retrieved {data.from ? `since ${data.from}` : 'on any date'}{#if data.to} through {data.to}{/if} · {view.graded} graded{#if view.hidden} · {view.hidden} hidden for dealbreakers{/if}
		{:else if view.day}{view.listings} listings on {view.day} · {view.graded} graded{#if view.hidden} · {view.hidden} hidden for dealbreakers{/if}{:else}Never run{/if}
	</span>
	{#if c.last_run}
		<a href="/jobs/{c.last_run.id}"><Status status={c.last_run.status} /></a>
		<span class="muted">{ago(c.last_run.finished_at ?? c.last_run.created_at)}</span>
	{/if}
	<button type="button" onclick={run} disabled={c.last_run?.status === 'queued' || c.last_run?.status === 'running'}>Run now</button>
	<a class="button" href="/collections/{encodeURIComponent(c.id)}/edit">Edit</a>
</div>
<Errors error={runError} />
{#if view.stale}<p class="note">* Some grades predate a change to the profile or listing and will be refreshed on the next run.</p>{/if}

{#if rows.length}
	<div class="wrap">
		<table class="listings">
			<thead>
				<tr>
					{#each columns as col (col.key)}
						{#if visible(col)}
							<th class={col.cls} aria-sort={col.key === sortKey ? (desc ? 'descending' : 'ascending') : undefined}>
								<a href={sortHref(col)} data-sveltekit-noscroll data-sveltekit-replacestate>{col.label}</a>
							</th>
						{/if}
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.listing.source + '/' + r.listing.source_id)}
					{@const ch = change(r)}
					<tr>
						{#each columns as col (col.key)}
							{#if visible(col)}
								<td class={col.cls}>
									{#if col.key === 'rank'}
										<span class="rank"><Star source={r.listing.source} id={r.listing.source_id} on={r.favorite} />{r.rank}</span>
									{:else if col.key === 'match'}
										{match(r.best.match, r.best.calibrated, r.best.stale)}
									{:else if col.key === 'score'}
										{r.best.score.toFixed(1)}
									{:else if col.key === 'coverage'}
										{percent(r.best.coverage)}
									{:else if col.key.startsWith('p:')}
										{@const g = (r.grades ?? []).find((g) => 'p:' + g.profile === col.key)}
										{g ? match(g.match, g.calibrated, g.stale) : '-'}
									{:else if col.key === 'price'}
										{money(r.listing.price.cents, r.listing.offer)}{#if ch}<span class="badge {ch.cls}">{ch.text}</span>{/if}
									{:else if col.key === 'beds'}
										{beds(r.listing.beds)}
									{:else if col.key === 'address'}
										<a href={listingHref(r)}>{address(r.listing.address) || r.listing.source_id}</a>
										{#if r.new}<span class="badge new">new</span>{:else if r.status === 'repeat'}<span class="badge repeat">repeat</span>{/if}
									{:else if col.key === 'sources'}
										<a class="ext" href={r.listing.url} target="_blank" rel="noopener">{r.listing.source}</a>
										{#each r.also_listed ?? [] as also (also.source + also.source_id)}
											· <a class="ext" href={also.url} target="_blank" rel="noopener">{also.source}</a>
										{/each}
									{:else}
										{col.value(r)}
									{/if}
								</td>
							{/if}
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else if !data.single && view.listings === 0}
	<p>Nothing was retrieved in this range. <a href={href({ day: 'latest', from: null, to: null })}>Show the latest run</a>.</p>
{:else if view.day || !data.single}
	<p>No graded listings match.</p>
{:else}
	<p>This collection hasn't run yet.</p>
{/if}
