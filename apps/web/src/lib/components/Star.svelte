<script lang="ts">
	import { api, call } from '#lib/api/client.ts';

	let { source, id, on = $bindable(false) }: { source: string; id: string; on?: boolean } = $props();
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
	class="star"
	aria-pressed={on}
	aria-label={on ? 'Remove from favorites' : 'Add to favorites'}
	title={on ? 'Remove from favorites' : 'Add to favorites'}
	disabled={busy}
	onclick={toggle}>{on ? '★' : '☆'}</button
>
