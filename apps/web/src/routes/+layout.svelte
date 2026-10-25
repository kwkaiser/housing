<script lang="ts">
	import '../app.css';
	import ActivityIcon from '@lucide/svelte/icons/activity';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import HouseIcon from '@lucide/svelte/icons/house';
	import ListIcon from '@lucide/svelte/icons/list';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import MapIcon from '@lucide/svelte/icons/map';
	import StarIcon from '@lucide/svelte/icons/star';
	import UserIcon from '@lucide/svelte/icons/user';
	import { afterNavigate, onNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import Sheet from '#lib/components/Sheet.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	type Item = { label: string; href: string; icon: typeof ListIcon; prefixes?: string[]; secondary?: boolean; fillActive?: boolean };

	const nav: Item[] = [
		{ label: 'Listings', href: '/', icon: ListIcon, prefixes: ['/c/', '/listing/'] },
		{ label: 'Map', href: '/map', icon: MapIcon },
		{ label: 'Favorites', href: '/favorites', icon: StarIcon, fillActive: true },
		{ label: 'Collections', href: '/collections', icon: FolderIcon },
		{ label: 'Profiles', href: '/profiles', icon: UserIcon, secondary: true },
		{ label: 'Jobs', href: '/jobs', icon: ActivityIcon, secondary: true }
	];

	let more = $state(false);

	function active(item: Item, path: string): boolean {
		if (item.href === '/') return path === '/' || (item.prefixes ?? []).some((p) => path.startsWith(p));
		return path === item.href || path.startsWith(item.href + '/');
	}

	let moreActive = $derived(nav.some((item) => item.secondary && active(item, page.url.pathname)));

	afterNavigate(() => (more = false));

	onNavigate((navigation) => {
		if (!document.startViewTransition || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		if (navigation.from?.url.pathname === navigation.to?.url.pathname) return;
		return new Promise((resolve) => {
			document.startViewTransition(async () => {
				resolve();
				await navigation.complete;
			});
		});
	});
</script>

{#snippet account()}
	{#if data.me.auth === 'session'}
		<form class="account" method="post" action="/auth/logout" data-sveltekit-reload>
			<span title={data.me.email}>{data.me.name}</span>
			<button type="submit" class="ghost"><LogOutIcon size={18} /> Sign out</button>
		</form>
	{/if}
{/snippet}

<header class="topbar">
	<a class="brand" href="/"><HouseIcon size={20} /> housing</a>
	<nav aria-label="Main">
		{#each nav as item (item.href)}
			<a href={item.href} aria-current={active(item, page.url.pathname) ? 'page' : undefined}>{item.label}</a>
		{/each}
	</nav>
	{@render account()}
</header>

<main>
	{@render children()}
</main>

<nav class="tabbar" aria-label="Main">
	{#each nav.filter((item) => !item.secondary) as item (item.href)}
		{@const current = active(item, page.url.pathname)}
		<a href={item.href} aria-current={current ? 'page' : undefined}>
			<item.icon size={22} fill={item.fillActive && current ? 'currentColor' : 'none'} />
			<span>{item.label}</span>
		</a>
	{/each}
	<button type="button" class:active={moreActive} aria-expanded={more} aria-haspopup="dialog" onclick={() => (more = true)}>
		<EllipsisIcon size={22} />
		<span>More</span>
	</button>
</nav>

<Sheet bind:open={more} title="More">
	<ul class="menu-list">
		{#each nav.filter((item) => item.secondary) as item (item.href)}
			<li>
				<a href={item.href} aria-current={active(item, page.url.pathname) ? 'page' : undefined}>
					<item.icon size={20} />
					{item.label}
					<ChevronRightIcon size={18} class="chevron" />
				</a>
			</li>
		{/each}
	</ul>
	{@render account()}
</Sheet>

<style>
	.topbar {
		position: sticky;
		top: 0;
		z-index: 10;
		display: flex;
		align-items: center;
		gap: 1.5rem;
		padding: calc(0.5rem + var(--safe-top)) calc(1.25rem + var(--safe-right)) 0.5rem calc(1.25rem + var(--safe-left));
		background: color-mix(in srgb, var(--bg) 85%, transparent);
		backdrop-filter: saturate(1.8) blur(12px);
		border-bottom: 1px solid var(--line);
		view-transition-name: topbar;
	}

	.brand {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
		color: var(--fg);
		font-weight: 700;
		letter-spacing: -0.01em;
	}

	.brand :global(.lucide-icon) {
		color: var(--accent);
	}

	.topbar nav {
		flex: 1;
		display: flex;
		gap: 0.25rem;
	}

	.topbar nav a {
		padding: 0.4rem 0.75rem;
		border-radius: 999px;
		color: var(--muted);
		font-weight: 500;
	}

	.topbar nav a:hover {
		color: var(--fg);
		background: var(--surface-2);
		text-decoration: none;
	}

	.topbar nav a[aria-current='page'] {
		color: var(--fg);
		background: var(--surface-2);
	}

	.account {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0;
		color: var(--muted);
		white-space: nowrap;
	}

	.menu-list + .account {
		justify-content: space-between;
		margin-top: 1rem;
		padding-top: 1rem;
		border-top: 1px solid var(--line);
	}

	.tabbar {
		display: none;
	}

	@media (width < 900px) {
		.topbar {
			display: none;
		}

		.tabbar {
			position: fixed;
			z-index: 10;
			inset: auto 0 0;
			display: grid;
			grid-template-columns: repeat(5, 1fr);
			padding: 0 var(--safe-right) var(--safe-bottom) var(--safe-left);
			background: color-mix(in srgb, var(--surface) 88%, transparent);
			backdrop-filter: saturate(1.8) blur(16px);
			border-top: 1px solid var(--line);
			view-transition-name: tabbar;
		}
	}

	.tabbar :is(a, button) {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 2px;
		height: var(--tabbar);
		min-height: 0;
		padding: 0;
		border: 0;
		border-radius: 0;
		background: none;
		color: var(--muted);
		font-size: 0.68rem;
		font-weight: 500;
		letter-spacing: 0.01em;
	}

	.tabbar a:hover {
		text-decoration: none;
	}

	.tabbar :is([aria-current='page'], .active) {
		color: var(--accent);
	}

	.menu-list {
		margin: 0;
		padding: 0;
		list-style: none;
	}

	.menu-list a {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-height: 3rem;
		color: var(--fg);
		font-weight: 500;
		border-bottom: 1px solid var(--line);
	}

	.menu-list a:hover {
		text-decoration: none;
	}

	.menu-list a[aria-current='page'] {
		color: var(--accent);
	}

	.menu-list :global(.chevron) {
		margin-left: auto;
		color: var(--muted);
	}
</style>
