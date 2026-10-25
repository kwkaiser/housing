<script lang="ts">
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlayIcon from '@lucide/svelte/icons/play';
	import SlidersHorizontalIcon from '@lucide/svelte/icons/sliders-horizontal';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, call, type ListingRow } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import ListingCard from '#lib/components/ListingCard.svelte';
	import Sheet from '#lib/components/Sheet.svelte';
	import Match from '#lib/components/Match.svelte';
	import RangeSelect from '#lib/components/RangeSelect.svelte';
	import Star from '#lib/components/Star.svelte';
	import Status from '#lib/components/Status.svelte';
	import { address, ago, match, money, beds, percent, type Tag } from '#lib/format.ts';
	import { wide } from '#lib/media.ts';
	import { load, save } from '#lib/storage.ts';
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
	let filtersOpen = $state(false);
	let newOnly = $derived(page.url.searchParams.get('new') === '1');
	let hideRepeats = $derived(page.url.searchParams.get('repeats') === '0');
	let activeFilters = $derived([withDealbreakers, newOnly, hideRepeats].filter(Boolean).length);
	let running = $derived(c.last_run?.status === 'queued' || c.last_run?.status === 'running');

	const sorts = [
		{ value: '', label: 'Rank' },
		{ value: 'match:desc', label: 'Best match' },
		{ value: 'price:asc', label: 'Price: low to high' },
		{ value: 'price:desc', label: 'Price: high to low' },
		{ value: 'beds:desc', label: 'Most beds' },
		{ value: 'coverage:desc', label: 'Coverage' }
	];

	function sortBy(value: string) {
		const [sort, dir] = value.split(':');
		goto(href({ sort: sort || null, dir: dir || null }), { reset: false, replace: true });
	}

	type Column = { key: string; label: string; cls?: string; off?: boolean; desc?: boolean; value: (r: ListingRow) => number | string };

	let columns = $derived.by((): Column[] => {
		const grade = (r: ListingRow, id: string) => (r.grades ?? []).find((g) => g.profile === id);
		const cols: Column[] = [
			{ key: 'rank', label: '#', cls: 'num', value: (r) => r.rank },
			{ key: 'match', label: 'Match', cls: 'num', desc: true, value: (r) => r.best.match },
			{ key: 'score', label: 'Score', cls: 'num', desc: true, value: (r) => r.best.score }
		];
		if (multi) {
			cols.push({ key: 'profile', label: 'Profile', cls: 'nowrap', value: (r) => r.best.profile });
			for (const id of profiles) cols.push({ key: `p:${id}`, label: id, cls: 'num', desc: true, value: (r) => grade(r, id)?.match ?? -1 });
		}
		cols.push(
			{ key: 'coverage', label: 'Coverage', cls: 'num', desc: true, value: (r) => r.best.coverage },
			{ key: 'price', label: 'Price', cls: 'num', value: (r) => r.listing.price.cents },
			{ key: 'beds', label: 'Beds', cls: 'num', value: (r) => r.listing.beds ?? -1 },
			{ key: 'address', label: 'Address', cls: 'addr', value: (r) => address(r.listing.address).toLowerCase() },
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
	let sortValue = $derived(sortKey && !(sortKey === 'rank' && !desc) ? `${sortKey}:${desc ? 'desc' : 'asc'}` : '');
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

	function change(r: ListingRow): Tag | null {
		if (r.previous_price_cents !== undefined) {
			const diff = r.listing.price.cents - r.previous_price_cents;
			if (diff > 0) return { text: `↑ ${money(diff)}`, tone: 'bad' };
			if (diff < 0) return { text: `↓ ${money(-diff)}`, tone: 'good' };
		}
		return r.status === 'changed' ? { text: 'updated', tone: 'muted' } : null;
	}

	function seen(r: ListingRow): Tag | null {
		if (r.new) return { text: 'new', tone: 'accent' };
		return r.status === 'repeat' ? { text: 'repeat', tone: 'muted' } : null;
	}

	function listingHref(r: ListingRow): string {
		const q = new URLSearchParams({ collection: c.id });
		if (r.day) q.set('day', r.day);
		return `/listing/${encodeURIComponent(r.listing.source)}/${encodeURIComponent(r.listing.source_id)}?${q}`;
	}
</script>

<svelte:head><title>{c.id} · housing</title></svelte:head>

<div class="page-head">
	<h1>
		{#if data.collections.length > 1}
			<select class="title-select" aria-label="Collection" value={c.id} onchange={(e) => goto(`/c/${encodeURIComponent(e.currentTarget.value)}`)}>
				{#each data.collections as x (x.id)}<option value={x.id}>{x.id}</option>{/each}
			</select>
		{:else}
			{c.id}
		{/if}
	</h1>
	<div class="actions">
		<button type="button" onclick={run} disabled={running}><PlayIcon size={16} /> Run now</button>
		<a class="button icon-button" href="/collections/{encodeURIComponent(c.id)}/edit" aria-label="Edit collection" title="Edit collection"><PencilIcon size={18} /></a>
	</div>
</div>

<div class="toolbar">
	<RangeSelect
		from={data.from}
		to={data.to}
		{extras}
		active={preset}
		onrange={(from, to) => goto(rangeHref(from, to))}
		onextra={(value) => value === 'latest' && goto(href({ day: 'latest', from: null, to: null }))}
	/>
	{#if data.single && view.day}
		<span class="stepper">
			{#if view.previous_day}
				<a class="button" href={href({ day: view.previous_day })} aria-label="Previous run, {view.previous_day}"><ChevronLeftIcon size={18} /></a>
			{:else}
				<span class="button" aria-disabled="true"><ChevronLeftIcon size={18} /></span>
			{/if}
			<span class="day">{view.day}</span>
			{#if view.next_day}
				<a class="button" href={href({ day: view.next_day })} aria-label="Next run, {view.next_day}"><ChevronRightIcon size={18} /></a>
			{:else}
				<span class="button" aria-disabled="true"><ChevronRightIcon size={18} /></span>
			{/if}
		</span>
	{/if}
	<button type="button" onclick={() => (filtersOpen = true)}>
		<SlidersHorizontalIcon size={16} /> Filters{#if activeFilters}<span class="count">{activeFilters}</span>{/if}
	</button>
	{#if wide.current}
		<details class="picker right">
			<summary>Columns</summary>
			<div class="menu">
				{#each columns as col (col.key)}
					<label><input type="checkbox" checked={visible(col)} onchange={(e) => toggleColumn(col.key, e.currentTarget.checked)} /> {col.label}</label>
				{/each}
			</div>
		</details>
	{:else}
		<select aria-label="Sort" value={sortValue} onchange={(e) => sortBy(e.currentTarget.value)}>
			{#each sorts as s (s.value)}<option value={s.value}>{s.label}</option>{/each}
		</select>
	{/if}
</div>

<p class="summary-line">
	{#if !data.single}
		{view.listings} retrieved {data.from ? `since ${data.from}` : 'on any date'}{data.to ? ` through ${data.to}` : ''} · {view.graded} graded{view.hidden ? ` · ${view.hidden} hidden for dealbreakers` : ''}
	{:else if view.day}
		{view.listings} listings · {view.graded} graded{view.hidden ? ` · ${view.hidden} hidden for dealbreakers` : ''}
	{:else}
		Never run
	{/if}
	{#if c.last_run}
		· <a href="/jobs/{c.last_run.id}"><Status status={c.last_run.status} /></a>
		{ago(c.last_run.finished_at ?? c.last_run.created_at)}
	{/if}
</p>
<Errors error={runError} />
{#if view.stale}<p class="note">* Some grades predate a change to the profile or listing and will be refreshed on the next run.</p>{/if}

<Sheet bind:open={filtersOpen} title="Filters" done>
	<div class="option-list">
		<label>Show dealbreakers <input type="checkbox" checked={withDealbreakers} onchange={(e) => goto(href({ dealbreakers: e.currentTarget.checked ? '1' : null }), { reset: false })} /></label>
		<label>New only <input type="checkbox" checked={newOnly} onchange={(e) => goto(href({ new: e.currentTarget.checked ? '1' : null }), { reset: false })} /></label>
		<label>Hide repeats <input type="checkbox" checked={hideRepeats} onchange={(e) => goto(href({ repeats: e.currentTarget.checked ? '0' : null }), { reset: false })} /></label>
	</div>
</Sheet>

{#if rows.length && !wide.current}
	<ul class="cards">
		{#each rows as r (r.listing.source + '/' + r.listing.source_id)}
			<li>
				<ListingCard
					listing={r.listing}
					href={listingHref(r)}
					grade={r.best}
					favorite={r.favorite}
					also={r.also_listed}
					tags={[change(r), seen(r)].filter((t) => t !== null)}
					note={multi ? r.best.profile : undefined}
				/>
			</li>
		{/each}
	</ul>
{:else if rows.length}
	<div class="table-wrap">
		<table>
			<thead>
				<tr>
					{#each columns as col (col.key)}
						{#if visible(col)}
							<th class={col.cls} aria-sort={col.key === sortKey ? (desc ? 'descending' : 'ascending') : undefined}>
								<a href={sortHref(col)} data-sveltekit-reset="false" data-sveltekit-replacestate>{col.label}</a>
							</th>
						{/if}
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.listing.source + '/' + r.listing.source_id)}
					{@const ch = change(r)}
					{@const tag = seen(r)}
					<tr>
						{#each columns as col (col.key)}
							{#if visible(col)}
								<td class={col.cls}>
									{#if col.key === 'rank'}
										<span class="rank"><Star source={r.listing.source} id={r.listing.source_id} on={r.favorite} compact />{r.rank}</span>
									{:else if col.key === 'match'}
										<Match value={r.best.match} calibrated={r.best.calibrated} stale={r.best.stale} />
									{:else if col.key === 'score'}
										{r.best.score.toFixed(1)}
									{:else if col.key === 'coverage'}
										{percent(r.best.coverage)}
									{:else if col.key.startsWith('p:')}
										{@const g = (r.grades ?? []).find((g) => 'p:' + g.profile === col.key)}
										{g ? match(g.match, g.calibrated, g.stale) : '-'}
									{:else if col.key === 'price'}
										{money(r.listing.price.cents, r.listing.offer)}
										{#if ch}<span class="pill {ch.tone}">{ch.text}</span>{/if}
									{:else if col.key === 'beds'}
										{beds(r.listing.beds)}
									{:else if col.key === 'address'}
										<a href={listingHref(r)}>{address(r.listing.address) || r.listing.source_id}</a>
										{#if tag}<span class="pill {tag.tone}">{tag.text}</span>{/if}
									{:else if col.key === 'sources'}
										{#each [r.listing, ...(r.also_listed ?? [])] as x, i (x.source + x.source_id)}
											{i ? ' · ' : ''}<a class="ext" href={x.url} target="_blank" rel="noopener">{x.source}</a>
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
	<div class="card empty">
		<p>Nothing was retrieved in this range.</p>
		<a class="button" href={href({ day: 'latest', from: null, to: null })}>Show the latest run</a>
	</div>
{:else if view.day || !data.single}
	<div class="card empty"><p>No graded listings match.</p></div>
{:else}
	<div class="card empty">
		<p>This collection hasn't run yet.</p>
		<button type="button" class="primary" onclick={run} disabled={running}><PlayIcon size={16} /> Run now</button>
	</div>
{/if}

<style>
	.title-select {
		field-sizing: content;
		max-width: 100%;
		min-height: 0;
		padding: 0 1.9rem 0 0;
		border: 0;
		background-color: transparent;
		background-position: right 0.1rem center;
		font-size: inherit;
		font-weight: inherit;
		letter-spacing: inherit;
	}

	.stepper {
		display: inline-flex;
		align-items: center;
		border: 1px solid var(--line);
		border-radius: 10px;
		background: var(--surface);
	}

	.stepper .button {
		width: var(--control);
		padding: 0;
		border: 0;
		background: none;
	}

	.day {
		padding: 0 0.35rem;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.rank {
		display: inline-flex;
		align-items: center;
		gap: 0.1rem;
	}

	td.addr {
		min-width: 16rem;
	}

	.ext,
	.tags {
		color: var(--muted);
	}
</style>
