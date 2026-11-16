import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../../../lib/server/auth';
import { workErrorResponse } from '../../../../../lib/server/api';
import { assignIssues } from '../../../../../lib/server/work';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ params, request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json()) as { issueIds?: unknown; actor?: string };
		const issueIds = Array.isArray(body.issueIds)
			? body.issueIds
					.map((value) => Number(value))
					.filter((value) => Number.isInteger(value) && value > 0)
			: [];
		assignIssues(Number(params.id), issueIds, actorFrom(request, body));
		return json({ ok: true });
	} catch (error) {
		return workErrorResponse(error);
	}
};
