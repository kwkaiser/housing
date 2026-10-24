<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Status from '#lib/components/Status.svelte';
	import { ago, cost, dateTime, elapsed, kindLabel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const statuses = ['queued', 'running', 'succeeded', 'degraded', 'failed', 'cancelled'];
	const kinds = ['run_collection', 'create_profile', 'draft_profile'];

	function href(changes: Record<string, string | undefined>): string {
		const q = new URLSearchParams(page.url.search);
		q.delete('before');
		for (const [k, v] of Object.entries(changes)) {
			if (v) q.set(k, v);
			else q.delete(k);
		}
		const s = q.toString();
		return `/jobs${s ? `?${s}` : ''}`;
	}

	function duration(started?: string, finished?: string): string {
		if (!started) return '-';
		return elapsed((finished ? Date.parse(finished) : Date.now()) - Date.parse(started));
	}
</script>

<svelte:head><title>Jobs · housing</title></svelte:head>

<h1>Jobs</h1>
<div class="controls">
	<select aria-label="Collection" value={data.query.collection ?? ''} onchange={(e) => goto(href({ collection: e.currentTarget.value }))}>
		<option value="">All collections</option>
		{#each data.collections as c (c.id)}<option value={c.id}>{c.id}</option>{/each}
	</select>
	<select aria-label="Kind" value={data.query.kind ?? ''} onchange={(e) => goto(href({ kind: e.currentTarget.value }))}>
		<option value="">All kinds</option>
		{#each kinds as k (k)}<option value={k}>{kindLabel(k)}</option>{/each}
	</select>
	<select aria-label="Status" value={data.query.status ?? ''} onchange={(e) => goto(href({ status: e.currentTarget.value }))}>
		<option value="">All statuses</option>
		{#each statuses as s (s)}<option value={s}>{s}</option>{/each}
	</select>
</div>

{#if data.jobs.length}
	<div class="wrap">
		<table class="jobs">
			<thead><tr><th class="num">#</th><th>Kind</th><th>Target</th><th>Status</th><th>Created</th><th class="num">Duration</th><th class="num">Cost</th><th>Error</th></tr></thead>
			<tbody>
				{#each data.jobs as j (j.id)}
					<tr>
						<td class="num"><a href="/jobs/{j.id}">{j.id}</a></td>
						<td>{kindLabel(j.kind)}<span class="muted">{j.trigger === 'schedule' ? ' · scheduled' : ''}</span></td>
						<td>
							{#if j.collection_id}<a href="/c/{encodeURIComponent(j.collection_id)}">{j.collection_id}</a>
							{:else if j.profile_id}<a href="/profiles/{encodeURIComponent(j.profile_id)}">{j.profile_id}</a>{/if}
						</td>
						<td><Status status={j.status} /></td>
						<td class="nowrap" title={dateTime(j.created_at)}>{ago(j.created_at)}</td>
						<td class="num">{duration(j.started_at, j.finished_at)}</td>
						<td class="num">{cost(j.cost_usd)}</td>
						<td class="bad">{j.error && j.error.length > 80 ? j.error.slice(0, 79) + '…' : (j.error ?? '')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
	<div class="pager">
		{#if page.url.searchParams.has('before')}<a class="button" href={href({})}>Newest</a>{/if}
		{#if data.next}<a class="button" href="{href({})}{href({}).includes('?') ? '&' : '?'}before={data.next}">Older →</a>{/if}
	</div>
{:else}
	<p>No jobs match.</p>
{/if}
