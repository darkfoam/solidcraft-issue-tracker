import { json } from '@sveltejs/kit';
import { timingSafeEqual } from 'node:crypto';

function safeEqual(left: string, right: string): boolean {
	const a = Buffer.from(left);
	const b = Buffer.from(right);
	if (a.length !== b.length) return false;
	return timingSafeEqual(a, b);
}

export function requireAgent(request: Request): Response | null {
	const expected = process.env.AGENT_TOKEN?.trim();
	if (!expected) return null;
	const header = request.headers.get('authorization') ?? '';
	const match = /^Bearer\s+(.+)$/i.exec(header);
	if (!match || !safeEqual(match[1], expected)) {
		return json(
			{ error: 'agent token required' },
			{ status: 401, headers: { 'www-authenticate': 'Bearer' } }
		);
	}
	return null;
}

export function actorFrom(request: Request, body?: { actor?: unknown }): string {
	if (typeof body?.actor === 'string' && body.actor.trim()) return body.actor.trim();
	const header = request.headers.get('x-agent');
	if (header?.trim()) return header.trim();
	return 'local-agent';
}
