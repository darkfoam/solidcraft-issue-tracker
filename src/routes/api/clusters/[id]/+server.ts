import { json } from '@sveltejs/kit';
import { getClusterDetail } from '../../../../lib/server/read';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ params }) => {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0)
		return json({ error: 'invalid cluster id' }, { status: 400 });
	const cluster = getClusterDetail(id);
	if (!cluster) return json({ error: 'cluster not found' }, { status: 404 });
	return json(cluster, {
		headers: { 'cache-control': 'public, max-age=30, stale-while-revalidate=30' }
	});
};
