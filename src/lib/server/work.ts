import { DatabaseSync } from 'node:sqlite';
import { DB_PATH, invalidateGraphCache } from './graph';

export interface QueueItem {
	clusterId: number;
	name: string;
	area: string;
	topic: string;
	signature: string;
	issueCount: number;
	openCount: number;
	status: string;
	priority: string;
	layer: string;
	assignee: string;
	note: string;
	version: number;
	claimedAt: string;
	claimExpiresAt: string;
	updatedAt: string;
}

export interface UnclusteredIssue {
	id: number;
	repo: string;
	number: number;
	title: string;
	state: string;
	url: string;
	area: string;
	topic: string;
}

export interface QueueFilters {
	area?: string;
	topic?: string;
	status?: string;
	assignee?: string;
	limit?: number;
}

export class WorkError extends Error {
	status: number;

	constructor(message: string, status = 400) {
		super(message);
		this.name = 'WorkError';
		this.status = status;
	}
}

function openRead() {
	return new DatabaseSync(DB_PATH, { readOnly: true });
}

function openWrite() {
	const db = new DatabaseSync(DB_PATH);
	db.exec('PRAGMA busy_timeout=10000');
	return db;
}

function now(): string {
	return new Date().toISOString();
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

export function getQueue(filters: QueueFilters = {}): QueueItem[] {
	const db = openRead();
	try {
		const rows = db
			.prepare(
				`select c.id, c.name, c.rationale, c.score,
				        coalesce(q.status, 'new') status,
				        coalesce(q.priority, 'none') priority,
				        coalesce(q.layer, 'unknown') layer,
				        coalesce(q.assignee, '') assignee,
				        coalesce(q.note, '') note,
				        coalesce(q.version, 0) version,
				        coalesce(q.claimed_at, '') claimed_at,
				        coalesce(q.claim_expires_at, '') claim_expires_at,
				        coalesce(q.updated_at, '') updated_at,
				        (select count(*) from cluster_issues ci where ci.cluster_id = c.id) issue_count,
				        (select count(*) from cluster_issues ci
				          join issues i on i.id = ci.issue_id
				          where ci.cluster_id = c.id and i.state = 'open') open_count
				 from clusters c
				 left join cluster_queue q on q.cluster_id = c.id
				 order by open_count desc, issue_count desc, c.id`
			)
			.all() as unknown as Array<Record<string, unknown>>;

		const items: QueueItem[] = [];
		for (const row of rows) {
			const rationale = parseRationale(String(row.rationale ?? ''));
			const area = rationale.area?.rootSlug ?? 'unclassified';
			const topic = rationale.area?.leafSlug ?? 'general';
			if (filters.area && area !== filters.area) continue;
			if (filters.topic && topic !== filters.topic) continue;
			if (filters.status && String(row.status ?? '') !== filters.status) continue;
			if (filters.assignee && String(row.assignee ?? '') !== filters.assignee) continue;
			items.push({
				clusterId: Number(row.id),
				name: String(row.name ?? ''),
				area,
				topic,
				signature: rationale.signature ?? '',
				issueCount: Number(row.issue_count ?? 0),
				openCount: Number(row.open_count ?? 0),
				status: String(row.status ?? 'new'),
				priority: String(row.priority ?? 'none'),
				layer: String(row.layer ?? 'unknown'),
				assignee: String(row.assignee ?? ''),
				note: String(row.note ?? ''),
				version: Number(row.version ?? 0),
				claimedAt: String(row.claimed_at ?? ''),
				claimExpiresAt: String(row.claim_expires_at ?? ''),
				updatedAt: String(row.updated_at ?? '')
			});
		}
		return filters.limit ? items.slice(0, filters.limit) : items;
	} finally {
		db.close();
	}
}

export function getQueueItem(clusterId: number): QueueItem | undefined {
	return getQueue().find((item) => item.clusterId === clusterId);
}

export function getUnclustered(filters: QueueFilters & { repo?: string } = {}): UnclusteredIssue[] {
	const db = openRead();
	try {
		const rows = db
			.prepare(
				`select i.id, i.number, i.title, i.state, i.html_url, r.slug repo,
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
				.filter(Boolean);
			const primary = tags
				.map((slug) => slug.split('/'))
				.sort(
					(left, right) =>
						right.length - left.length || left.join('/').localeCompare(right.join('/'))
				)[0] ?? ['unclassified'];
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
				area,
				topic
			});
		}
		return filters.limit ? items.slice(0, filters.limit) : items;
	} finally {
		db.close();
	}
}

function addEvent(
	db: DatabaseSync,
	clusterId: number,
	actor: string,
	action: string,
	payload: unknown
): void {
	db.prepare(
		`insert into cluster_events(cluster_id, actor, action, payload, created_at)
		 values(?, ?, ?, ?, ?)`
	).run(clusterId, actor, action, JSON.stringify(payload ?? {}), now());
}

function existingQueue(db: DatabaseSync, clusterId: number): Record<string, unknown> | undefined {
	return db.prepare('select * from cluster_queue where cluster_id=?').get(clusterId) as unknown as
		Record<string, unknown> | undefined;
}

export function claimCluster(clusterId: number, actor: string, ttlMinutes = 60): QueueItem {
	if (!actor.trim()) throw new WorkError('actor is required');
	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const cluster = db.prepare('select id from clusters where id=?').get(clusterId);
		if (!cluster) throw new WorkError('cluster not found', 404);
		const current = existingQueue(db, clusterId);
		const expiry = current?.claim_expires_at
			? new Date(String(current.claim_expires_at)).getTime()
			: 0;
		if (
			current?.assignee &&
			String(current.assignee) !== actor &&
			Number.isFinite(expiry) &&
			expiry > Date.now()
		) {
			throw new WorkError(`cluster is claimed by ${String(current.assignee)}`, 409);
		}
		const claimedAt = now();
		const expires = new Date(Date.now() + Math.max(5, ttlMinutes) * 60_000).toISOString();
		db.prepare(
			`insert into cluster_queue(cluster_id, status, assignee, claimed_at, claim_expires_at, version, updated_at)
			 values(?, 'claimed', ?, ?, ?, 1, ?)
			 on conflict(cluster_id) do update set
			   status='claimed', assignee=excluded.assignee, claimed_at=excluded.claimed_at,
			   claim_expires_at=excluded.claim_expires_at, version=cluster_queue.version+1,
			   updated_at=excluded.updated_at`
		).run(clusterId, actor, claimedAt, expires, claimedAt);
		addEvent(db, clusterId, actor, 'claim', { ttlMinutes });
		db.exec('COMMIT');
		invalidateGraphCache();
		const item = getQueueItem(clusterId);
		if (!item) throw new WorkError('cluster queue not found', 404);
		return item;
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}

export function releaseCluster(clusterId: number, actor: string): void {
	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const current = existingQueue(db, clusterId);
		if (!current) throw new WorkError('cluster queue not found', 404);
		db.prepare(
			`update cluster_queue set status='triaged', assignee='', claim_expires_at='',
			 version=version+1, updated_at=? where cluster_id=?`
		).run(now(), clusterId);
		addEvent(db, clusterId, actor, 'release', { previousAssignee: current.assignee ?? '' });
		db.exec('COMMIT');
		invalidateGraphCache();
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}

export function updateCluster(
	clusterId: number,
	patch: {
		status?: string;
		priority?: string;
		layer?: string;
		assignee?: string;
		note?: string;
		version?: number;
	},
	actor: string
): void {
	const allowedStatus = ['new', 'triaged', 'claimed', 'in_progress', 'blocked', 'done', 'wont_fix'];
	const allowedPriority = ['none', 'p0', 'p1', 'p2', 'p3'];
	const allowedLayer = ['app', 'ops', 'kernel', 'unknown'];
	if (patch.status && !allowedStatus.includes(patch.status)) throw new WorkError('invalid status');
	if (patch.priority && !allowedPriority.includes(patch.priority))
		throw new WorkError('invalid priority');
	if (patch.layer && !allowedLayer.includes(patch.layer)) throw new WorkError('invalid layer');

	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const current = existingQueue(db, clusterId);
		if (!current) throw new WorkError('cluster queue not found', 404);
		if (patch.version !== undefined && Number(current.version) !== patch.version) {
			throw new WorkError('version conflict', 409);
		}
		const nextStatus = patch.status ?? String(current.status);
		const nextPriority = patch.priority ?? String(current.priority);
		const nextLayer = patch.layer ?? String(current.layer);
		const nextAssignee = patch.assignee ?? String(current.assignee ?? '');
		const nextNote = patch.note ?? String(current.note ?? '');
		db.prepare(
			`update cluster_queue set status=?, priority=?, layer=?, assignee=?, note=?,
			 version=version+1, updated_at=? where cluster_id=?`
		).run(nextStatus, nextPriority, nextLayer, nextAssignee, nextNote, now(), clusterId);
		addEvent(db, clusterId, actor, 'update', patch);
		db.exec('COMMIT');
		invalidateGraphCache();
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}

export function assignIssues(clusterId: number, issueIds: number[], actor: string): void {
	issueIds = [...new Set(issueIds)];
	if (issueIds.length === 0) throw new WorkError('issueIds must not be empty');
	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const cluster = db.prepare('select id from clusters where id=?').get(clusterId);
		if (!cluster) throw new WorkError('cluster not found', 404);
		const placeholders = issueIds.map(() => '?').join(',');
		const found = db
			.prepare(`select id from issues where id in (${placeholders})`)
			.all(...issueIds) as unknown as Array<{ id: number }>;
		if (found.length !== issueIds.length) throw new WorkError('one or more issues not found', 404);
		db.prepare(`delete from cluster_issues where issue_id in (${placeholders})`).run(...issueIds);
		const insert = db.prepare(
			`insert into cluster_issues(cluster_id, issue_id, role) values(?, ?, 'member')`
		);
		for (const issueId of issueIds) insert.run(clusterId, issueId);
		db.prepare(
			`update clusters set
			   score=(select count(*) from cluster_issues where cluster_id=?),
			   updated_at=? where id=?`
		).run(clusterId, now(), clusterId);
		db.prepare(
			`insert into cluster_queue(cluster_id, status, version, updated_at)
			 values(?, 'triaged', 1, ?)
			 on conflict(cluster_id) do update set status='triaged', version=cluster_queue.version+1,
			 updated_at=excluded.updated_at`
		).run(clusterId, now());
		addEvent(db, clusterId, actor, 'assign_issues', { issueIds });
		db.exec('COMMIT');
		invalidateGraphCache();
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}

export function createCluster(
	input: {
		area: string;
		topic: string;
		label: string;
		signature?: string;
		issueIds?: number[];
	},
	actor: string
): number {
	if (!input.area.trim() || !input.topic.trim() || !input.label.trim()) {
		throw new WorkError('area, topic and label are required');
	}
	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const tag = db
			.prepare('select id from tags where slug=?')
			.get(input.topic === 'general' ? input.area : `${input.area}/${input.topic}`) as
			{ id: number } | undefined;
		const rationale = JSON.stringify({
			area: { rootSlug: input.area, leafSlug: input.topic },
			signature: input.signature ?? '',
			source: 'agent'
		});
		const result = db
			.prepare(
				`insert into clusters(name, rationale, root_tag_id, score, created_at, updated_at)
				 values(?, ?, ?, 0, ?, ?)`
			)
			.run(input.label, rationale, tag?.id ?? null, now(), now());
		const id = Number(result.lastInsertRowid);
		db.prepare(`insert into cluster_queue(cluster_id, status, updated_at) values(?, 'new', ?)`).run(
			id,
			now()
		);
		addEvent(db, id, actor, 'create', { area: input.area, topic: input.topic });
		db.exec('COMMIT');
		invalidateGraphCache();
		if (input.issueIds?.length) assignIssues(id, input.issueIds, actor);
		return id;
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}

export function mergeClusters(sourceId: number, targetId: number, actor: string): void {
	if (sourceId === targetId) throw new WorkError('cannot merge a cluster into itself');
	const db = openWrite();
	try {
		db.exec('BEGIN IMMEDIATE');
		const source = db.prepare('select id from clusters where id=?').get(sourceId);
		const target = db.prepare('select id from clusters where id=?').get(targetId);
		if (!source || !target) throw new WorkError('source or target cluster not found', 404);
		const members = db
			.prepare('select issue_id from cluster_issues where cluster_id=?')
			.all(sourceId) as unknown as Array<{ issue_id: number }>;
		const move = db.prepare(
			`insert or ignore into cluster_issues(cluster_id, issue_id, role) values(?, ?, 'member')`
		);
		for (const member of members) move.run(targetId, member.issue_id);
		db.prepare('delete from cluster_issues where cluster_id=?').run(sourceId);
		db.prepare('delete from clusters where id=?').run(sourceId);
		db.prepare(
			`update clusters set
			   score=(select count(*) from cluster_issues where cluster_id=?),
			   updated_at=? where id=?`
		).run(targetId, now(), targetId);
		db.prepare(
			`insert into cluster_queue(cluster_id, status, version, updated_at)
			 values(?, 'triaged', 1, ?)
			 on conflict(cluster_id) do update set status='triaged',
			 version=cluster_queue.version+1, updated_at=excluded.updated_at`
		).run(targetId, now());
		addEvent(db, targetId, actor, 'merge', { sourceId, moved: members.length });
		db.exec('COMMIT');
		invalidateGraphCache();
	} catch (error) {
		try {
			db.exec('ROLLBACK');
		} catch {
			// ignore
		}
		throw error;
	} finally {
		db.close();
	}
}
