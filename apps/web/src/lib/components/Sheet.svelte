<script lang="ts">
	import type { Snippet } from 'svelte';
	import XIcon from '@lucide/svelte/icons/x';

	let { open = $bindable(false), title, done = false, children }: { open?: boolean; title: string; done?: boolean; children: Snippet } = $props();

	const id = $props.id();
	let dialog: HTMLDialogElement;

	$effect(() => {
		if (open && !dialog.open) dialog.showModal();
		else if (!open && dialog.open) dialog.close();
	});

	function onclick(e: MouseEvent) {
		if (e.target === dialog) open = false;
	}
</script>

<dialog bind:this={dialog} onclose={() => (open = false)} {onclick} aria-labelledby="{id}-title">
	<div class="body">
		<header>
			<h2 id="{id}-title">{title}</h2>
			<button type="button" class="close" aria-label="Close" onclick={() => (open = false)}><XIcon size={20} /></button>
		</header>
		{@render children()}
		{#if done}<button type="button" class="primary block" onclick={() => (open = false)}>Done</button>{/if}
	</div>
</dialog>

<style>
	dialog {
		width: min(30rem, calc(100vw - 2rem));
		max-height: min(85dvh, 44rem);
		margin: auto;
		padding: 0;
		border: 1px solid var(--line);
		border-radius: var(--radius-lg);
		background: var(--surface);
		color: var(--fg);
		box-shadow: var(--shadow-lg);
		overscroll-behavior: contain;
	}

	dialog::backdrop {
		background: rgb(0 0 0 / 0.4);
		backdrop-filter: blur(2px);
	}

	dialog[open] {
		animation: fade-in 0.2s ease-out;
	}

	.body {
		padding: 0 1.1rem 1.25rem;
	}

	header {
		position: sticky;
		top: 0;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin: 0 -1.1rem 0.25rem;
		padding: 0.75rem 0.6rem 0.5rem 1.1rem;
		background: var(--surface);
	}

	h2 {
		margin: 0;
		font-size: 1.05rem;
	}

	.close {
		width: 2.25rem;
		min-height: 2.25rem;
		padding: 0;
		border: 0;
		border-radius: 999px;
		background: var(--surface-2);
	}

	@media (width < 900px) {
		dialog {
			width: 100%;
			max-width: 100%;
			margin: auto 0 0;
			padding-bottom: var(--safe-bottom);
			border-width: 1px 0 0;
			border-radius: 20px 20px 0 0;
		}

		dialog[open] {
			animation: slide-up 0.25s cubic-bezier(0.2, 0.8, 0.2, 1);
		}

		header {
			padding-top: 1.1rem;
		}

		header::before {
			content: '';
			position: absolute;
			top: 0.4rem;
			left: 50%;
			width: 2.25rem;
			height: 4px;
			margin-left: -1.125rem;
			border-radius: 4px;
			background: var(--line);
		}
	}

	@keyframes fade-in {
		from {
			opacity: 0;
			transform: translateY(8px) scale(0.98);
		}
	}

	@keyframes slide-up {
		from {
			transform: translateY(100%);
		}
	}
</style>
