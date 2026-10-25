<script lang="ts">
	import MapPinPlusIcon from '@lucide/svelte/icons/map-pin-plus';
	import { api, call, type PlaceMatch } from '#lib/api/client.ts';

	let { near, onpick }: { near: () => { lat: number; lng: number } | undefined; onpick: (match: PlaceMatch) => void } = $props();

	let details: HTMLDetailsElement;
	let input = $state<HTMLInputElement>();
	let q = $state('');
	let results = $state<PlaceMatch[]>([]);
	let status = $state<'' | 'loading' | 'empty' | 'error'>('');
	let timer: ReturnType<typeof setTimeout> | undefined;
	let ctrl: AbortController | undefined;

	function search() {
		clearTimeout(timer);
		ctrl?.abort();
		const text = q.trim();
		if (text.length < 3) {
			results = [];
			status = '';
			return;
		}
		status = 'loading';
		timer = setTimeout(async () => {
			const c = (ctrl = new AbortController());
			const at = near();
			try {
				const found = await call(api().GET('/api/v1/places/search', { params: { query: { q: text, lat: at?.lat, lng: at?.lng } }, signal: c.signal }));
				if (c !== ctrl) return;
				results = found;
				status = found.length ? '' : 'empty';
			} catch {
				if (c === ctrl && !c.signal.aborted) status = 'error';
			}
		}, 300);
	}

	function pick(match: PlaceMatch) {
		details.open = false;
		q = '';
		results = [];
		status = '';
		onpick(match);
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && results[0]) pick(results[0]);
		if (e.key === 'Escape') details.open = false;
	}
</script>

<details class="picker" bind:this={details} ontoggle={() => details.open && input?.focus()}>
	<summary><MapPinPlusIcon size={16} /> <span class="text">Add place</span></summary>
	<div class="menu">
		<input bind:this={input} type="search" placeholder="Search for an address" aria-label="Address" autocomplete="off" enterkeyhint="search" bind:value={q} oninput={search} {onkeydown} />
		{#if !results.length && status}
			<p class="muted">
				{#if status === 'loading'}Searching…{:else if status === 'empty'}No matches.{:else}Address search is unavailable right now.{/if}
			</p>
		{/if}
		{#each results as match (`${match.lat},${match.lng},${match.address}`)}
			<button type="button" class="ghost result" onclick={() => pick(match)}>
				<strong>{match.name || match.address}</strong>
				{#if match.name}<span class="muted">{match.address}</span>{/if}
			</button>
		{/each}
	</div>
</details>

<style>
	summary {
		box-shadow: var(--shadow-lg);
	}

	.menu {
		width: min(24rem, calc(100vw - 2rem));
	}

	input {
		width: 100%;
	}

	p {
		margin: 0.5rem 0.6rem 0.25rem;
		font-size: 0.875rem;
	}

	.result {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.1rem;
		width: 100%;
		margin-top: 0.25rem;
		padding: 0.45rem 0.6rem;
		text-align: left;
		white-space: normal;
		font-weight: 400;
	}

	.result .muted {
		font-size: 0.8rem;
	}

	@media (width < 640px) {
		.text {
			display: none;
		}
	}
</style>
