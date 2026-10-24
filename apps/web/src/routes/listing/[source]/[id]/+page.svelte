<script lang="ts">
	import type { Schemas } from '#lib/api/client.ts';
	import ListingMap from '#lib/components/ListingMap.svelte';
	import Star from '#lib/components/Star.svelte';
	import { address, baths, beds, dateTime, label, match, money, percent, shortModel, sized, sqft } from '#lib/format.ts';
	import { load, save } from '#lib/storage.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let d = $derived(data.detail);
	let l = $derived(d.listing);
	let title = $derived(address(l.address) || `${l.source} ${l.source_id}`);
	let favorite = $derived(d.favorite);
	let mapOpen = $state(load('listingMapOpen', false));

	function verdictClass(r: Schemas['APICriterionResult'], avoid: boolean): string {
		if (!avoid) return r.verdict;
		if (r.verdict === 'present') return 'avoid-present';
		if (r.verdict === 'absent') return 'avoid-absent';
		return r.verdict;
	}

	let back = $derived(d.collection ? `/c/${encodeURIComponent(d.collection)}${d.day ? `?day=${d.day}` : ''}` : '/');
</script>

<svelte:head><title>{title} · housing</title></svelte:head>

<p><a href={back}>← Back</a></p>
<h1 class="starred"><Star source={l.source} id={l.source_id} bind:on={favorite} />{title}</h1>
<p class="facts">
	{money(l.price.cents, l.offer)} · {beds(l.beds)} bd · {baths(l.baths)} ba · {sqft(l.sqft)} ·
	<a class="ext" href={l.url} target="_blank" rel="noopener">{l.source}</a>
	{#each d.also_listed ?? [] as also (also.source + also.source_id)}
		· <a class="ext" href={also.url} target="_blank" rel="noopener">{also.source} ({money(also.price.cents, also.offer)})</a>
	{/each}
</p>
{#if d.day}<p>As seen on {d.day}{#if d.collection} in {d.collection}{/if}.</p>{/if}

{#if l.collages?.length}
	<div class="collages">
		{#each l.collages as src (src)}
			<a href={src} target="_blank" rel="noopener"><img src={sized(src, 1280)} alt="Listing collage" loading="lazy" /></a>
		{/each}
	</div>
{/if}

{#if d.location}
	<details class="section" bind:open={mapOpen} ontoggle={() => save('listingMapOpen', mapOpen)}>
		<summary>Map{#if d.location.approx} <span class="muted">(approximate, from the zip code)</span>{/if}</summary>
		{#if mapOpen}<ListingMap config={data.config} location={d.location} {title} />{/if}
	</details>
{/if}

<h2>Grades</h2>
{#each d.grades ?? [] as g (g.profile)}
	<section class="grade">
		<header>
			<div class="match"><span class="score">{match(g.match, g.calibrated, g.stale)}</span><span class="muted">match</span></div>
			<div class="who">
				<a href="/profiles/{encodeURIComponent(g.profile)}">{g.profile_name || g.profile}</a>
				<div class="muted">{g.profile}</div>
			</div>
			<dl class="stats">
				<div><dt>Score</dt><dd>{g.score.toFixed(1)}</dd></div>
				<div><dt>Coverage</dt><dd>{percent(g.coverage)}</dd></div>
			</dl>
		</header>
		{#if g.summary}<p class="assessment">{g.summary}</p>{/if}
		<div class="pills">
			{#each g.dealbreakers ?? [] as x (x)}<span class="pill bad">dealbreaker: {x}</span>{/each}
			{#each g.missing_essentials ?? [] as x (x)}<span class="pill">missing: {x}</span>{/each}
			{#each g.avoids_hit ?? [] as x (x)}<span class="pill bad">avoid: {x}</span>{/each}
		</div>
		{#each g.models ?? [] as m (m.model)}
			<details class="section">
				<summary>{shortModel(m.model)} · {m.score.toFixed(1)} · {percent(m.coverage)} coverage{#if m.stale} · stale{/if} <span class="muted">{dateTime(m.assessed_at)}</span></summary>
				<div class="wrap">
					<table class="criteria">
						<thead><tr><th>Criterion</th><th>Importance</th><th>Verdict</th><th>Confidence</th><th>Evidence</th></tr></thead>
						<tbody>
							{#each [{ name: 'Wants', avoid: false, rows: m.wants ?? [] }, { name: 'Avoids', avoid: true, rows: m.avoids ?? [] }] as group (group.name)}
								{#if group.rows.length}
									<tr class="group"><th colspan="5">{group.name}</th></tr>
									{#each group.rows as r (r.id)}
										<tr>
											<td>{r.label}{#if r.dealbreaker} <span class="pill bad">dealbreaker</span>{/if}</td>
											<td>{r.importance}</td>
											<td class="verdict {verdictClass(r, group.avoid)}">{r.verdict}</td>
											<td>{r.confidence}</td>
											<td class="summary">{r.evidence}{#if r.photos?.length} <span class="muted">(photos {r.photos.join(', ')})</span>{/if}</td>
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
	<p>Not graded yet.</p>
{/each}

{#if l.description}
	<h2>Description</h2>
	<p class="note" style="white-space: pre-wrap">{l.description}</p>
{/if}

{#if l.amenities && Object.keys(l.amenities).length}
	<h2>Amenities</h2>
	<div class="pills">
		{#each Object.entries(l.amenities) as [k, v] (k)}<span class="pill {v ? 'good' : 'bad'}">{label(k)}</span>{/each}
	</div>
{/if}

{#if d.history?.length}
	<h2>Price history</h2>
	<table class="history">
		<thead><tr><th>Day</th><th class="num">Price</th></tr></thead>
		<tbody>
			{#each d.history as h (h.day)}<tr><td>{h.day}</td><td class="num">{money(h.price_cents, l.offer)}</td></tr>{/each}
		</tbody>
	</table>
{/if}
