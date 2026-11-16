import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../../../lib/server/auth';
import { workErrorResponse } from '../../../../../lib/server/api';
import { mergeClusters } from '../../../../../lib/server/work';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ params, request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json()) as {
			into?: number;
			actor?: string;
		};
		const target = Number(body.into);
		if (!Number.isInteger(target) || target <= 0) {
			return json({ error: 'into must be a cluster id' }, { status: 400 });
		}
		mergeClusters(Number(params.id), target, actorFrom(request, body));
		return json({ ok: true });
	} catch (error) {
		return workErrorResponse(error);
	}
};
