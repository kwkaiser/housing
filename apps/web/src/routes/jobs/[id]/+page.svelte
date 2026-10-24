<script lang="ts">
	import type { Job, JobEvent } from '#lib/api/client.ts';
	import Status from '#lib/components/Status.svelte';
	import { cost, dateTime, elapsed, kindLabel, label, time } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let job = $derived<Job>(data.job);
	let events = $derived<JobEvent[]>(data.events);
	let live = $state(false);
	let now = $state(Date.now());

	let active = $derived(job.status === 'queued' || job.status === 'running');

	$effect(() => {
		if (!active) return;
		const after = events.at(-1)?.seq ?? 0;
		const source = new EventSource(`/api/v1/jobs/${job.id}/stream?after=${after}`);
		live = true;
		source.addEventListener('event', (e) => {
			const ev = JSON.parse((e as MessageEvent).data) as JobEvent;
			if (!events.some((x) => x.seq === ev.seq)) events = [...events, ev];
		});
		source.addEventListener('job', (e) => {
			job = JSON.parse((e as MessageEvent).data) as Job;
			if (job.status !== 'queued' && job.status !== 'running') source.close();
		});
		source.onerror = () => {
			if (source.readyState === EventSource.CLOSED) live = false;
		};
		const tick = setInterval(() => (now = Date.now()), 1000);
		return () => {
			source.close();
			clearInterval(tick);
			live = false;
		};
	});

	function fields(v: unknown): [string, string][] {
		if (!v || typeof v !== 'object') return [];
		return Object.entries(v as Record<string, unknown>)
			.sort(([a], [b]) => a.localeCompare(b))
			.map(([k, x]) => [label(k), typeof x === 'string' ? x : typeof x === 'boolean' ? (x ? 'yes' : 'no') : JSON.stringify(x)]);
	}

	function attrs(e: JobEvent): [string, string][] {
		if (!e.record || typeof e.record !== 'object') return [];
		return Object.entries(e.record as Record<string, unknown>)
			.filter(([k]) => !['time', 'level', 'msg', 'stage'].includes(k))
			.map(([k, v]) => [k, typeof v === 'string' ? v : JSON.stringify(v)]);
	}

	function stage(e: JobEvent): string {
		const r = e.record as Record<string, unknown> | undefined;
		return typeof r?.stage === 'string' ? r.stage : '';
	}

	let duration = $derived(job.started_at ? elapsed((job.finished_at ? Date.parse(job.finished_at) : now) - Date.parse(job.started_at)) : '-');
	let result = $derived(job.result as Record<string, unknown> | undefined);
</script>

<svelte:head><title>Job {job.id} · housing</title></svelte:head>

<h1>Job {job.id} <Status status={job.status} /></h1>
{#if active}<p class="note running">{live ? 'Following live…' : 'Waiting for progress…'}</p>{/if}
<dl class="fields">
	<dt>Kind</dt><dd>{kindLabel(job.kind)} ({job.trigger})</dd>
	{#if job.collection_id}<dt>Collection</dt><dd><a href="/c/{encodeURIComponent(job.collection_id)}">{job.collection_id}</a></dd>{/if}
	{#if job.profile_id}<dt>Profile</dt><dd><a href="/profiles/{encodeURIComponent(job.profile_id)}">{job.profile_id}</a></dd>{/if}
	<dt>Created</dt><dd>{dateTime(job.created_at)}</dd>
	<dt>Duration</dt><dd>{duration}</dd>
	<dt>Cost</dt><dd>{cost(job.cost_usd)}</dd>
	{#each fields(job.params) as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}
</dl>
{#if job.error}<pre class="error">{job.error}</pre>{/if}

{#if result}
	<h2>Result</h2>
	{#if job.kind === 'run_collection' && typeof result.day === 'string' && job.collection_id}
		<p><a href="/c/{encodeURIComponent(job.collection_id)}?day={result.day}">View listings for {result.day}</a></p>
	{:else if typeof result.profile_id === 'string'}
		<p><a href="/profiles/{encodeURIComponent(result.profile_id)}">View profile {result.profile_id}</a></p>
	{/if}
	<dl class="fields">{#each fields(result) as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}</dl>
{/if}

<h2>Progress</h2>
{#if events.length}
	<div class="wrap">
		<table class="events">
			<tbody>
				{#each events as e (e.seq)}
					<tr class={e.level.toLowerCase()}>
						<td class="meta">{time(e.at)}{#if stage(e)}<div>{stage(e)}</div>{/if}</td>
						<td class="message">
							{#if e.level !== 'INFO'}<span class="level">{e.level}</span>{/if}
							{e.message}
							{#if attrs(e).length}
								<div class="attrs">{#each attrs(e) as [k, v] (k)}<span><span class="key">{k}</span> {v}</span>{/each}</div>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p>No progress logged yet.</p>
{/if}
