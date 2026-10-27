<script lang="ts">
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import { api, call } from '#lib/api/client.ts';

	let {
		source,
		id,
		on = $bindable(false),
		onchange
	}: { source: string; id: string; on?: boolean; onchange?: (on: boolean) => void } = $props();
	let busy = $state(false);

	async function toggle() {
		busy = true;
		const next = !on;
		try {
			const path = '/api/v1/listings/{source}/{source_id}/hidden' as const;
			const params = { path: { source, source_id: id } };
			await call(next ? api().PUT(path, { params }) : api().DELETE(path, { params }));
			on = next;
			onchange?.(next);
		} finally {
			busy = false;
		}
	}
</script>

<button type="button" aria-pressed={on} title={on ? 'Show this listing again' : 'Hide this listing from listings and the map'} disabled={busy} onclick={toggle}>
	{#if on}<EyeIcon size={16} /> Unhide{:else}<EyeOffIcon size={16} /> Hide{/if}
</button>
