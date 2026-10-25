<script lang="ts">
	import { goto } from '$app/navigation';
	import Back from '#lib/components/Back.svelte';
	import { api, call, type Criterion } from '#lib/api/client.ts';
	import Collages from '#lib/components/Collages.svelte';
	import Gallery from '#lib/components/Gallery.svelte';
	import Errors from '#lib/components/Errors.svelte';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import Status from '#lib/components/Status.svelte';
	import { ago, dateTime, kindLabel, lines, shortModel } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let p = $derived(data.detail.profile);
	let kind = $derived(p.kind || 'want');
	let inherited = $derived.by(() => {
		const groups: { id: string; name: string; criteria: Criterion[] }[] = [];
		for (const c of data.detail.inherited_avoids ?? []) {
			const last = groups.at(-1);
			if (last && last.id === c.profile) last.criteria.push(c);
			else groups.push({ id: c.profile, name: c.profile_name, criteria: [c] });
		}
		return groups;
	});

	let model = $state('');
	let notes = $state('');
	let error = $state<unknown>();
	let busy = $state(false);
	$effect(() => {
		notes = (data.detail.profile.notes ?? []).join('\n');
	});

	async function redraft(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = undefined;
		try {
			const job = await call(
				api().POST('/api/v1/profiles/{id}/drafts', { params: { path: { id: p.id } }, body: { model: model || undefined, notes: lines(notes) } })
			);
			goto(`/jobs/${job.id}`);
		} catch (err) {
			error = err;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{p.name || p.id} · housing</title></svelte:head>

<Back href="/profiles" label="Profiles" />
<div class="page-head">
	<h1>{p.name || p.id} <span class="pill {kind === 'avoid' ? 'bad' : 'good'}">{kind}</span></h1>
	<div class="actions"><a class="button" href="/profiles/{encodeURIComponent(p.id)}/edit"><PencilIcon size={16} /> Edit</a></div>
</div>
<p class="summary-line">
	<code>{p.id}</code>
	{p.drafted ? ` · Drafted by ${shortModel(p.drafted.model)} · ${dateTime(p.drafted.at)}` : ''}
	{#if data.job}{' · '}<a href="/jobs/{data.job.id}">{kindLabel(data.job.kind)} <Status status={data.job.status} /></a> {ago(data.job.created_at)}{/if}
</p>
{#if p.summary}<p class="note">{p.summary}</p>{/if}
{#if data.detail.collections?.length}
	<p>
		Used by
		{#each data.detail.collections as c, i (c)}{i ? ', ' : ''}<a href="/c/{encodeURIComponent(c)}">{c}</a>{/each}.
	</p>
{/if}

{#each [{ name: 'Wants', rows: p.want ?? [] }, { name: 'Avoids', rows: p.avoid ?? [] }] as group (group.name)}
	{#if group.rows.length}
		<h2>{group.name}</h2>
		<div class="table-wrap">
			<table class="stack">
				<thead><tr><th>Criterion</th><th>Importance</th><th>Look for</th><th>Evidence</th></tr></thead>
				<tbody>
					{#each group.rows as c (c.id)}
						<tr>
							<td>{c.label} <code class="muted">{c.id}</code></td>
							<td data-label="Importance">{c.importance}</td>
							<td data-label="Look for" class="summary">{c.look_for}{#if c.not_this}<div class="muted">Not: {c.not_this}</div>{/if}</td>
							<td data-label="Evidence">{c.evidence}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/each}

{#each inherited as g (g.id)}
	<h2>Avoids from <a href="/profiles/{encodeURIComponent(g.id)}">{g.name || g.id}</a></h2>
	<ul>{#each g.criteria as c (c.id)}<li>{c.label} <span class="muted">({c.importance})</span></li>{/each}</ul>
{/each}

{#if p.notes?.length}
	<h2>Drafting notes</h2>
	<ul>{#each p.notes as n, i (i)}<li>{n}</li>{/each}</ul>
{/if}

{#if data.detail.references?.length}
	<h2>References</h2>
	{#each data.detail.references as r (r.source + r.source_id)}
		<div class="reference">
			<p>
				<a href="/listing/{encodeURIComponent(r.source)}/{encodeURIComponent(r.source_id)}">{r.address || r.source_id}</a>
				· <a class="muted" href={r.url} target="_blank" rel="noopener">{r.source}</a>
				{#each Object.entries(r.scores) as [m, s] (m)} · <span class="muted">{shortModel(m)} {s.toFixed(1)}</span>{/each}
			</p>
			{#if r.gallery.length}
				<Gallery srcs={r.gallery} alt={r.address || r.source_id} />
			{:else if r.collages?.length}
				<Collages srcs={r.collages} size={640} alt="Reference collage" />
			{/if}
		</div>
	{/each}

	<h2>Re-draft criteria</h2>
	<p>Replaces this profile's criteria with a fresh draft from its references.</p>
	<Errors {error} />
	<form class="form" onsubmit={redraft}>
		<fieldset>
			<div class="field"><label for="model">Model</label><input id="model" bind:value={model} placeholder={data.meta.defaults.draft_model} /></div>
			<div class="field"><label for="notes">Drafting notes</label><textarea id="notes" rows="4" bind:value={notes}></textarea></div>
		</fieldset>
		<div class="form-actions"><button type="submit" disabled={busy}>Re-draft</button></div>
	</form>
{/if}

<style>
	ul {
		max-width: 70ch;
		margin: 0 0 1rem;
		padding-left: 1.2rem;
	}

	li {
		margin: 0.2rem 0;
	}

	.reference {
		margin: 0 0 1.25rem;
	}

	.reference p {
		margin-bottom: 0.5rem;
	}
</style>
