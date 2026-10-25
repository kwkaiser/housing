<script lang="ts">
	import PlusIcon from '@lucide/svelte/icons/plus';
	import { dateTime, shortModel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>Profiles · housing</title></svelte:head>

<div class="page-head">
	<h1>Profiles</h1>
	<div class="actions"><a class="button primary" href="/profiles/new"><PlusIcon size={18} /> New</a></div>
</div>
{#if data.profiles.length}
	<ul class="cards">
		{#each data.profiles as p (p.id)}
			<li class="card item">
				<div class="head">
					<a class="stretch name" href="/profiles/{encodeURIComponent(p.id)}">{p.name || p.id}</a>
					<span class="pill {p.kind === 'avoid' ? 'bad' : 'good'}">{p.kind}</span>
				</div>
				<p class="line"><code>{p.id}</code> · {p.wants} wants · {p.avoids} avoids · {p.references} references</p>
				{#if p.collections?.length}
					<p class="line">
						Used by {#each p.collections as c, i (c)}{i ? ', ' : ''}<a href="/c/{encodeURIComponent(c)}">{c}</a>{/each}
					</p>
				{/if}
				{#if p.drafted}<p class="line">Drafted by {shortModel(p.drafted.model)} · {dateTime(p.drafted.at)}</p>{/if}
			</li>
		{/each}
	</ul>
{:else}
	<div class="card empty">
		<p>No profiles yet.</p>
		<a class="button primary" href="/profiles/new">Create a profile</a>
	</div>
{/if}
