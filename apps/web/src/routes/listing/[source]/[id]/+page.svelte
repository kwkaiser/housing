<script lang="ts">
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import type { Schemas } from '#lib/api/client.ts';
	import Back from '#lib/components/Back.svelte';
	import Collages from '#lib/components/Collages.svelte';
	import Gallery from '#lib/components/Gallery.svelte';
	import ListingMap from '#lib/components/ListingMap.svelte';
	import Match from '#lib/components/Match.svelte';
	import Star from '#lib/components/Star.svelte';
	import { address, baths, beds, dateTime, label, money, percent, shortModel, sqft, type Tone } from '#lib/format.ts';
	import { load, save } from '#lib/storage.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let d = $derived(data.detail);
	let l = $derived(d.listing);
	let title = $derived(address(l.address) || `${l.source} ${l.source_id}`);
	let favorite = $derived(d.favorite);
	let mapOpen = $state(load('listingMapOpen', false));
	let collagesOpen = $state(false);

	function verdictTone(r: Schemas['APICriterionResult'], avoid: boolean): Tone {
		if (r.verdict === 'present') return avoid ? 'bad' : 'good';
		if (r.verdict === 'absent') return avoid ? 'good' : 'bad';
		return 'muted';
	}

	let back = $derived(d.collection ? `/c/${encodeURIComponent(d.collection)}${d.day ? `?day=${d.day}` : ''}` : '/');
</script>

<svelte:head><title>{title} · housing</title></svelte:head>

<Back href={back} label={d.collection} />
<header class="hero">
	<h1><Star source={l.source} id={l.source_id} bind:on={favorite} />{title}</h1>
	<dl class="facts">
		<div><dt class="label">Price</dt><dd>{money(l.price.cents, l.offer)}</dd></div>
		<div><dt class="label">Beds</dt><dd>{beds(l.beds)}</dd></div>
		<div><dt class="label">Baths</dt><dd>{baths(l.baths)}</dd></div>
		{#if l.sqft}<div><dt class="label">Size</dt><dd>{sqft(l.sqft)}</dd></div>{/if}
	</dl>
	<div class="links">
		<a class="button" href={l.url} target="_blank" rel="noopener">{l.source} <ExternalLinkIcon size={14} /></a>
		{#each d.also_listed ?? [] as also (also.source + also.source_id)}
			<a class="button" href={also.url} target="_blank" rel="noopener">{also.source} · {money(also.price.cents, also.offer)} <ExternalLinkIcon size={14} /></a>
		{/each}
	</div>
	{#if d.day}<p class="summary-line">As seen on {d.day}{d.collection ? ` in ${d.collection}` : ''}.</p>{/if}
</header>

{#if d.gallery.length}
	<Gallery srcs={d.gallery} alt={title} />
{:else if l.collages?.length}
	<Collages srcs={l.collages} size={1280} alt="Listing collage" />
{/if}

{#if d.location}
	<details class="section" bind:open={mapOpen} ontoggle={() => save('listingMapOpen', mapOpen)}>
		<summary>Map{#if d.location.approx}<span class="muted sub">approximate, from the zip code</span>{/if}</summary>
		{#if mapOpen}<ListingMap config={data.config} location={d.location} {title} />{/if}
	</details>
{/if}

{#if d.gallery.length && l.collages?.length}
	<details class="section" bind:open={collagesOpen}>
		<summary>Grading collages<span class="muted sub">what the models saw</span></summary>
		{#if collagesOpen}<Collages srcs={l.collages} size={1280} alt="Listing collage" />{/if}
	</details>
{/if}

<h2>Grades</h2>
{#each d.grades ?? [] as g (g.profile)}
	<section class="card grade">
		<header>
			<Match value={g.match} calibrated={g.calibrated} stale={g.stale} size="lg" />
			<div class="who">
				<a href="/profiles/{encodeURIComponent(g.profile)}">{g.profile_name || g.profile}</a>
				<div class="muted">{g.profile}</div>
			</div>
		</header>
		<dl class="stats">
			<div><dt class="label">Score</dt><dd>{g.score.toFixed(1)}</dd></div>
			<div><dt class="label">Coverage</dt><dd>{percent(g.coverage)}</dd></div>
		</dl>
		{#if g.summary}<p class="note">{g.summary}</p>{/if}
		<div class="pills">
			{#each g.dealbreakers ?? [] as x (x)}<span class="pill bad">dealbreaker: {x}</span>{/each}
			{#each g.missing_essentials ?? [] as x (x)}<span class="pill">missing: {x}</span>{/each}
			{#each g.avoids_hit ?? [] as x (x)}<span class="pill bad">avoid: {x}</span>{/each}
		</div>
		{#each g.models ?? [] as m (m.model)}
			<details class="section">
				<summary>
					{shortModel(m.model)} · {m.score.toFixed(1)} · {percent(m.coverage)}{m.stale ? ' · stale' : ''}
					<span class="muted sub block">{dateTime(m.assessed_at)}</span>
				</summary>
				<div class="table-wrap">
					<table class="stack">
						<thead><tr><th>Criterion</th><th>Importance</th><th>Verdict</th><th>Confidence</th><th>Evidence</th></tr></thead>
						<tbody>
							{#each [{ name: 'Wants', avoid: false, rows: m.wants ?? [] }, { name: 'Avoids', avoid: true, rows: m.avoids ?? [] }] as group (group.name)}
								{#if group.rows.length}
									<tr class="group"><th colspan="5">{group.name}</th></tr>
									{#each group.rows as r (r.id)}
										<tr>
											<td>{r.label}{#if r.dealbreaker}{' '}<span class="pill bad">dealbreaker</span>{/if}</td>
											<td data-label="Importance">{r.importance}</td>
											<td data-label="Verdict" class="verdict {verdictTone(r, group.avoid)}">{r.verdict}</td>
											<td data-label="Confidence">{r.confidence}</td>
											<td data-label="Evidence" class="summary">{r.evidence}{r.photos?.length ? ` (photos ${r.photos.join(', ')})` : ''}</td>
										</tr>
									{/each}
								{/if}
							{/each}
						</tbody>
					</table>
				</div>
			</details>
		{/each}
	</section>
{:else}
	<div class="card empty"><p>Not graded yet.</p></div>
{/each}

{#if l.description}
	<h2>Description</h2>
	<p class="note description">{l.description}</p>
{/if}

{#if l.amenities && Object.keys(l.amenities).length}
	<h2>Amenities</h2>
	<div class="pills">
		{#each Object.entries(l.amenities) as [k, v] (k)}<span class="pill {v ? 'good' : 'bad'}">{label(k)}</span>{/each}
	</div>
{/if}

{#if d.history?.length}
	<h2>Price history</h2>
	<div class="table-wrap history">
		<table>
			<thead><tr><th>Day</th><th class="num">Price</th></tr></thead>
			<tbody>
				{#each d.history as h (h.day)}<tr><td>{h.day}</td><td class="num">{money(h.price_cents, l.offer)}</td></tr>{/each}
			</tbody>
		</table>
	</div>
{/if}

<style>
	.hero {
		margin: 0 0 1.25rem;
	}

	h1 {
		display: flex;
		align-items: flex-start;
		gap: 0.25rem;
	}

	h1 :global(button) {
		margin: -0.35rem 0 0 -0.6rem;
	}

	dl {
		display: flex;
		flex-wrap: wrap;
		margin: 0;
	}

	dd {
		margin: 0;
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.facts {
		gap: 0.5rem 1.5rem;
		margin-top: 0.75rem;
	}

	.facts dd {
		font-size: 1.1rem;
	}

	.links {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin: 0.9rem 0 0;
	}

	.hero .summary-line {
		margin: 0.75rem 0 0;
	}

	.section {
		margin: 0 0 1rem;
	}

	.section > summary {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 0.5rem;
		min-height: var(--control);
		padding: 0.35rem 0;
		cursor: pointer;
		user-select: none;
		font-weight: 600;
		list-style: none;
	}

	.section > summary::-webkit-details-marker {
		display: none;
	}

	.section > summary::before {
		content: '';
		flex: none;
		width: 0.45rem;
		height: 0.45rem;
		border: solid var(--muted);
		border-width: 0 2px 2px 0;
		transform: rotate(-45deg);
		transition: transform 0.15s;
	}

	.section[open] > summary::before {
		transform: rotate(45deg);
	}

	.section[open] > summary {
		margin-bottom: 0.5rem;
	}

	.sub {
		font-weight: 400;
		font-size: 0.85rem;
	}

	.sub.block {
		flex-basis: 100%;
		padding-left: 0.95rem;
	}

	.grade {
		margin: 0 0 1rem;
		padding: 1rem;
	}

	.grade header {
		display: flex;
		align-items: center;
		gap: 0.9rem;
		margin: 0 0 0.75rem;
	}

	.who {
		flex: 1;
		min-width: 0;
	}

	.who a {
		font-size: 1.02rem;
		font-weight: 600;
	}

	.who .muted {
		font-size: 0.8rem;
	}

	.stats {
		gap: 1.25rem;
		margin-bottom: 0.75rem;
	}

	.grade .section {
		margin: 0;
		border-top: 1px solid var(--line);
	}

	.grade .table-wrap {
		box-shadow: none;
	}

	tr.group th {
		background: var(--surface-2);
	}

	.verdict.good {
		color: var(--good);
	}

	.verdict.bad {
		color: var(--bad);
	}

	.verdict.muted {
		color: var(--muted);
	}

	.description {
		white-space: pre-wrap;
	}

	.history {
		width: fit-content;
		max-width: 100%;
	}

	.history table {
		min-width: 16rem;
	}
</style>
