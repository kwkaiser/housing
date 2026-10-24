<script lang="ts">
	import { ApiError } from '#lib/api/client.ts';

	let { error }: { error: unknown } = $props();
	let messages = $derived.by(() => {
		if (!error) return [];
		if (error instanceof ApiError) {
			const fields = Object.entries(error.fields).map(([k, v]) => `${k}: ${v}`);
			return fields.length || error.general.length ? [...error.general, ...fields] : [error.message];
		}
		return [error instanceof Error ? error.message : String(error)];
	});
</script>

{#if messages.length}
	<div class="errors" role="alert">
		<p>That didn't work:</p>
		<ul>
			{#each messages as m, i (i)}<li>{m}</li>{/each}
		</ul>
	</div>
{/if}
