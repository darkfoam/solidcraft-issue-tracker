import { json } from '@sveltejs/kit';
import { actorFrom, requireAgent } from '../../../lib/server/auth';
import { workErrorResponse } from '../../../lib/server/api';
import { createCluster } from '../../../lib/server/work';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	try {
		const body = (await request.json()) as {
			area?: string;
			topic?: string;
			label?: string;
			signature?: string;
			issueIds?: unknown;
			actor?: string;
		};
		const issueIds = Array.isArray(body.issueIds)
			? body.issueIds
					.map((value) => Number(value))
					.filter((value) => Number.isInteger(value) && value > 0)
			: [];
		const clusterId = createCluster(
			{
				area: String(body.area ?? ''),
				topic: String(body.topic ?? ''),
				label: String(body.label ?? ''),
				signature: body.signature ? String(body.signature) : undefined,
				issueIds
			},
			actorFrom(request, body)
		);
		return json({ clusterId }, { status: 201 });
	} catch (error) {
		return workErrorResponse(error);
	}
};
