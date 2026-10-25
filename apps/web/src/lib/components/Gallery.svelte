<script lang="ts">
	import ChevronLeftIcon from '@lucide/svelte/icons/chevron-left';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import XIcon from '@lucide/svelte/icons/x';
	import { sized } from '#lib/format.ts';

	let { srcs, alt }: { srcs: string[]; alt: string } = $props();

	let index = $state(0);
	let fullscreen = $state(false);
	let stage: HTMLDivElement;
	let viewer = $state<HTMLDivElement>();
	let dialog: HTMLDialogElement;
	let strip = $state<HTMLDivElement>();

	$effect(() => {
		if (fullscreen && !dialog.open) {
			dialog.showModal();
			if (viewer) jump(viewer, index, 'instant');
		} else if (!fullscreen && dialog.open) dialog.close();
	});

	$effect(() => {
		strip?.children[index]?.scrollIntoView({ block: 'nearest', inline: 'nearest', behavior: 'smooth' });
	});

	function jump(track: HTMLElement, i: number, behavior: ScrollBehavior = 'smooth') {
		track.scrollTo({ left: i * track.clientWidth, behavior });
	}

	function go(i: number) {
		index = Math.max(0, Math.min(srcs.length - 1, i));
		jump(stage, index);
		if (viewer && fullscreen) jump(viewer, index);
	}

	function onscroll(e: Event) {
		const track = e.currentTarget as HTMLElement;
		index = Math.round(track.scrollLeft / track.clientWidth);
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft') go(index - 1);
		else if (e.key === 'ArrowRight') go(index + 1);
		else return;
		e.preventDefault();
	}

	function close() {
		fullscreen = false;
		jump(stage, index, 'instant');
	}
</script>

{#snippet nav()}
	{#if srcs.length > 1}
		<button type="button" class="nav prev" aria-label="Previous photo" disabled={index === 0} onclick={() => go(index - 1)}><ChevronLeftIcon size={22} /></button>
		<button type="button" class="nav next" aria-label="Next photo" disabled={index === srcs.length - 1} onclick={() => go(index + 1)}><ChevronRightIcon size={22} /></button>
	{/if}
	<span class="count">{index + 1} / {srcs.length}</span>
{/snippet}

<div class="gallery">
	<div class="frame">
		<div class="track" bind:this={stage} {onscroll}>
			{#each srcs as src, i (src)}
				<button type="button" class="slide" aria-label="View photo {i + 1} full screen" {onkeydown} onclick={() => (fullscreen = true)}>
					<img src={sized(src, 1280)} alt="{alt} {i + 1}" loading={i === 0 ? 'eager' : 'lazy'} />
				</button>
			{/each}
		</div>
		{@render nav()}
	</div>
	{#if srcs.length > 1}
		<div class="strip" bind:this={strip}>
			{#each srcs as src, i (src)}
				<button type="button" class="thumb" class:active={i === index} aria-label="Photo {i + 1}" aria-current={i === index} {onkeydown} onclick={() => go(i)}>
					<img src={sized(src, 320)} alt="" loading="lazy" />
				</button>
			{/each}
		</div>
	{/if}
</div>

<dialog bind:this={dialog} class="viewer" onclose={close} {onkeydown} aria-label="{alt} photos">
	{#if fullscreen}
		<div class="track" bind:this={viewer} {onscroll}>
			{#each srcs as src, i (src)}
				<div class="slide"><img src={sized(src, 1280)} alt="{alt} {i + 1}" loading="lazy" /></div>
			{/each}
		</div>
		{@render nav()}
		<button type="button" class="close" aria-label="Close" onclick={() => (fullscreen = false)}><XIcon size={22} /></button>
	{/if}
</dialog>

<style>
	.gallery {
		margin: 0 0 1.5rem;
	}

	.frame {
		position: relative;
		overflow: hidden;
		border-radius: var(--radius);
		background: #000;
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

	.frame .slide:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: -2px;
	}

	.slide {
		flex: 0 0 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 0;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: none;
		scroll-snap-align: start;
		scroll-snap-stop: always;
		cursor: zoom-in;
	}

	.frame .slide {
		aspect-ratio: 4 / 3;
		max-height: min(70dvh, 40rem);
	}

	.slide img {
		display: block;
		max-width: 100%;
		max-height: 100%;
		object-fit: contain;
	}

	.frame .slide img {
		width: 100%;
		height: 100%;
	}

	.nav,
	.close {
		position: absolute;
		display: grid;
		place-items: center;
		width: 2.5rem;
		min-height: 2.5rem;
		padding: 0;
		border: 0;
		border-radius: 999px;
		background: rgb(0 0 0 / 0.5);
		color: #fff;
		backdrop-filter: blur(4px);
	}

	.nav {
		top: 50%;
		transform: translateY(-50%);
		transition: opacity 0.15s;
	}

	.nav:disabled {
		opacity: 0;
		pointer-events: none;
	}

	.prev {
		left: 0.5rem;
	}

	.next {
		right: 0.5rem;
	}

	.count {
		position: absolute;
		right: 0.6rem;
		bottom: 0.6rem;
		padding: 0.15rem 0.55rem;
		border-radius: 999px;
		background: rgb(0 0 0 / 0.55);
		color: #fff;
		font-size: 0.8rem;
		font-variant-numeric: tabular-nums;
		pointer-events: none;
	}

	.strip {
		display: flex;
		gap: 0.4rem;
		margin-top: 0.5rem;
		overflow-x: auto;
		scrollbar-width: none;
	}

	.strip::-webkit-scrollbar {
		display: none;
	}

	.thumb {
		flex: 0 0 auto;
		width: 4.5rem;
		min-height: 0;
		height: 3.4rem;
		padding: 0;
		overflow: hidden;
		border: 2px solid transparent;
		border-radius: var(--radius-sm);
		background: var(--surface-2);
		opacity: 0.6;
		transition: opacity 0.15s;
	}

	.thumb.active {
		border-color: var(--accent);
		opacity: 1;
	}

	.thumb img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.viewer {
		width: 100vw;
		max-width: none;
		height: 100dvh;
		max-height: none;
		margin: 0;
		padding: 0;
		border: 0;
		background: #000;
		overscroll-behavior: contain;
	}

	.viewer::backdrop {
		background: #000;
	}

	.viewer .track {
		height: 100%;
	}

	.viewer .slide {
		cursor: default;
	}

	.viewer .count {
		bottom: calc(0.9rem + var(--safe-bottom));
		left: 50%;
		right: auto;
		transform: translateX(-50%);
	}

	.close {
		top: calc(0.6rem + var(--safe-top));
		right: calc(0.6rem + var(--safe-right));
	}

	@media (pointer: coarse) {
		.frame .nav {
			display: none;
		}
	}

	@media (width < 640px) {
		.gallery {
			margin-inline: calc(-1 * var(--gutter));
		}

		.frame {
			border-radius: 0;
		}

		.strip {
			padding-inline: var(--gutter);
		}
	}
</style>
