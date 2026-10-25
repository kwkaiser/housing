<script lang="ts">
	import { match, tier } from '#lib/format.ts';

	let { value, calibrated, stale = false, size = 'md' }: { value: number; calibrated: boolean; stale?: boolean; size?: 'sm' | 'md' | 'lg' } = $props();
</script>

<span class="match {tier(value)} {size}">{match(value, calibrated, stale)}</span>

<style>
	.match {
		--tone: var(--muted);
		display: inline-grid;
		place-items: center;
		min-width: 3.25rem;
		padding: 0.25rem 0.5rem;
		border-radius: 10px;
		color: var(--tone);
		background: color-mix(in srgb, var(--tone) 12%, transparent);
		font-weight: 700;
		font-variant-numeric: tabular-nums;
	}

	.high {
		--tone: var(--good);
	}

	.mid {
		--tone: var(--warn);
	}

	.sm {
		min-width: 0;
		padding: 0.05rem 0.4rem;
		font-size: 0.78rem;
		background: var(--surface);
		box-shadow: var(--shadow);
	}

	.lg {
		min-width: 4.25rem;
		padding: 0.4rem 0.6rem;
		font-size: 1.25rem;
	}
</style>
