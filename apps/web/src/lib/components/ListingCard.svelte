<script lang="ts">
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import type { Grade, Listing, Schemas } from '#lib/api/client.ts';
	import { address, baths, beds, money, sized, sqft, type Tag } from '#lib/format.ts';
	import Match from './Match.svelte';
	import Star from './Star.svelte';

	let {
		listing: l,
		href,
		grade,
		favorite,
		also = [],
		tags = [],
		note
	}: {
		listing: Listing;
		href: string;
		grade?: Grade;
		favorite: boolean;
		also?: Schemas['Link'][];
		tags?: Tag[];
		note?: string;
	} = $props();

	let failed = $state(false);
	let src = $derived([failed ? '' : l.thumbnail, l.collages?.[0]].find(Boolean));
	let specs = $derived(
		[l.beds === 0 ? 'Studio' : l.beds !== undefined ? `${beds(l.beds)} bd` : '', l.baths !== undefined ? `${baths(l.baths)} ba` : '', l.sqft ? sqft(l.sqft) : '']
			.filter(Boolean)
			.join(' · ')
	);
	let dealbreakers = $derived(grade?.dealbreakers ?? []);
</script>

<article class="card">
	<div class="thumb">
		{#if src}
			<img src={sized(src, 320)} class:collage={src !== l.thumbnail} alt="" loading="lazy" onerror={() => (failed = true)} />
		{/if}
		{#if grade}
			<span class="score"><Match value={grade.match} calibrated={grade.calibrated} stale={grade.stale} size="sm" /></span>
		{/if}
	</div>
	<div class="body">
		<div class="top">
			<div>
				<span class="price">{money(l.price.cents, l.offer)}</span>
				{#each tags as t (t.text)}<span class="pill {t.tone}">{t.text}</span>{/each}
			</div>
			<span class="star"><Star source={l.source} id={l.source_id} on={favorite} /></span>
		</div>
		{#if specs}<div class="specs">{specs}</div>{/if}
		<a class="stretch addr" {href}>{address(l.address) || l.source_id}</a>
		{#if dealbreakers.length}<div class="warn">Dealbreaker: {dealbreakers.join(', ')}</div>{/if}
		<div class="meta">
			{#each [l, ...also] as x (x.source + x.source_id)}
				<a href={x.url} target="_blank" rel="noopener">{x.source} <ExternalLinkIcon size={11} /></a>
			{/each}
			{#if note}<span>· {note}</span>{/if}
		</div>
	</div>
</article>

<style>
	article {
		position: relative;
		display: grid;
		grid-template-columns: 6rem 1fr;
		gap: 0.85rem;
		padding: 0.7rem;
	}

	article:has(.stretch:hover) {
		border-color: color-mix(in srgb, var(--muted) 45%, var(--line));
	}

	.thumb {
		position: relative;
		width: 6rem;
		height: 6rem;
		border-radius: 10px;
		overflow: hidden;
		background: var(--surface-2);
	}

	img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.collage {
		object-position: top left;
	}

	.score {
		position: absolute;
		left: 0.3rem;
		bottom: 0.3rem;
	}

	.body {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.top {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.top .pill {
		margin-left: 0.3rem;
	}

	.price {
		font-size: 1.1rem;
		font-weight: 700;
		letter-spacing: -0.01em;
		font-variant-numeric: tabular-nums;
	}

	.star {
		position: relative;
		z-index: 1;
		margin: -0.4rem -0.4rem 0 0;
	}

	.specs {
		font-size: 0.875rem;
	}

	.addr {
		color: var(--muted);
		font-size: 0.875rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.warn {
		font-size: 0.8rem;
		color: var(--bad);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem 0.5rem;
		margin-top: 0.2rem;
		font-size: 0.78rem;
		color: var(--muted);
	}

	.meta a {
		position: relative;
		z-index: 1;
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		color: var(--muted);
	}
</style>
