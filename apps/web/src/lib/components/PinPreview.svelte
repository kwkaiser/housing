<script lang="ts" module>
	import { api, type MapPin } from '#lib/api/client.ts';

	const galleries = new Map<string, Promise<string[]>>();

	export function gallery(pin: MapPin): Promise<string[]> {
		const key = `${pin.source}/${pin.source_id}`;
		let p = galleries.get(key);
		if (!p) {
			p = api()
				.GET('/api/v1/listings/{source}/{source_id}/gallery', { params: { path: { source: pin.source, source_id: pin.source_id } } })
				.then(({ data }) => data?.gallery ?? [])
				.catch(() => {
					galleries.delete(key);
					return [];
				});
			galleries.set(key, p);
		}
		return p;
	}
</script>

<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import { beds, match, money, sized } from '#lib/format.ts';

	let { pin, lead }: { pin: MapPin; lead: string } = $props();

	let srcs = $state<string[]>(untrack(() => (pin.preview ? [pin.preview] : [])));
	let index = $state(0);
	let track = $state<HTMLDivElement>();

	let href = $derived(`/listing/${encodeURIComponent(pin.source)}/${encodeURIComponent(pin.source_id)}?collection=${encodeURIComponent(lead)}&day=${pin.last_seen}`);
	let g = $derived(pin.grades[lead]);
	let bd = $derived(pin.beds === undefined ? null : pin.beds === 0 ? 'studio' : `${beds(pin.beds)} bd`);
	let retrieved = $derived(pin.last_seen !== pin.first_seen ? ` · last retrieved ${pin.last_seen}` : '');

	onMount(() => {
		let live = true;
		gallery(pin).then((g) => {
			if (live && g.length) srcs = g;
		});
		return () => {
			live = false;
		};
	});

	function go(i: number) {
		if (!track) return;
		index = Math.max(0, Math.min(srcs.length - 1, i));
		track.scrollTo({ left: index * track.clientWidth, behavior: 'smooth' });
	}

	function onscroll() {
		if (track) index = Math.round(track.scrollLeft / track.clientWidth);
	}
</script>

{#if srcs.length}
	<div class="frame">
		<div class="track" bind:this={track} {onscroll}>
			{#each srcs as src, i (src)}
				<a class="slide" {href} tabindex={i === index ? 0 : -1}><img src={sized(src, 640)} alt="" loading={i <= index + 2 ? 'eager' : 'lazy'} /></a>
			{/each}
		</div>
		{#if srcs.length > 1}
			<button type="button" class="nav prev" aria-label="Previous photo" disabled={index === 0} onclick={() => go(index - 1)}><ChevronLeftIcon size={18} /></button>
			<button type="button" class="nav next" aria-label="Next photo" disabled={index === srcs.length - 1} onclick={() => go(index + 1)}><ChevronRightIcon size={18} /></button>
			<span class="count">{index + 1} / {srcs.length}</span>
		{/if}
	</div>
{/if}
<a {href}>{pin.address}</a>
<div>{[g ? match(g.match, g.calibrated, g.stale) : 'Not graded yet', money(pin.price_cents, pin.offer), bd].filter(Boolean).join(' · ')}</div>
<div class="muted">First seen {pin.first_seen}{retrieved} · {(pin.collections ?? []).join(', ')}</div>
{#if g?.dealbreakers?.length}<div class="bad">Dealbreakers: {g.dealbreakers.join(', ')}</div>{/if}
{#if pin.approx}<div class="muted">Approximate: placed at the zip code's center</div>{/if}

<style>
	.frame {
		position: relative;
		margin-bottom: 0.5rem;
		overflow: hidden;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
	}

	.track {
		display: flex;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		overscroll-behavior-x: contain;
		scrollbar-width: none;
	}

	.track::-webkit-scrollbar {
		display: none;
	}

	.slide {
		flex: 0 0 100%;
		aspect-ratio: 4 / 3;
		scroll-snap-align: start;
		scroll-snap-stop: always;
	}

	.slide img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
		object-position: top;
	}

	.nav {
		position: absolute;
		top: 50%;
		display: grid;
		place-items: center;
		width: 2rem;
		min-height: 2rem;
		padding: 0;
		border: 0;
		border-radius: 999px;
		background: rgb(0 0 0 / 0.5);
		color: #fff;
		transform: translateY(-50%);
		opacity: 0;
		transition: opacity 0.15s;
	}

	.frame:hover .nav:not(:disabled) {
		opacity: 1;
	}

	.nav:disabled {
		pointer-events: none;
	}

	.prev {
		left: 0.4rem;
	}

	.next {
		right: 0.4rem;
	}

	.count {
		position: absolute;
		right: 0.4rem;
		bottom: 0.4rem;
		padding: 0.1rem 0.45rem;
		border-radius: 999px;
		background: rgb(0 0 0 / 0.55);
		color: #fff;
		font-size: 0.75rem;
		font-variant-numeric: tabular-nums;
		pointer-events: none;
	}

	@media (pointer: coarse) {
		.nav {
			display: none;
		}
	}
</style>
