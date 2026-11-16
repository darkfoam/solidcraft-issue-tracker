import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../../lib/server/auth';
import { workErrorResponse } from '../../../../lib/server/api';
import { getQueueItem, updateCluster } from '../../../../lib/server/work';
import type { RequestHandler } from './$types';

function clusterId(params: { id: string }): number {
	const id = Number(params.id);
	if (!Number.isInteger(id) || id <= 0) throw new Error('invalid cluster id');
	return id;
}

export const GET: RequestHandler = ({ params }) => {
	const item = getQueueItem(clusterId(params));
	if (!item) return json({ error: 'cluster not found' }, { status: 404 });
	return json(item);
};

export const PATCH: RequestHandler = async ({ params, request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json()) as {
			status?: string;
			priority?: string;
			layer?: string;
			assignee?: string;
			note?: string;
			version?: number;
			actor?: string;
		};
		updateCluster(
			clusterId(params),
			{
				status: body.status,
				priority: body.priority,
				layer: body.layer,
				assignee: body.assignee,
				note: body.note,
				version: body.version
			},
			actorFrom(request, body)
		);
		return json(getQueueItem(clusterId(params)) ?? { ok: true });
	} catch (error) {
		return workErrorResponse(error);
	}
};
