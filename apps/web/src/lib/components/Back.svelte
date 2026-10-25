<script lang="ts">
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';

	let { href, label = 'Back' }: { href: string; label?: string } = $props();
</script>

<a {href}><ArrowLeftIcon size={18} /> {label}</a>

<style>
	a {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		margin: 0 0 0.75rem;
		font-weight: 500;
	}
</style>
