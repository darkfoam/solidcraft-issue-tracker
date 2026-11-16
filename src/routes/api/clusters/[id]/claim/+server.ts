import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../../../lib/server/auth';
import { workErrorResponse } from '../../../../../lib/server/api';
import { claimCluster } from '../../../../../lib/server/work';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ params, request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json().catch(() => ({}))) as {
			as?: string;
			ttlMinutes?: number;
			actor?: string;
		};
		const actor = body.as?.trim() || actorFrom(request, body);
		const ttl = Number(body.ttlMinutes ?? 60);
		const item = claimCluster(Number(params.id), actor, Number.isFinite(ttl) ? ttl : 60);
		return json(item);
	} catch (error) {
		return workErrorResponse(error);
	}
};
