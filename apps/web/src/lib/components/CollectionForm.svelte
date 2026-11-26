<script lang="ts">
	import { untrack } from 'svelte';
	import type { Collection, Meta, ProfileItem, Schemas } from '#lib/api/client.ts';
	import { ApiError } from '#lib/api/client.ts';
	import XIcon from '@lucide/svelte/icons/x';
	import Errors from './Errors.svelte';
	import RegionSelect from './RegionSelect.svelte';
	import { label } from '#lib/format.ts';

	type Body = Schemas['APICreateCollection'];

	let {
		meta,
		profiles,
		collection,
		submit
	}: { meta: Meta; profiles: ProfileItem[]; collection?: Collection; submit: (body: Body, clear: Schemas['APIUpdateCollection']['clear']) => Promise<void> } = $props();

	const initial = untrack(() => collection);
	const s = initial?.search;
	const num = (n: number | undefined) => (n === undefined || n === 0 ? '' : String(n));

	let id = $state(initial?.id ?? '');
	let mode = $state(initial?.mode ?? 'rent');
	let sources = $state<string[]>(initial?.sources ?? ['zillow']);
	let picked = $state<string[]>(initial?.profiles ?? []);
	let location = $state(s?.location ?? '');
	let radius = $state(num(s?.radius_miles));
	let regions = $state<Record<string, string[]>>({ ...(s?.regions ?? {}) });
	let byRegion = $state<Record<string, boolean>>(Object.fromEntries(Object.entries(s?.regions ?? {}).map(([src, ids]) => [src, ids.length > 0])));
	let minPrice = $state(num(s?.min_price));
	let maxPrice = $state(num(s?.max_price));
	let minBeds = $state(s?.min_beds === undefined ? '' : String(s.min_beds));
	let maxBeds = $state(s?.max_beds === undefined ? '' : String(s.max_beds));
	let maxAge = $state(num(s?.max_age_days));
	let amenities = $state<string[]>(s?.amenities ?? []);
	let limit = $state(initial ? num(s?.limit) : '50');
	let model = $state(initial?.model ?? '');
	let budget = $state(num(initial?.max_run_cost_usd));
	let schedule = $state(initial?.schedule ?? '');
	let scheduleInput = $state<HTMLInputElement>();
	let notifyURL = $state(initial?.notify_url ?? '');
	let notifyMin = $state(num(initial?.notify_min_score));
	let error = $state<unknown>();
	let busy = $state(false);

	let rentalTerms = $derived(meta.rental_terms ?? []);
	let general = $derived((meta.amenities ?? []).filter((a) => !rentalTerms.includes(a)));
	let groups = $derived([
		{ title: 'Required amenities', items: general },
		...(mode === 'rent' ? [{ title: 'Rental terms', items: rentalTerms }] : [])
	]);
	let wanted = $derived(amenities.filter((a) => mode === 'rent' || !rentalTerms.includes(a)));

	const levels: Record<string, { text: string; tone: string; title: string }> = {
		search: { text: 'search', tone: 'good', title: 'The source filters on this when searching' },
		results: { text: 'results', tone: 'accent', title: 'Checked against what search results report' },
		enrich: { text: 'details', tone: 'warn', title: 'Only known after fetching listing details' },
		'': { text: 'none', tone: 'bad', title: 'This source never reports it, so none of its listings will match' }
	};
	const support = (src: string, am: string) => levels[meta.capabilities?.[src]?.[mode]?.[am] ?? ''];
	let blocked = $derived(
		wanted.flatMap((am) => sources.filter((src) => support(src, am) === levels['']).map((src) => `${src} cannot report ${label(am)}`))
	);

	let wants = $derived(profiles.filter((p) => p.kind !== 'avoid'));
	let avoids = $derived(profiles.filter((p) => p.kind === 'avoid'));
	let fields = $derived(error instanceof ApiError ? error.fields : {});
	let regional = $derived(sources.filter((src) => (meta.regions?.[src]?.length ?? 0) > 0));
	let searchedByRegion = $derived(sources.filter((src) => byRegion[src] && regional.includes(src)));
	let emptyRegions = $derived(searchedByRegion.filter((src) => !regions[src]?.length));
	let radiusUnused = $derived(sources.length > 0 && searchedByRegion.length === sources.length);

	function toggle(list: string[], value: string, on: boolean): string[] {
		return on ? [...list.filter((v) => v !== value), value] : list.filter((v) => v !== value);
	}

	function move(i: number, by: number) {
		const next = [...picked];
		const [x] = next.splice(i, 1);
		next.splice(i + by, 0, x);
		picked = next;
	}

	function clearSchedule() {
		schedule = '';
		if (scheduleInput) scheduleInput.value = '';
	}

	const opt = (v: string) => (v.trim() === '' ? undefined : Number(v));

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		error = undefined;
		if (emptyRegions.length) {
			error = new Error(regional.length > 1 ? `Choose at least one region for ${emptyRegions.join(', ')}, or search by radius.` : 'Choose at least one region, or search by radius.');
			return;
		}
		busy = true;
		const body: Body = {
			id: id.trim(),
			mode: mode as Body['mode'],
			sources,
			profiles: picked,
			location,
			radius_miles: opt(radius) ?? 0,
			regions: Object.fromEntries(searchedByRegion.map((src) => [src, regions[src]])),
			min_price: opt(minPrice),
			max_price: opt(maxPrice),
			min_beds: opt(minBeds),
			max_beds: opt(maxBeds),
			max_age_days: opt(maxAge) ?? 0,
			amenities: wanted,
			limit: opt(limit) ?? 0,
			model,
			max_run_cost_usd: opt(budget) ?? 0,
			schedule,
			notify_url: notifyURL,
			notify_min_score: opt(notifyMin) ?? 0
		};
		const clear = (['min_price', 'max_price', 'min_beds', 'max_beds'] as const).filter((k) => body[k] === undefined);
		try {
			await submit(body, clear);
		} catch (err) {
			error = err;
		} finally {
			busy = false;
		}
	}
</script>

<Errors {error} />
<form class="form" {onsubmit}>
	<fieldset>
		<legend>Collection</legend>
		{#if !collection}
			<div class="field" class:invalid={fields.id}>
				<label for="id">ID</label>
				<input id="id" bind:value={id} required pattern="[a-z0-9-]+" autocapitalize="off" />
				<small>Lowercase letters, digits and dashes.</small>
				{#if fields.id}<p class="field-error">{fields.id}</p>{/if}
			</div>
		{/if}
		<div class="field">
			<span class="label">Mode</span>
			<div class="chips">
				{#each meta.modes ?? [] as m (m)}<label><input type="radio" name="mode" value={m} bind:group={mode} /> {m}</label>{/each}
			</div>
		</div>
		<div class="field" class:invalid={fields.sources}>
			<span class="label">Sources</span>
			<div class="chips">
				{#each meta.sources ?? [] as src (src)}
					<label><input type="checkbox" checked={sources.includes(src)} onchange={(e) => (sources = toggle(sources, src, e.currentTarget.checked))} /> {src}</label>
				{/each}
			</div>
			{#if fields.sources}<p class="field-error">{fields.sources}</p>{/if}
			{#each regional as src (src)}
				<div class="regions" class:invalid={fields.regions || emptyRegions.includes(src)}>
					{#if regional.length > 1}<span class="source">{src}</span>{/if}
					<label class="toggle">
						<input type="checkbox" aria-label="Search {src} by region instead of radius" bind:checked={() => byRegion[src] ?? false, (on) => (byRegion[src] = on)} />
						Search by region instead of radius
					</label>
					{#if byRegion[src]}
						<RegionSelect label="{src} regions" regions={meta.regions[src]} bind:selected={() => regions[src] ?? [], (ids) => (regions[src] = ids)} />
					{/if}
				</div>
			{/each}
			{#if fields.regions}<p class="field-error">{fields.regions}</p>{/if}
		</div>
		<div class="field" class:invalid={fields.profiles}>
			<span class="label">Want profiles</span>
			<small>Listings are graded against each, in this priority order.</small>
			<ul class="order">
				{#each picked as pid, i (pid)}
					<li>
						<input type="checkbox" checked aria-label="Use {pid}" onchange={() => (picked = picked.filter((p) => p !== pid))} />
						<span class="pos">{i + 1}</span>
						<span class="text">{wants.find((p) => p.id === pid)?.name || pid} <span class="muted">{pid}</span></span>
						<span class="moves">
							<button type="button" disabled={i === 0} onclick={() => move(i, -1)} aria-label="Move up">↑</button>
							<button type="button" disabled={i === picked.length - 1} onclick={() => move(i, 1)} aria-label="Move down">↓</button>
						</span>
					</li>
				{/each}
				{#each wants.filter((p) => !picked.includes(p.id)) as p (p.id)}
					<li class="off">
						<input type="checkbox" aria-label="Use {p.id}" onchange={() => (picked = [...picked, p.id])} />
						<span class="text">{p.name || p.id} <span class="muted">{p.id}</span></span>
					</li>
				{/each}
			</ul>
			{#if fields.profiles}<p class="field-error">{fields.profiles}</p>{/if}
			{#if avoids.length}
				<small>Avoid profiles apply to every collection:</small>
				<ul class="avoids">{#each avoids as p (p.id)}<li><a href="/profiles/{p.id}">{p.name || p.id}</a></li>{/each}</ul>
			{/if}
		</div>
	</fieldset>

	<fieldset>
		<legend>Search</legend>
		<div class="grid">
			<div class="field" class:invalid={fields['search.location'] || fields.location}>
				<label for="location">Location</label>
				<input id="location" bind:value={location} placeholder="ZIP code or city" disabled={radiusUnused} />
				{#if radiusUnused}<small>Not used: every source searches by region.</small>{:else if searchedByRegion.length}<small>Used by sources searched by radius.</small>{/if}
			</div>
			<div class="field"><label for="radius">Radius (miles)</label><input id="radius" inputmode="decimal" bind:value={radius} disabled={radiusUnused} /></div>
			<div class="field"><label for="min-price">Min price</label><input id="min-price" inputmode="numeric" bind:value={minPrice} /></div>
			<div class="field"><label for="max-price">Max price</label><input id="max-price" inputmode="numeric" bind:value={maxPrice} /></div>
			<div class="field"><label for="min-beds">Min beds</label><input id="min-beds" inputmode="numeric" bind:value={minBeds} /></div>
			<div class="field"><label for="max-beds">Max beds</label><input id="max-beds" inputmode="numeric" bind:value={maxBeds} /></div>
			<div class="field"><label for="max-age">Max listing age (days)</label><input id="max-age" inputmode="numeric" bind:value={maxAge} /></div>
			<div class="field"><label for="limit">Result limit per source</label><input id="limit" inputmode="numeric" bind:value={limit} /></div>
		</div>
		{#each groups as group, gi (group.title)}
			<div class="field" class:invalid={gi === 0 && fields.amenities}>
				<span class="label">{group.title}</span>
				<div class="chips">
					{#each group.items as am (am)}
						<label><input type="checkbox" checked={amenities.includes(am)} onchange={(e) => (amenities = toggle(amenities, am, e.currentTarget.checked))} /> {label(am)}</label>
					{/each}
				</div>
				{#if sources.length}
					<div class="support">
						<table>
							<thead>
								<tr><th scope="col"><span class="sr-only">Filter</span></th>{#each sources as src (src)}<th scope="col">{src}</th>{/each}</tr>
							</thead>
							<tbody>
								{#each group.items as am (am)}
									<tr class:off={!wanted.includes(am)}>
										<th scope="row">{label(am)}</th>
										{#each sources as src (src)}
											{@const lv = support(src, am)}
											<td><span class="pill {lv.tone}" title={lv.title}>{lv.text}</span></td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
				{#if gi === 0 && fields.amenities}<p class="field-error">{fields.amenities}</p>{/if}
			</div>
		{/each}
		{#if blocked.length}
			<p class="warning">Listings from these sources will all be dropped: {blocked.join('; ')}.</p>
		{/if}
		<small>Filters marked <strong>details</strong> cost more: each listing's page is fetched before it is checked.</small>
	</fieldset>

	<fieldset>
		<legend>Runs</legend>
		<div class="grid">
			<div class="field"><label for="model">Grading model</label><input id="model" bind:value={model} placeholder={meta.defaults.assess_model} /></div>
			<div class="field"><label for="budget">Max cost per run ($)</label><input id="budget" inputmode="decimal" bind:value={budget} placeholder={meta.defaults.collection_budget_usd.toFixed(2)} /></div>
			<div class="field" class:invalid={fields.schedule}>
				<label for="schedule">Daily run time</label>
				<div class="with-clear">
					<input id="schedule" type="time" bind:this={scheduleInput} bind:value={schedule} />
					<button type="button" class="ghost icon-button" aria-label="Clear daily run time" title="Don't run on a schedule" onclick={clearSchedule}><XIcon size={18} /></button>
				</div>
				<small>Leave empty for manual runs only.</small>
				{#if fields.schedule}<p class="field-error">{fields.schedule}</p>{/if}
			</div>
		</div>
		<div class="grid">
			<div class="field" class:invalid={fields['notify.url']}><label for="notify-url">ntfy topic URL</label><input id="notify-url" type="url" bind:value={notifyURL} /></div>
			<div class="field"><label for="notify-min">Notify at match ≥</label><input id="notify-min" inputmode="decimal" bind:value={notifyMin} placeholder={String(meta.defaults.notify_min_score)} /></div>
		</div>
	</fieldset>

	<div class="form-actions">
		<button class="primary" type="submit" disabled={busy}>{collection ? 'Save changes' : 'Create collection'}</button>
	</div>
</form>

<style>
	.regions {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 0.5rem;
		padding: 0.6rem 0.75rem;
		border: 1px solid var(--line);
		border-radius: var(--radius-sm);
		background: var(--surface-2);
	}

	.regions.invalid {
		border-color: var(--bad);
	}

	.source {
		font: var(--label);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--muted);
	}

	.toggle {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.with-clear {
		display: flex;
		gap: 0.25rem;
	}

	.with-clear input {
		flex: 1;
		min-width: 0;
	}

	.support {
		overflow-x: auto;
		margin-top: 0.5rem;
	}

	.support table {
		border-collapse: collapse;
		font-size: 0.85rem;
	}

	.support th,
	.support td {
		padding: 0.2rem 0.6rem 0.2rem 0;
		text-align: left;
		white-space: nowrap;
	}

	.support thead th {
		color: var(--muted);
		font-weight: 600;
	}

	.support tbody th {
		font-weight: 500;
	}

	.support tr.off {
		opacity: 0.55;
	}

	.warning {
		margin: 0;
		color: var(--warn);
	}

	.avoids {
		margin: 0;
		padding-left: 1rem;
	}

	.order {
		display: flex;
		flex-direction: column;
		margin: 0.25rem 0 0;
		padding: 0;
		list-style: none;
		border: 1px solid var(--line);
		border-radius: var(--radius-sm);
	}

	li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-height: 3rem;
		padding: 0.25rem 0.5rem 0.25rem 0.75rem;
		border-bottom: 1px solid var(--line);
	}

	li:last-child {
		border-bottom: 0;
	}

	li.off {
		color: var(--muted);
	}

	.text {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.text .muted {
		display: block;
		font-size: 0.8rem;
	}

	.pos {
		display: inline-grid;
		place-items: center;
		width: 1.5rem;
		height: 1.5rem;
		border-radius: 50%;
		background: var(--accent);
		color: var(--accent-fg);
		font-size: 0.75rem;
		font-weight: 700;
	}

	.moves {
		display: flex;
		gap: 0.25rem;
	}

	.moves button {
		width: 2.25rem;
		min-height: 2.25rem;
		padding: 0;
	}
</style>
