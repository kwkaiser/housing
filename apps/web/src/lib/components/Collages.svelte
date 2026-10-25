<script lang="ts">
	import { sized } from '#lib/format.ts';

	let { srcs, size, alt }: { srcs: string[]; size: 640 | 1280; alt: string } = $props();
</script>

<div class="collages">
	{#each srcs as src (src)}
		<a href={src} target="_blank" rel="noopener"><img src={sized(src, size)} {alt} loading="lazy" /></a>
	{/each}
</div>

<style>
	.collages {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(min(100%, 28rem), 1fr));
		gap: 0.75rem;
		margin: 0 0 1.5rem;
	}

	img {
		display: block;
		width: 100%;
		height: auto;
		border-radius: var(--radius);
		background: var(--surface-2);
	}

	@media (width < 640px) {
		.collages {
			display: flex;
			overflow-x: auto;
			scroll-snap-type: x mandatory;
			margin-inline: calc(-1 * var(--gutter));
			padding-inline: var(--gutter);
			scroll-padding-inline: var(--gutter);
			scrollbar-width: none;
		}

		a {
			flex: 0 0 88%;
			scroll-snap-align: start;
		}

		a:only-child {
			flex-basis: 100%;
		}
	}
</style>
