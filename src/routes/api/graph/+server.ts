import { json } from '@sveltejs/kit';
import { filterGraph, getGraph } from '../../../lib/server/graph';
import type { IssueState } from '../../../lib/graph/types';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ url }) => {
	const stateParam = url.searchParams.get('state');
	const state: IssueState | undefined =
		stateParam === 'open' || stateParam === 'closed' ? stateParam : undefined;

	try {
		const graph = getGraph();
		return json(
			filterGraph(graph, {
				repo: url.searchParams.get('repo') ?? undefined,
				state,
				area: url.searchParams.get('area') ?? undefined,
				topic: url.searchParams.get('topic') ?? undefined
			}),
			{
				headers: {
					'cache-control': 'no-store'
				}
			}
		);
	} catch (error) {
		const message = error instanceof Error ? error.message : 'Unable to read the triage database';
		return json(
			{ error: message },
			{
				status: 503,
				headers: {
					'cache-control': 'no-store'
				}
			}
		);
	}
};
