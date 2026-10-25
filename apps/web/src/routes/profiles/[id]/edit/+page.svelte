<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import Back from '#lib/components/Back.svelte';
	import { api, ApiError, call, type Criterion, type Schemas } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import { lines } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const p = untrack(() => data.detail.profile);

	type Row = { id: string; label: string; importance: string; remove: boolean; original: Criterion };
	const rows = (cs: Criterion[] | null | undefined): Row[] => (cs ?? []).map((c) => ({ id: c.id, label: c.label, importance: c.importance, remove: false, original: c }));

	let name = $state(p.name);
	let summary = $state(p.summary);
	let notes = $state((p.notes ?? []).join('\n'));
	let want = $state(rows(p.want));
	let avoid = $state(rows(p.avoid));
	let error = $state<unknown>();
	let busy = $state(false);
	let fields = $derived(error instanceof ApiError ? error.fields : {});

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = undefined;
		const criteria: Schemas['CriterionPatch'][] = [...want, ...avoid]
			.filter((r) => r.remove || r.label !== r.original.label || r.importance !== r.original.importance)
			.map((r) => ({ id: r.id, label: r.label, importance: r.importance as Schemas['CriterionPatch']['importance'], remove: r.remove }));
		try {
			await call(api().PATCH('/api/v1/profiles/{id}', { params: { path: { id: p.id } }, body: { name, summary, notes: lines(notes), criteria } }));
			goto(`/profiles/${encodeURIComponent(p.id)}`);
		} catch (err) {
			error = err;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Edit {p.id} · housing</title></svelte:head>

<Back href="/profiles/{encodeURIComponent(p.id)}" label={p.name || p.id} />
<div class="page-head"><h1>Edit profile</h1></div>
<Errors {error} />
<form class="form" {onsubmit}>
	<fieldset>
		<legend>Profile</legend>
		<div class="field"><label for="name">Name</label><input id="name" bind:value={name} /></div>
		<div class="field"><label for="summary">Summary</label><textarea id="summary" rows="3" bind:value={summary}></textarea></div>
		<div class="field"><label for="notes">Drafting notes</label><textarea id="notes" rows="4" bind:value={notes}></textarea><small>One per line.</small></div>
	</fieldset>
	{#each [{ name: 'Wants', rows: want }, { name: 'Avoids', rows: avoid }] as group (group.name)}
		{#if group.rows.length}
			<fieldset>
				<legend>{group.name}</legend>
				<div>
					{#each group.rows as r (r.id)}
						<div class="crit" class:removed={r.remove} class:invalid={fields[`criteria.${r.id}.label`] || fields[`criteria.${r.id}.importance`]}>
							<input bind:value={r.label} aria-label="Label for {r.id}" disabled={r.remove} />
							<select bind:value={r.importance} aria-label="Importance for {r.id}" disabled={r.remove}>
								{#each data.meta.importances ?? [] as imp (imp)}<option value={imp}>{imp}</option>{/each}
							</select>
							<label class="check"><input type="checkbox" bind:checked={r.remove} aria-label="Remove {r.id}" /> Remove</label>
						</div>
					{/each}
				</div>
			</fieldset>
		{/if}
	{/each}
	<div class="form-actions"><button class="primary" type="submit" disabled={busy}>Save changes</button></div>
</form>

<style>
	.crit {
		display: grid;
		grid-template-columns: 1fr 9rem auto;
		gap: 0.5rem;
		align-items: center;
		padding: 0.5rem 0;
		border-bottom: 1px solid var(--line);
	}

	.crit:last-child {
		border-bottom: 0;
	}

	.crit input:not([type='checkbox']) {
		width: 100%;
		min-width: 0;
	}

	.removed :is(input:not([type='checkbox']), select) {
		text-decoration: line-through;
		opacity: 0.5;
	}

	.invalid :is(input, select) {
		border-color: var(--bad);
	}

	@media (width < 640px) {
		.crit {
			grid-template-columns: 1fr auto;
		}

		.crit input:not([type='checkbox']) {
			grid-column: 1 / -1;
		}
	}
</style>
