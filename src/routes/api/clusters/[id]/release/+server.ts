import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../../../lib/server/auth';
import { workErrorResponse } from '../../../../../lib/server/api';
import { releaseCluster } from '../../../../../lib/server/work';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ params, request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json().catch(() => ({}))) as { actor?: string };
		releaseCluster(Number(params.id), actorFrom(request, body));
		return json({ ok: true });
	} catch (error) {
		return workErrorResponse(error);
	}
};
