import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
	appId: 'io.kwkaiser.housing',
	appName: 'housing',
	webDir: 'www',
	server: {
		url: process.env.HOUSING_APP_URL || 'https://housing.kwkaiser.io',
		allowNavigation: ['*.kwkaiser.io']
	}
};

export default config;
