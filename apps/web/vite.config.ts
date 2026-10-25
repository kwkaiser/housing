import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
	const env = loadEnv(mode, '.', 'HOUSING_');
	const server = { target: env.HOUSING_API_URL || 'http://127.0.0.1:8080' };
	return {
		plugins: [
			sveltekit({
				compilerOptions: {
					runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
				},
				adapter: adapter({ fallback: '200.html', precompress: true }),
				version: env.HOUSING_VERSION ? { name: env.HOUSING_VERSION } : undefined
			})
		],
		build: { chunkSizeWarningLimit: 1200 },
		server: {
			proxy: { '/api': server, '/auth': server }
		}
	};
});
