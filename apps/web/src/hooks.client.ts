import type { ClientInit } from '@sveltejs/kit/hooks';
import { listenForSignIn } from '#lib/native.ts';

export const init: ClientInit = () => listenForSignIn();
