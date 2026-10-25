import { Capacitor } from '@capacitor/core';
import { App } from '@capacitor/app';
import { Browser } from '@capacitor/browser';

const verifierKey = 'housing.signin.verifier';
const appScheme = 'io.kwkaiser.housing:';

let signingIn = false;

export function signIn(rd: string) {
	if (signingIn) return;
	signingIn = true;
	if (Capacitor.isNativePlatform()) {
		signInWithBrowser(rd).catch((e) => {
			signingIn = false;
			console.error('sign-in', e);
		});
		return;
	}
	location.assign(`/auth/login?rd=${encodeURIComponent(rd)}`);
}

export function listenForSignIn() {
	if (!Capacitor.isNativePlatform()) return;
	App.addListener('appUrlOpen', ({ url }) => finishSignIn(url));
	Browser.addListener('browserFinished', () => (signingIn = false));
	App.getLaunchUrl().then((launch) => launch && finishSignIn(launch.url));
}

async function signInWithBrowser(rd: string) {
	const verifier = base64url(crypto.getRandomValues(new Uint8Array(32)));
	const challenge = base64url(new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))));
	localStorage.setItem(verifierKey, verifier);
	await Browser.open({ url: `${location.origin}/auth/login?${new URLSearchParams({ rd, app: challenge })}` });
}

async function finishSignIn(link: string) {
	const url = new URL(link);
	if (url.protocol !== appScheme || url.host !== 'auth') return;
	const verifier = localStorage.getItem(verifierKey);
	if (!verifier) return;
	localStorage.removeItem(verifierKey);
	const res = await fetch('/auth/handoff', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ code: url.searchParams.get('code'), verifier })
	});
	signingIn = false;
	if (!res.ok) {
		alert(`Sign-in failed: ${(await res.text()).trim()}`);
		return;
	}
	const rd = url.searchParams.get('rd') ?? '/';
	location.assign(rd.startsWith('/') && !rd.startsWith('//') && !rd.startsWith('/\\') ? rd : '/');
}

function base64url(bytes: Uint8Array): string {
	return btoa(String.fromCharCode(...bytes))
		.replace(/\+/g, '-')
		.replace(/\//g, '_')
		.replace(/=+$/, '');
}
