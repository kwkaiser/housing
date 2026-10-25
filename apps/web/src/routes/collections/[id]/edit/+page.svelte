<script lang="ts">
	import { goto } from '$app/navigation';
	import Back from '#lib/components/Back.svelte';
	import { api, call } from '#lib/api/client.ts';
	import CollectionForm from '#lib/components/CollectionForm.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let id = $derived(data.collection.id);
</script>

<svelte:head><title>Edit {id} · housing</title></svelte:head>

<Back href="/c/{encodeURIComponent(id)}" label={id} />
<div class="page-head"><h1>Edit collection</h1></div>
{#key id}
	<CollectionForm
		meta={data.meta}
		profiles={data.profiles}
		collection={data.collection}
		submit={async (body, clear) => {
			const { id: _, ...fields } = body;
			await call(api().PATCH('/api/v1/collections/{id}', { params: { path: { id } }, body: { ...fields, clear } }));
			goto('/collections');
		}}
	/>
{/key}
