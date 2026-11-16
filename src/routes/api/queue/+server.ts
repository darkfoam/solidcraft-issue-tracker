import { json } from '@sveltejs/kit';
import { getQueue } from '../../../lib/server/work';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ url }) => {
	const limit = Number(url.searchParams.get('limit') ?? 100);
	return json({
		items: getQueue({
			area: url.searchParams.get('area') ?? undefined,
			topic: url.searchParams.get('topic') ?? undefined,
			status: url.searchParams.get('status') ?? undefined,
			assignee: url.searchParams.get('assignee') ?? undefined,
			limit: Number.isFinite(limit) && limit > 0 ? Math.min(limit, 500) : 100
		})
	});
};
