import { json } from '@sveltejs/kit';
import { getLastSyncedAt } from '../../../lib/server/read';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = () =>
	json(
		{ lastSyncedAt: getLastSyncedAt() },
		{ headers: { 'cache-control': 'public, max-age=30, stale-while-revalidate=30' } }
	);
