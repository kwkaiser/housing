<script lang="ts">
	import ListingCard from '#lib/components/ListingCard.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Favorites · housing</title></svelte:head>

<div class="page-head"><h1>Favorites</h1></div>
{#if data.favorites.length}
	<ul class="cards">
		{#each data.favorites as f (f.listing.source + '/' + f.listing.source_id)}
			{@const l = f.listing}
			<li>
				<ListingCard
					listing={l}
					href="/listing/{encodeURIComponent(l.source)}/{encodeURIComponent(l.source_id)}"
					grade={f.best}
					favorite={true}
					note={f.last_seen ? `seen ${f.last_seen}` : undefined}
				/>
			</li>
		{/each}
	</ul>
{:else}
	<div class="card empty">
		<p>No favorites yet. Star a listing to keep it here.</p>
		<a class="button" href="/">Browse listings</a>
	</div>
{/if}
