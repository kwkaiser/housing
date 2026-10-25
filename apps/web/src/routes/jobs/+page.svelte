<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import Status from '#lib/components/Status.svelte';
	import { ago, cost, dateTime, elapsed, kindLabel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const statuses = ['queued', 'running', 'succeeded', 'degraded', 'failed', 'cancelled'];
	const kinds = ['run_collection', 'create_profile', 'draft_profile', 'prune_media'];

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

<div class="page-head"><h1>Jobs</h1></div>
<div class="toolbar">
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
	<ul class="cards single">
		{#each data.jobs as j (j.id)}
			<li class="card item">
				<div class="head">
					<a class="stretch name" href="/jobs/{j.id}">{kindLabel(j.kind)} <span class="muted">#{j.id}</span></a>
					<Status status={j.status} />
				</div>
				<p class="line">
					{#if j.collection_id}<a href="/c/{encodeURIComponent(j.collection_id)}">{j.collection_id}</a> · {:else if j.profile_id}<a href="/profiles/{encodeURIComponent(j.profile_id)}">{j.profile_id}</a> · {/if}<span title={dateTime(j.created_at)}>{ago(j.created_at)}</span>{j.trigger === 'schedule' ? ' · scheduled' : ''} · {duration(j.started_at, j.finished_at)} · {cost(j.cost_usd)}
				</p>
				{#if j.error}<p class="line bad">{j.error.length > 140 ? j.error.slice(0, 139) + '…' : j.error}</p>{/if}
			</li>
		{/each}
	</ul>
	<div class="pager">
		{#if page.url.searchParams.has('before')}<a class="button" href={href({})}>Newest</a>{/if}
		{#if data.next}<a class="button" href="{href({})}{href({}).includes('?') ? '&' : '?'}before={data.next}">Older <ChevronRightIcon size={16} /></a>{/if}
	</div>
{:else}
	<div class="card empty"><p>No jobs match.</p></div>
{/if}

<style>
	.pager {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		margin: 1rem 0;
	}
</style>
