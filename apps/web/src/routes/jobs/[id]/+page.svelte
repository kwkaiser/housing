<script lang="ts">
	import Back from '#lib/components/Back.svelte';
	import type { Job, JobEvent } from '#lib/api/client.ts';
	import Status from '#lib/components/Status.svelte';
	import { cost, dateTime, elapsed, kindLabel, label, time, type Tone } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let job = $derived<Job>(data.job);
	let events = $derived<JobEvent[]>(data.events);
	let live = $state(false);
	let now = $state(Date.now());

	const levelTones: Record<string, Tone> = { WARN: 'warn', ERROR: 'bad' };

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

<Back href="/jobs" label="Jobs" />
<div class="page-head">
	<h1>{kindLabel(job.kind)} <span class="muted">#{job.id}</span></h1>
	<Status status={job.status} />
</div>
{#if active}<p class="live">{live ? 'Following live…' : 'Waiting for progress…'}</p>{/if}
<dl class="card">
	<dt>Trigger</dt><dd>{job.trigger}</dd>
	{#if job.collection_id}<dt>Collection</dt><dd><a href="/c/{encodeURIComponent(job.collection_id)}">{job.collection_id}</a></dd>{/if}
	{#if job.profile_id}<dt>Profile</dt><dd><a href="/profiles/{encodeURIComponent(job.profile_id)}">{job.profile_id}</a></dd>{/if}
	<dt>Created</dt><dd>{dateTime(job.created_at)}</dd>
	<dt>Duration</dt><dd>{duration}</dd>
	<dt>Cost</dt><dd>{cost(job.cost_usd)}</dd>
	{#each fields(job.params) as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}
</dl>
{#if job.error}<pre class="failure">{job.error}</pre>{/if}

{#if result}
	<h2>Result</h2>
	{#if job.kind === 'run_collection' && typeof result.day === 'string' && job.collection_id}
		<p><a class="button primary" href="/c/{encodeURIComponent(job.collection_id)}?day={result.day}">View listings for {result.day}</a></p>
	{:else if typeof result.profile_id === 'string'}
		<p><a class="button primary" href="/profiles/{encodeURIComponent(result.profile_id)}">View profile {result.profile_id}</a></p>
	{/if}
	<dl class="card">{#each fields(result) as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}</dl>
{/if}

<h2>Progress</h2>
{#if events.length}
	<div class="card events">
		{#each events as e (e.seq)}
			<div class="ev {e.level.toLowerCase()}">
				<div class="meta">{time(e.at)}{#if stage(e)}<div>{stage(e)}</div>{/if}</div>
				<div class="message">
					{#if e.level !== 'INFO'}<span class="pill {levelTones[e.level] ?? 'muted'}">{e.level}</span>{/if}{e.message}
					{#if attrs(e).length}
						<div class="attrs">{#each attrs(e) as [k, v] (k)}<span><span class="key">{k}</span> {v}</span>{/each}</div>
					{/if}
				</div>
			</div>
		{/each}
	</div>
{:else}
	<div class="card empty"><p>No progress logged yet.</p></div>
{/if}

<style>
	.live {
		color: var(--accent);
	}

	dl {
		display: grid;
		grid-template-columns: max-content 1fr;
		gap: 0.5rem 1.25rem;
		margin: 0 0 1rem;
		padding: 1rem;
	}

	dt {
		color: var(--muted);
	}

	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}

	@media (width < 640px) {
		dl {
			grid-template-columns: 1fr;
			gap: 0;
		}

		dt {
			margin-top: 0.6rem;
			font: var(--label);
			text-transform: uppercase;
			letter-spacing: 0.05em;
		}

		dt:first-child {
			margin-top: 0;
		}
	}

	.failure {
		margin: 0 0 1rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--bad) 10%, var(--surface));
		color: var(--bad);
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		font-size: 0.85rem;
	}

	.events {
		padding: 0.25rem 0;
	}

	.ev {
		display: grid;
		grid-template-columns: 4.75rem 1fr;
		gap: 0.75rem;
		padding: 0.5rem 1rem;
		border-bottom: 1px solid var(--line);
		font-size: 0.875rem;
	}

	.ev:last-child {
		border-bottom: 0;
	}

	.meta {
		color: var(--muted);
		font-size: 0.78rem;
		font-variant-numeric: tabular-nums;
	}

	.message {
		min-width: 0;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}

	.pill {
		margin-right: 0.35rem;
		font-size: 0.68rem;
	}

	.attrs {
		display: flex;
		flex-wrap: wrap;
		gap: 0.1rem 0.9rem;
		margin-top: 0.2rem;
		color: var(--muted);
		font-size: 0.78rem;
		white-space: normal;
	}

	.key {
		opacity: 0.75;
	}

	.warn .message {
		color: var(--warn);
	}

	.error .message {
		color: var(--bad);
	}

	.debug .message {
		color: var(--muted);
	}
</style>
