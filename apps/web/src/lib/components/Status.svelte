<script lang="ts">
	import type { Tone } from '#lib/format.ts';

	let { status }: { status: string } = $props();

	const tones: Record<string, Tone> = { running: 'accent', succeeded: 'good', degraded: 'warn', failed: 'bad' };
</script>

<span class="pill {tones[status] ?? 'muted'}" class:running={status === 'running'} class:cancelled={status === 'cancelled'}>{status}</span>

<style>
	.pill::before {
		content: '';
		width: 0.45rem;
		height: 0.45rem;
		border-radius: 50%;
		background: currentColor;
	}

	.running::before {
		animation: pulse 1.2s ease-in-out infinite;
	}

	.cancelled {
		text-decoration: line-through;
	}

	@keyframes pulse {
		50% {
			opacity: 0.3;
		}
	}
</style>
