<script lang="ts">
	import { goto } from '$app/navigation';
	import { api, call } from '#lib/api/client.ts';
	import CollectionForm from '#lib/components/CollectionForm.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>New collection · housing</title></svelte:head>

<h1>New collection</h1>
<CollectionForm
	meta={data.meta}
	profiles={data.profiles}
	submit={async (body) => {
		await call(api().POST('/api/v1/collections', { body }));
		goto('/collections');
	}}
/>
