<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, call } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
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

<h1>Collections</h1>
<div class="controls"><a class="button" href="/collections/new">New collection</a></div>
<Errors {error} />
{#if data.collections.length}
	<div class="wrap">
		<table class="collections-table">
			<thead><tr><th>Collection</th><th>Mode</th><th>Sources</th><th>Location</th><th>Profiles</th><th>Schedule</th><th class="num">Budget</th><th>Last run</th><th></th></tr></thead>
			<tbody>
				{#each data.collections as c (c.id)}
					<tr>
						<td><a href="/c/{encodeURIComponent(c.id)}">{c.id}</a></td>
						<td>{c.mode}</td>
						<td>{(c.sources ?? []).join(', ')}</td>
						<td>{c.search.location ?? ''}{#if c.search.radius_miles} <span class="muted">+{c.search.radius_miles} mi</span>{/if}</td>
						<td>{(c.profiles ?? []).join(', ')}</td>
						<td>{c.schedule ? `daily at ${c.schedule}` : 'manual'}</td>
						<td class="num">{c.max_run_cost_usd ? `$${c.max_run_cost_usd.toFixed(2)}` : 'default'}</td>
						<td>
							{#if c.last_run}<a href="/jobs/{c.last_run.id}"><Status status={c.last_run.status} /></a> <span class="muted">{ago(c.last_run.finished_at ?? c.last_run.created_at)}</span>{:else}<span class="muted">never</span>{/if}
						</td>
						<td class="actions">
							<button type="button" onclick={() => run(c.id)} disabled={c.last_run?.status === 'queued' || c.last_run?.status === 'running'}>Run</button>
							<a class="button" href="/collections/{encodeURIComponent(c.id)}/edit">Edit</a>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p>No collections yet.</p>
{/if}
