import { json } from '@sveltejs/kit';
import { requireAgent } from '../../../lib/server/auth';
import { getSyncStatus, runSync } from '../../../lib/server/sync';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = () => json(getSyncStatus());

export const POST: RequestHandler = async ({ request }) => {
	const denied = requireAgent(request);
	if (denied) return denied;
	const body = (await request.json().catch(() => ({}))) as { full?: boolean; noRetag?: boolean };
	const args = [];
	if (body.full) args.push('--full');
	if (body.noRetag) args.push('--no-retag');
	return json(runSync(args), { status: 202 });
};
