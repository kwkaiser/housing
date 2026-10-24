<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { api, call, type Schemas } from '#lib/api/client.ts';
	import Errors from '#lib/components/Errors.svelte';
	import { ago, dateTime } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let name = $state('');
	let created = $state<Schemas['APICreatedKey']>();
	let error = $state<unknown>();

	async function create(e: SubmitEvent) {
		e.preventDefault();
		error = undefined;
		try {
			created = await call(api().POST('/api/v1/keys', { body: { name } }));
			name = '';
			await invalidateAll();
		} catch (err) {
			error = err;
		}
	}

	async function remove(id: number) {
		if (!confirm('Delete this key? Anything using it will stop working.')) return;
		error = undefined;
		try {
			await call(api().DELETE('/api/v1/keys/{id}', { params: { path: { id } } }));
			if (created?.key.id === id) created = undefined;
			await invalidateAll();
		} catch (err) {
			error = err;
		}
	}
</script>

<svelte:head><title>API keys · housing</title></svelte:head>

<h1>API keys</h1>
<p>Keys let scripts, phones and MCP clients call the API at <code>/api/v1</code>.</p>
<Errors {error} />
{#if created}
	<div class="notice">
		<p>Created <strong>{created.key.name}</strong>. Copy the token now; it won't be shown again.</p>
		<input class="secret" readonly value={created.token} onfocus={(e) => e.currentTarget.select()} />
	</div>
{/if}
<form class="controls" onsubmit={create}>
	<input bind:value={name} placeholder="Key name, e.g. phone" aria-label="Key name" required />
	<button type="submit">Create key</button>
</form>
{#if data.keys.length}
	<div class="wrap">
		<table class="keys">
			<thead><tr><th>Name</th><th>Token</th><th>Created</th><th>Last used</th><th></th></tr></thead>
			<tbody>
				{#each data.keys as k (k.id)}
					<tr>
						<td>{k.name}</td>
						<td><code>{k.hint}…</code></td>
						<td>{dateTime(k.created_at)}</td>
						<td>{k.last_used_at ? ago(k.last_used_at) : 'never'}</td>
						<td><button type="button" onclick={() => remove(k.id)}>Delete</button></td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<p>No keys yet.</p>
{/if}
