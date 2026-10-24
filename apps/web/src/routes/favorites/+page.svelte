<script lang="ts">
	import Star from '#lib/components/Star.svelte';
	import { address, beds, localDay, match, money } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Favorites · housing</title></svelte:head>

<h1>Favorites</h1>
{#if data.favorites.length}
	<div class="wrap">
		<table>
			<thead><tr><th></th><th class="num">Match</th><th class="num">Price</th><th class="num">Beds</th><th>Address</th><th>Sources</th><th>Last seen</th><th>Added</th></tr></thead>
			<tbody>
				{#each data.favorites as f (f.listing.source + '/' + f.listing.source_id)}
					{@const l = f.listing}
					<tr>
						<td><Star source={l.source} id={l.source_id} on={true} /></td>
						<td class="num score">{f.best ? match(f.best.match, f.best.calibrated, f.best.stale) : '-'}</td>
						<td class="num">{money(l.price.cents, l.offer)}</td>
						<td class="num">{beds(l.beds)}</td>
						<td><a href="/listing/{encodeURIComponent(l.source)}/{encodeURIComponent(l.source_id)}">{address(l.address) || l.source_id}</a></td>
						<td><a class="ext" href={l.url} target="_blank" rel="noopener">{l.source}</a></td>
						<td>{f.last_seen ?? '-'}</td>
						<td>{localDay(new Date(f.added_at))}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p>No favorites yet. Star a listing to keep it here.</p>
{/if}
