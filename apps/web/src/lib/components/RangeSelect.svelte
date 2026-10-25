<script lang="ts">
	import { quickRanges } from '#lib/ranges.ts';

	type Extra = { value: string; label: string };

	let {
		from,
		to,
		extras = [],
		active,
		onrange,
		onextra
	}: {
		from: string;
		to: string;
		extras?: Extra[];
		active?: string;
		onrange: (from: string, to: string) => void;
		onextra?: (value: string) => void;
	} = $props();

	const quick = quickRanges();
	const CUSTOM = 'custom';
	let custom = $state(false);

	let preset = $derived(active ?? (to ? '' : (quick.find((q) => q.from === from)?.label ?? '')));
	let picking = $derived(custom || preset === '');

	function pick(value: string) {
		if (value === CUSTOM) {
			custom = true;
			return;
		}
		custom = false;
		if (extras.some((x) => x.value === value)) {
			onextra?.(value);
			return;
		}
		const q = quick.find((x) => x.label === value);
		if (q) onrange(q.from, '');
	}
</script>

<span class="range">
	<select aria-label="Date range" value={picking ? CUSTOM : preset} onchange={(e) => pick(e.currentTarget.value)}>
		{#each extras as x (x.value)}<option value={x.value}>{x.label}</option>{/each}
		{#each quick as q (q.label)}<option value={q.label}>{q.label}</option>{/each}
		<option value={CUSTOM}>Custom range…</option>
	</select>
	{#if picking}
		<input type="date" aria-label="From" value={from} max={to || undefined} onchange={(e) => onrange(e.currentTarget.value, to)} />
		<span class="muted">to</span>
		<input type="date" aria-label="To" value={to} min={from || undefined} onchange={(e) => onrange(from, e.currentTarget.value)} />
	{/if}
</span>

<style>
	.range {
		display: inline-flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.4rem;
	}

	input {
		min-width: 0;
	}
</style>
