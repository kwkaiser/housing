import { shiftDay } from './format.ts';

export type QuickRange = { label: string; from: string };

export function quickRanges(today = new Date()): QuickRange[] {
	return [
		{ label: 'Last day', from: shiftDay(today, { days: -1 }) },
		{ label: 'Last week', from: shiftDay(today, { days: -7 }) },
		{ label: 'Last 2 weeks', from: shiftDay(today, { days: -14 }) },
		{ label: 'Last month', from: shiftDay(today, { months: -1 }) },
		{ label: 'All dates', from: '' }
	];
}

export const ALL_DATES = '1970-01-01';
