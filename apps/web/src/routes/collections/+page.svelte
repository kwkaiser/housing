<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, call } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlayIcon from '@lucide/svelte/icons/play';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Status from '#lib/components/Status.svelte';
	import { ago } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let error = $state<unknown>();

	async function run(id: string) {
		error = undefined;
		try {
			const job = await call(api().POST('/api/v1/collections/{id}/runs', { params: { path: { id } } }));
			goto(`/jobs/${job.id}`);
		} catch (e) {
			error = e;
		}
	}
</script>

<svelte:head><title>Collections · housing</title></svelte:head>

<div class="page-head">
	<h1>Collections</h1>
	<div class="actions"><a class="button primary" href="/collections/new"><PlusIcon size={18} /> New</a></div>
</div>
<Errors {error} />
{#if data.collections.length}
	<ul class="cards">
		{#each data.collections as c (c.id)}
			<li class="card item">
				<div class="head">
					<a class="stretch name" href="/c/{encodeURIComponent(c.id)}">{c.id}</a>
					<span class="pill">{c.mode}</span>
				</div>
				<p class="line">
					{c.search.location ?? ''}{#if c.search.radius_miles} +{c.search.radius_miles} mi{/if} · {(c.sources ?? []).join(', ')}
				</p>
				{#if c.profiles?.length}<p class="line">Profiles: {c.profiles.join(', ')}</p>{/if}
				<p class="line">
					{c.schedule ? `Daily at ${c.schedule}` : 'Manual runs'} · {c.max_run_cost_usd ? `$${c.max_run_cost_usd.toFixed(2)} budget` : 'default budget'}
				</p>
				<div class="foot">
					{#if c.last_run}
						<a href="/jobs/{c.last_run.id}"><Status status={c.last_run.status} /></a>
						<span class="muted">{ago(c.last_run.finished_at ?? c.last_run.created_at)}</span>
					{:else}
						<span class="muted">Never run</span>
					{/if}
					<span class="actions">
						<button type="button" onclick={() => run(c.id)} disabled={c.last_run?.status === 'queued' || c.last_run?.status === 'running'}><PlayIcon size={16} /> Run</button>
						<a class="button icon-button" href="/collections/{encodeURIComponent(c.id)}/edit" aria-label="Edit {c.id}"><PencilIcon size={18} /></a>
					</span>
				</div>
			</li>
		{/each}
	</ul>
{:else}
	<div class="card empty">
		<p>No collections yet.</p>
		<a class="button primary" href="/collections/new">Create a collection</a>
	</div>
{/if}
