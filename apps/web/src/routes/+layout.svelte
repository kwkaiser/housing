<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	const nav = [
		{ label: 'Listings', href: '/', prefixes: ['/c/', '/listing/'] },
		{ label: 'Favorites', href: '/favorites' },
		{ label: 'Map', href: '/map' },
		{ label: 'Collections', href: '/collections' },
		{ label: 'Profiles', href: '/profiles' },
		{ label: 'Jobs', href: '/jobs' },
		{ label: 'API keys', href: '/keys' }
	];

	function active(item: (typeof nav)[number], path: string): boolean {
		if (item.href === '/') return path === '/' || (item.prefixes ?? []).some((p) => path.startsWith(p));
		return path === item.href || path.startsWith(item.href + '/');
	}
</script>

<header class="top">
	<nav>
		<a class="brand" href="/">housing</a>
		{#each nav as item (item.href)}
			<a href={item.href} aria-current={active(item, page.url.pathname) ? 'page' : undefined}>{item.label}</a>
		{/each}
	</nav>
</header>
<main>
	{@render children()}
</main>
