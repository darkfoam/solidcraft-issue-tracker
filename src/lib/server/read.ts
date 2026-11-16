import { DatabaseSync } from 'node:sqlite';
import { DB_PATH } from './graph';

export interface Assignee {
	login: string;
	avatarUrl: string;
	htmlUrl: string;
}

export interface PullRequestSummary {
	id: number;
	number: number;
	title: string;
	state: string;
	mergedAt: string;
	author: string;
	url: string;
	relation: string;
	draft: boolean;
	reviewDecision: string;
	mergeable: string;
	checksState: string;
	status: string;
}

export interface IssueSummary {
	id: number;
	repo: string;
	number: number;
	title: string;
	state: string;
	url: string;
	assignees: Assignee[];
	updatedAt: string;
}

export interface IssueDetail extends IssueSummary {
	createdAt: string;
	clusterId: number | null;
	labels: string[];
	pullRequests: PullRequestSummary[];
	lastSyncedAt: string;
}

export interface ClusterDetail {
	id: number;
	name: string;
	area: string;
	topic: string;
	signature: string;
	issueCount: number;
	openCount: number;
	issues: IssueSummary[];
	pullRequests: PullRequestSummary[];
	lastSyncedAt: string;
}

export interface UnclusteredIssue extends IssueSummary {
	area: string;
	topic: string;
}

function parseAssignees(rawJson: string): Assignee[] {
	try {
		const value = JSON.parse(rawJson) as {
			assignees?: Array<{ login?: string; avatar_url?: string; html_url?: string }>;
		};
		return (value.assignees ?? [])
			.filter((assignee) => assignee.login)
			.map((assignee) => ({
				login: String(assignee.login),
				avatarUrl: String(assignee.avatar_url ?? ''),
				htmlUrl: String(assignee.html_url ?? `https://github.com/${assignee.login}`)
			}));
	} catch {
		return [];
	}
}

function parseRationale(value: string): {
	area?: { rootSlug: string; leafSlug: string };
	signature?: string;
} {
	try {
		return JSON.parse(value || '{}');
	} catch {
		return {};
	}
}

function prStatus(row: Record<string, unknown>): string {
	if (row.merged_at) return 'Merged';
	const state = String(row.state ?? '');
	if (state === 'closed') return 'Closed without merge';
	if (Number(row.draft ?? 0) === 1) return 'Draft';
	if (String(row.review_decision ?? '') === 'CHANGES_REQUESTED') return 'Changes requested';
	if (['FAILURE', 'ERROR'].includes(String(row.checks_state ?? ''))) return 'Checks failing';
	if (
		String(row.review_decision ?? '') === 'APPROVED' &&
		['SUCCESS', 'NEUTRAL'].includes(String(row.checks_state ?? '')) &&
		String(row.mergeable ?? '') === 'MERGEABLE'
	) {
		return 'Ready by visible checks';
	}
	if (String(row.review_decision ?? '') === 'APPROVED') return 'Approved; waiting on checks';
	return 'Open; waiting for review';
}

export function getLastSyncedAt(): string {
	const db = new DatabaseSync(DB_PATH, { readOnly: true });
	try {
		try {
			const row = db
				.prepare(
					`select finished_at from sync_runs
					 where status = 'ok' and finished_at != ''
					 order by id desc limit 1`
				)
				.get() as { finished_at?: string } | undefined;
			if (row?.finished_at) return row.finished_at;
		} catch {
			// Older databases may not have sync_runs yet.
		}
		const row = db
			.prepare(
				`select max(last_sync_completed_at) as updated_at from sync_state
				 where last_sync_completed_at != ''`
			)
			.get() as { updated_at?: string } | undefined;
		return row?.updated_at ?? '';
	} finally {
		db.close();
	}
}

function linkedPullRequests(
	db: DatabaseSync,
	repo: string,
	numbers: number[]
): Map<number, PullRequestSummary[]> {
	const result = new Map<number, PullRequestSummary[]>();
	if (numbers.length === 0) return result;
	const placeholders = numbers.map(() => '?').join(',');
	const rows = db
		.prepare(
			`select p.id, p.number, p.title, p.state, p.merged_at, p.author_login, p.html_url,
			        r.kind, r.target_number,
			        coalesce(ps.draft, 0) draft,
			        coalesce(ps.review_decision, '') review_decision,
			        coalesce(ps.mergeable, '') mergeable,
			        coalesce(ps.checks_state, '') checks_state
			 from refs r
			 join pull_requests p on p.id = r.from_pr_id
			 left join pull_request_status ps on ps.pr_id = p.id
			 where r.kind in ('fixes', 'closes', 'resolves', 'linked_pr', 'mention')
			   and r.target_number in (${placeholders})
			   and (r.target_repo = ? or r.target_repo = '')
			 order by p.number desc`
		)
		.all(...numbers, repo) as unknown as Array<Record<string, unknown>>;

	const seen = new Set<string>();
	for (const row of rows) {
		const target = Number(row.target_number);
		const key = `${row.id}:${target}`;
		if (seen.has(key)) continue;
		seen.add(key);
		const pullRequest: PullRequestSummary = {
			id: Number(row.id),
			number: Number(row.number),
			title: String(row.title ?? ''),
			state: String(row.state ?? ''),
			mergedAt: String(row.merged_at ?? ''),
			author: String(row.author_login ?? ''),
			url: String(row.html_url ?? ''),
			relation: String(row.kind ?? 'linked'),
			draft: Number(row.draft ?? 0) === 1,
			reviewDecision: String(row.review_decision ?? ''),
			mergeable: String(row.mergeable ?? ''),
			checksState: String(row.checks_state ?? ''),
			status: prStatus(row)
		};
		const list = result.get(target) ?? [];
		list.push(pullRequest);
		result.set(target, list);
	}
	return result;
}

export function getIssueDetail(repo: string, number: number): IssueDetail | null {
	const db = new DatabaseSync(DB_PATH, { readOnly: true });
	try {
		const row = db
			.prepare(
				`select i.id, i.number, i.title, i.state, i.html_url, i.created_at, i.updated_at,
				        i.raw_json, r.slug repo
				 from issues i join repos r on r.id = i.repo_id
				 where r.slug = ? and i.number = ?`
			)
			.get(repo, number) as unknown as Record<string, unknown> | undefined;
		if (!row) return null;
		const labels = db
			.prepare(
				`select l.name from issue_labels il join labels l on l.id = il.label_id
				 where il.issue_id = ? order by l.name`
			)
			.all(Number(row.id)) as unknown as Array<{ name: string }>;
		const cluster = db
			.prepare('select cluster_id from cluster_issues where issue_id = ? limit 1')
			.get(Number(row.id)) as { cluster_id?: number } | undefined;
		const byIssue = linkedPullRequests(db, repo, [number]);
		return {
			id: Number(row.id),
			repo: String(row.repo),
			number: Number(row.number),
			title: String(row.title ?? ''),
			state: String(row.state ?? ''),
			url: String(row.html_url ?? ''),
			assignees: parseAssignees(String(row.raw_json ?? '{}')),
			updatedAt: String(row.updated_at ?? ''),
			createdAt: String(row.created_at ?? ''),
			clusterId: cluster?.cluster_id ?? null,
			labels: labels.map((label) => label.name),
			pullRequests: byIssue.get(number) ?? [],
			lastSyncedAt: getLastSyncedAt()
		};
	} finally {
		db.close();
	}
}

export function getClusterDetail(id: number): ClusterDetail | null {
	const db = new DatabaseSync(DB_PATH, { readOnly: true });
	try {
		const cluster = db.prepare('select id, name, rationale from clusters where id = ?').get(id) as
			{ id: number; name: string; rationale: string } | undefined;
		if (!cluster) return null;
		const rationale = parseRationale(cluster.rationale);
		const issues = db
			.prepare(
				`select i.id, i.number, i.title, i.state, i.html_url, i.updated_at, i.raw_json, r.slug repo
				 from cluster_issues ci
				 join issues i on i.id = ci.issue_id
				 join repos r on r.id = i.repo_id
				 where ci.cluster_id = ?
				 order by i.number desc`
			)
			.all(id) as unknown as Array<Record<string, unknown>>;
		const issueSummaries: IssueSummary[] = issues.map((issue) => ({
			id: Number(issue.id),
			repo: String(issue.repo),
			number: Number(issue.number),
			title: String(issue.title ?? ''),
			state: String(issue.state ?? ''),
			url: String(issue.html_url ?? ''),
			assignees: parseAssignees(String(issue.raw_json ?? '{}')),
			updatedAt: String(issue.updated_at ?? '')
		}));
		const prMap = new Map<number, PullRequestSummary>();
		const numbersByRepo = new Map<string, number[]>();
		for (const issue of issues) {
			const repo = String(issue.repo);
			const list = numbersByRepo.get(repo) ?? [];
			list.push(Number(issue.number));
			numbersByRepo.set(repo, list);
		}
		for (const [repo, numbers] of numbersByRepo) {
			const linked = linkedPullRequests(db, repo, numbers);
			for (const list of linked.values()) {
				for (const pr of list) prMap.set(pr.id, pr);
			}
		}
		return {
			id: cluster.id,
			name: cluster.name,
			area: rationale.area?.rootSlug ?? 'unclassified',
			topic: rationale.area?.leafSlug ?? 'general',
			signature: rationale.signature ?? '',
			issueCount: issueSummaries.length,
			openCount: issueSummaries.filter((issue) => issue.state === 'open').length,
			issues: issueSummaries,
			pullRequests: [...prMap.values()],
			lastSyncedAt: getLastSyncedAt()
		};
	} finally {
		db.close();
	}
}

export function getUnclustered(
	filters: { area?: string; topic?: string; repo?: string; limit?: number } = {}
): UnclusteredIssue[] {
	const db = new DatabaseSync(DB_PATH, { readOnly: true });
	try {
		const rows = db
			.prepare(
				`select i.id, i.number, i.title, i.state, i.html_url, i.updated_at, i.raw_json, r.slug repo,
				        (select group_concat(t.slug, ' ') from issue_tags it
				          join tags t on t.id = it.tag_id
				          where it.issue_id = i.id and t.kind = 'area') area_tags
				 from issues i
				 join repos r on r.id = i.repo_id
				 left join cluster_issues ci on ci.issue_id = i.id
				 where ci.issue_id is null
				 order by i.number desc`
			)
			.all() as unknown as Array<Record<string, unknown>>;
		const items: UnclusteredIssue[] = [];
		for (const row of rows) {
			const repo = String(row.repo ?? '');
			if (filters.repo && repo !== filters.repo) continue;
			const tags = String(row.area_tags ?? '')
				.split(/\s+/)
				.filter(Boolean)
				.map((slug) => slug.split('/'))
				.sort(
					(left, right) =>
						right.length - left.length || left.join('/').localeCompare(right.join('/'))
				);
			const primary = tags[0] ?? ['unclassified'];
			const area = primary[0] ?? 'unclassified';
			const topic = primary.length > 1 ? primary.slice(1).join('/') : 'general';
			if (filters.area && area !== filters.area) continue;
			if (filters.topic && topic !== filters.topic) continue;
			items.push({
				id: Number(row.id),
				repo,
				number: Number(row.number),
				title: String(row.title ?? ''),
				state: String(row.state ?? ''),
				url: String(row.html_url ?? ''),
				assignees: parseAssignees(String(row.raw_json ?? '{}')),
				updatedAt: String(row.updated_at ?? ''),
				area,
				topic
			});
		}
		return filters.limit ? items.slice(0, filters.limit) : items;
	} finally {
		db.close();
	}
}
