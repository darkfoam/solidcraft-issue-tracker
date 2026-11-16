import { json } from '@sveltejs/kit';
import { WorkError } from './work';

export function workErrorResponse(error: unknown): Response {
	if (error instanceof WorkError) {
		return json({ error: error.message }, { status: error.status });
	}
	const message = error instanceof Error ? error.message : 'work queue request failed';
	return json({ error: message }, { status: 500 });
}
