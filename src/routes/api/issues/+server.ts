import { json } from '@sveltejs/kit';
import { getIssueDetail } from '../../../lib/server/read';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = ({ url }) => {
	const repo = url.searchParams.get('repo') ?? '';
	const number = Number(url.searchParams.get('number'));
	if (!repo || !Number.isInteger(number) || number <= 0) {
		return json({ error: 'repo and number are required' }, { status: 400 });
	}
	const issue = getIssueDetail(repo, number);
	if (!issue) return json({ error: 'issue not found' }, { status: 404 });
	return json(issue, {
		headers: { 'cache-control': 'public, max-age=30, stale-while-revalidate=30' }
	});
};
