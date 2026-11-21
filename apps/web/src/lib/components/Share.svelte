<script lang="ts">
	import CheckIcon from '@lucide/svelte/icons/check';
	import Share2Icon from '@lucide/svelte/icons/share-2';

	let { url, compact = false }: { url: string; compact?: boolean } = $props();
	let copied = $state(false);
	let size = $derived(compact ? 12 : 16);

	async function share() {
		await navigator.clipboard.writeText(url);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}
</script>

<button type="button" class:icon-button={!compact} class:compact aria-label="Copy listing link" title={copied ? 'Link copied' : 'Copy listing link'} onclick={share}>
	{#if copied}<CheckIcon {size} />{:else}<Share2Icon {size} />{/if}
</button>

<style>
	.compact {
		position: relative;
		z-index: 1;
		min-height: 0;
		padding: 0.1rem;
		margin: -0.1rem;
		border: 0;
		background: none;
		color: var(--muted);
	}

	.compact:hover {
		color: var(--fg);
	}
</style>
