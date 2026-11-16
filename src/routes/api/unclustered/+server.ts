import { json } from '@sveltejs/kit';
import { getUnclustered } from '../../../lib/server/work';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ url }) => {
	const limit = Number(url.searchParams.get('limit') ?? 200);
	return json({
		items: getUnclustered({
			area: url.searchParams.get('area') ?? undefined,
			topic: url.searchParams.get('topic') ?? undefined,
			repo: url.searchParams.get('repo') ?? undefined,
			limit: Number.isFinite(limit) && limit > 0 ? Math.min(limit, 500) : 200
		})
	});
};
