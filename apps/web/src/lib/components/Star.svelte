<script lang="ts">
	import StarIcon from '@lucide/svelte/icons/star';
	import { api, call } from '#lib/api/client.ts';

	let { source, id, on = $bindable(false), compact = false }: { source: string; id: string; on?: boolean; compact?: boolean } = $props();
	let busy = $state(false);

	async function toggle() {
		busy = true;
		const next = !on;
		try {
			const path = '/api/v1/listings/{source}/{source_id}/favorite' as const;
			const params = { path: { source, source_id: id } };
			await call(next ? api().PUT(path, { params }) : api().DELETE(path, { params }));
			on = next;
		} finally {
			busy = false;
		}
	}
</script>

<button
	type="button"
	class:compact
	aria-pressed={on}
	aria-label={on ? 'Remove from favorites' : 'Add to favorites'}
	title={on ? 'Remove from favorites' : 'Add to favorites'}
	disabled={busy}
	onclick={toggle}><StarIcon size={20} fill={on ? 'currentColor' : 'none'} /></button
>

<style>
	button {
		width: var(--control);
		min-height: var(--control);
		padding: 0;
		border: 0;
		background: none;
		color: var(--muted);
	}

	button[aria-pressed='true'],
	button:hover {
		color: var(--star);
	}

	.compact {
		width: 2rem;
		min-height: 2rem;
	}
</style>
