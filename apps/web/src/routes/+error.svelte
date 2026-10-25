<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head><title>{page.status} · housing</title></svelte:head>

<div class="page-head"><h1>{page.status === 404 ? 'Not found' : 'Something went wrong'}</h1></div>
<div class="card empty">
	<p>{page.error?.message}</p>
	<a class="button" href="/">Back to listings</a>
</div>
