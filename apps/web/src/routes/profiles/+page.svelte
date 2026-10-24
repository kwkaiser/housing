<script lang="ts">
	import { dateTime, shortModel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Profiles · housing</title></svelte:head>

<h1>Profiles</h1>
<div class="controls"><a class="button" href="/profiles/new">New profile</a></div>
{#if data.profiles.length}
	<div class="wrap">
		<table class="profiles-table">
			<thead><tr><th>Profile</th><th>Kind</th><th class="num">Wants</th><th class="num">Avoids</th><th class="num">References</th><th>Collections</th><th>Drafted</th></tr></thead>
			<tbody>
				{#each data.profiles as p (p.id)}
					<tr>
						<td><a href="/profiles/{encodeURIComponent(p.id)}">{p.name || p.id}</a> <code class="muted">{p.id}</code></td>
						<td><span class="kind {p.kind}">{p.kind}</span></td>
						<td class="num">{p.wants}</td>
						<td class="num">{p.avoids}</td>
						<td class="num">{p.references}</td>
						<td>
							{#each p.collections ?? [] as c, i (c)}{i ? ', ' : ''}<a href="/c/{encodeURIComponent(c)}">{c}</a>{/each}
						</td>
						<td class="muted">{p.drafted ? `${shortModel(p.drafted.model)} · ${dateTime(p.drafted.at)}` : ''}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p>No profiles yet.</p>
{/if}
