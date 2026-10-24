<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, ApiError, call } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import { lines } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let url = $state('');
	let id = $state('');
	let kind = $state<'want' | 'avoid'>('want');
	let name = $state('');
	let notes = $state('');
	let draft = $state(true);
	let force = $state(false);
	let model = $state('');
	let charge = $state('');
	let error = $state<unknown>();
	let busy = $state(false);
	let fields = $derived(error instanceof ApiError ? error.fields : {});

	async function onsubmit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = undefined;
		try {
			const job = await call(
				api().POST('/api/v1/profiles', {
					body: { url: url.trim(), id: id.trim(), kind, name, notes: lines(notes), no_draft: !draft, force, model: model || undefined, max_charge_usd: charge ? Number(charge) : undefined }
				})
			);
			goto(`/jobs/${job.id}`);
		} catch (err) {
			error = err;
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>New profile · housing</title></svelte:head>

<h1>New profile</h1>
<p>A profile is drafted from a reference listing you like (or, for an avoid profile, one you don't). Looking up the listing and drafting its criteria both cost money.</p>
<Errors {error} />
<form class="form" {onsubmit}>
	<fieldset>
		<legend>Reference</legend>
		<div class="field" class:invalid={fields.url}>
			<label for="url">Listing URL</label>
			<input id="url" type="url" bind:value={url} required />
			{#if fields.url}<p class="field-error">{fields.url}</p>{/if}
		</div>
		<div class="field" class:invalid={fields.id}>
			<label for="id">ID</label>
			<input id="id" bind:value={id} required pattern="[a-z0-9-]+" autocapitalize="off" />
			{#if fields.id}<p class="field-error">{fields.id}</p>{/if}
		</div>
		<div class="field">
			<span class="label">Kind</span>
			<div class="options">
				{#each data.meta.profile_kinds ?? [] as k (k)}<label class="toggle"><input type="radio" name="kind" value={k} bind:group={kind} /> {k}</label>{/each}
			</div>
		</div>
		<div class="field"><label for="name">Name</label><input id="name" bind:value={name} /></div>
		<div class="field">
			<label for="notes">Drafting notes</label>
			<textarea id="notes" rows="4" bind:value={notes}></textarea>
			<small>One per line, e.g. "must allow cats".</small>
		</div>
	</fieldset>
	<fieldset>
		<legend>Drafting</legend>
		<label class="toggle"><input type="checkbox" bind:checked={draft} /> Draft criteria now</label>
		<label class="toggle"><input type="checkbox" bind:checked={force} /> Replace an existing profile with this ID</label>
		<div class="grid">
			<div class="field"><label for="model">Model</label><input id="model" bind:value={model} placeholder={data.meta.defaults.draft_model} disabled={!draft} /></div>
			<div class="field" class:invalid={fields.max_charge_usd}>
				<label for="charge">Max lookup charge ($)</label>
				<input id="charge" inputmode="decimal" bind:value={charge} placeholder={data.meta.defaults.lookup_max_charge_usd.toFixed(2)} />
			</div>
		</div>
	</fieldset>
	<button class="primary" type="submit" disabled={busy}>Create profile</button>
</form>
