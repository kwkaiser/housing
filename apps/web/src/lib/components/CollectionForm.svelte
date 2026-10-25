<script lang="ts">
	import { untrack } from 'svelte';
	import type { Collection, Meta, ProfileItem, Schemas } from '#lib/api/client.ts';
	import { ApiError } from '#lib/api/client.ts';
	import Errors from './Errors.svelte';
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
	let notifyURL = $state(initial?.notify_url ?? '');
	let notifyMin = $state(num(initial?.notify_min_score));
	let error = $state<unknown>();
	let busy = $state(false);

	let wants = $derived(profiles.filter((p) => p.kind !== 'avoid'));
	let avoids = $derived(profiles.filter((p) => p.kind === 'avoid'));
	let fields = $derived(error instanceof ApiError ? error.fields : {});

	function toggle(list: string[], value: string, on: boolean): string[] {
		return on ? [...list.filter((v) => v !== value), value] : list.filter((v) => v !== value);
	}

	function move(i: number, by: number) {
		const next = [...picked];
		const [x] = next.splice(i, 1);
		next.splice(i + by, 0, x);
		picked = next;
	}

	const opt = (v: string) => (v.trim() === '' ? undefined : Number(v));

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = undefined;
		const body: Body = {
			id: id.trim(),
			mode: mode as Body['mode'],
			sources,
			profiles: picked,
			location,
			radius_miles: opt(radius) ?? 0,
			min_price: opt(minPrice),
			max_price: opt(maxPrice),
			min_beds: opt(minBeds),
			max_beds: opt(maxBeds),
			max_age_days: opt(maxAge) ?? 0,
			amenities,
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
				<input id="location" bind:value={location} placeholder="ZIP code or city" />
			</div>
			<div class="field"><label for="radius">Radius (miles)</label><input id="radius" inputmode="decimal" bind:value={radius} /></div>
			<div class="field"><label for="min-price">Min price</label><input id="min-price" inputmode="numeric" bind:value={minPrice} /></div>
			<div class="field"><label for="max-price">Max price</label><input id="max-price" inputmode="numeric" bind:value={maxPrice} /></div>
			<div class="field"><label for="min-beds">Min beds</label><input id="min-beds" inputmode="numeric" bind:value={minBeds} /></div>
			<div class="field"><label for="max-beds">Max beds</label><input id="max-beds" inputmode="numeric" bind:value={maxBeds} /></div>
			<div class="field"><label for="max-age">Max listing age (days)</label><input id="max-age" inputmode="numeric" bind:value={maxAge} /></div>
			<div class="field"><label for="limit">Result limit per source</label><input id="limit" inputmode="numeric" bind:value={limit} /></div>
		</div>
		<div class="field">
			<span class="label">Required amenities</span>
			<div class="chips">
				{#each meta.amenities ?? [] as am (am)}
					<label><input type="checkbox" checked={amenities.includes(am)} onchange={(e) => (amenities = toggle(amenities, am, e.currentTarget.checked))} /> {label(am)}</label>
				{/each}
			</div>
		</div>
	</fieldset>

	<fieldset>
		<legend>Runs</legend>
		<div class="grid">
			<div class="field"><label for="model">Grading model</label><input id="model" bind:value={model} placeholder={meta.defaults.assess_model} /></div>
			<div class="field"><label for="budget">Max cost per run ($)</label><input id="budget" inputmode="decimal" bind:value={budget} placeholder={meta.defaults.collection_budget_usd.toFixed(2)} /></div>
			<div class="field" class:invalid={fields.schedule}>
				<label for="schedule">Daily run time</label>
				<input id="schedule" type="time" bind:value={schedule} />
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
