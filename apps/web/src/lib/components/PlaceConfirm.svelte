<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import type { PlaceMatch } from '#lib/api/client.ts';

	let { match, onsave, oncancel }: { match: PlaceMatch; onsave: (name: string) => Promise<void>; oncancel: () => void } = $props();

	let name = $state(untrack(() => match.name ?? ''));
	let saving = $state(false);
	let error = $state('');
	let input: HTMLInputElement;

	onMount(() => {
		if (!matchMedia('(pointer: coarse)').matches) input.focus({ preventScroll: true });
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		const trimmed = name.trim();
		if (!trimmed) return;
		saving = true;
		error = '';
		try {
			await onsave(trimmed);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Could not save this place.';
			saving = false;
		}
	}
</script>

<form onsubmit={submit}>
	<strong>Add this place?</strong>
	<div class="muted">{match.address}</div>
	<input bind:this={input} bind:value={name} placeholder="Name, like Sam's place" aria-label="Name" maxlength="100" />
	{#if error}<div class="error">{error}</div>{/if}
	<div class="actions">
		<button type="button" class="ghost" onclick={oncancel}>Cancel</button>
		<button type="submit" class="primary" disabled={saving || !name.trim()}>{saving ? 'Adding…' : 'Add place'}</button>
	</div>
</form>

<style>
	form {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding-right: 1rem;
	}

	input {
		width: 100%;
	}

	.error {
		color: var(--bad);
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
	}
</style>
