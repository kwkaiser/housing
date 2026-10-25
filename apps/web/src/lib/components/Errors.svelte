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

<style>
	.errors {
		max-width: 48rem;
		margin: 0 0 1rem;
		padding: 0.75rem 1rem;
		border: 1px solid color-mix(in srgb, var(--bad) 30%, transparent);
		border-radius: var(--radius);
		background: color-mix(in srgb, var(--bad) 10%, var(--surface));
		color: var(--bad);
	}

	p {
		margin: 0;
		color: inherit;
		font-weight: 600;
	}

	ul {
		margin: 0.35rem 0 0;
		padding-left: 1.2rem;
	}
</style>
