#!/usr/bin/env node
import { DatabaseSync } from 'node:sqlite';
import { parseArgs } from 'node:util';

const dbPath = process.env.TRIAGE_DB_PATH ?? '/data/triage.sqlite';
const db = new DatabaseSync(dbPath);
db.exec('PRAGMA busy_timeout=10000');

const { values, positionals } = parseArgs({
	allowPositionals: true,
	options: {
		area: { type: 'string' },
		topic: { type: 'string' },
		label: { type: 'string' },
		signature: { type: 'string' },
		cluster: { type: 'string' },
		issues: { type: 'string' },
		from: { type: 'string' },
		into: { type: 'string' },
		actor: { type: 'string', default: 'opencode-organizer' },
		limit: { type: 'string', default: '100' }
	}
});

const command = positionals[0];
const now = new Date().toISOString();
const actor = values.actor;

function ids(value) {
	return (value ?? '')
		.split(',')
		.map((part) => Number(part.trim()))
		.filter((id) => Number.isInteger(id) && id > 0);
}

function event(clusterId, action, payload) {
	db.prepare(
		`insert into cluster_events(cluster_id, actor, action, payload, created_at)
		 values(?, ?, ?, ?, ?)`
	).run(clusterId, actor, action, JSON.stringify(payload), now);
}

function issueExists(id) {
	return Boolean(db.prepare('select id from issues where id=?').get(id));
}

function clusterExists(id) {
	return Boolean(db.prepare('select id from clusters where id=?').get(id));
}

function addCluster(area, topic, label, signature, issueIds) {
	db.exec('BEGIN IMMEDIATE');
	const tag = db
		.prepare('select id from tags where slug=?')
		.get(topic === 'general' ? area : `${area}/${topic}`);
	const rationale = JSON.stringify({
		area: { rootSlug: area, leafSlug: topic },
		signature: signature ?? '',
		source: 'organizer'
	});
	const result = db
		.prepare(
			`insert into clusters(name, rationale, root_tag_id, score, created_at, updated_at)
			 values(?, ?, ?, 0, ?, ?)`
		)
		.run(label, rationale, tag?.id ?? null, now, now);
	const clusterId = Number(result.lastInsertRowid);
	db.prepare(
		`insert into cluster_queue(cluster_id, status, version, updated_at)
		 values(?, 'new', 0, ?)`
	).run(clusterId, now);
	const move = db.prepare(
		`insert into cluster_issues(cluster_id, issue_id, role) values(?, ?, 'member')`
	);
	for (const issueId of issueIds) {
		if (!issueExists(issueId)) throw new Error(`issue ${issueId} not found`);
		db.prepare('delete from cluster_issues where issue_id=?').run(issueId);
		move.run(clusterId, issueId);
	}
	db.prepare('update clusters set score=? where id=?').run(issueIds.length, clusterId);
	event(clusterId, 'create', { area, topic, issueIds });
	db.exec('COMMIT');
	console.log(clusterId);
}

function assign(clusterId, issueIds) {
	if (!clusterExists(clusterId)) throw new Error(`cluster ${clusterId} not found`);
	db.exec('BEGIN IMMEDIATE');
	const move = db.prepare(
		`insert into cluster_issues(cluster_id, issue_id, role) values(?, ?, 'member')`
	);
	for (const issueId of issueIds) {
		if (!issueExists(issueId)) throw new Error(`issue ${issueId} not found`);
		db.prepare('delete from cluster_issues where issue_id=?').run(issueId);
		move.run(clusterId, issueId);
	}
	db.prepare(
		`update clusters set score=(select count(*) from cluster_issues where cluster_id=?),
		 updated_at=? where id=?`
	).run(clusterId, now, clusterId);
	event(clusterId, 'assign', { issueIds });
	db.exec('COMMIT');
	console.log(`assigned ${issueIds.length} issue(s) to cluster ${clusterId}`);
}

function merge(from, into) {
	if (!clusterExists(from) || !clusterExists(into)) throw new Error('cluster not found');
	if (from === into) throw new Error('cannot merge a cluster into itself');
	db.exec('BEGIN IMMEDIATE');
	const members = db.prepare('select issue_id from cluster_issues where cluster_id=?').all(from);
	const move = db.prepare(
		`insert or ignore into cluster_issues(cluster_id, issue_id, role) values(?, ?, 'member')`
	);
	for (const member of members) move.run(into, member.issue_id);
	db.prepare('delete from cluster_issues where cluster_id=?').run(from);
	db.prepare('delete from clusters where id=?').run(from);
	db.prepare(
		`update clusters set score=(select count(*) from cluster_issues where cluster_id=?),
		 updated_at=? where id=?`
	).run(into, now, into);
	event(into, 'merge', { from, into, moved: members.length });
	db.exec('COMMIT');
	console.log(`merged ${from} into ${into} (${members.length} issues)`);
}

function unclustered() {
	const limit = Math.min(500, Math.max(1, Number(values.limit) || 100));
	const rows = db
		.prepare(
			`select i.id, i.number, i.title, r.slug repo,
			        (select group_concat(t.slug, ' ') from issue_tags it
			          join tags t on t.id=it.tag_id
			          where it.issue_id=i.id and t.kind='area') areas
			 from issues i join repos r on r.id=i.repo_id
			 left join cluster_issues ci on ci.issue_id=i.id
			 where ci.issue_id is null
			 order by i.number desc limit ?`
		)
		.all(limit);
	for (const row of rows) {
		const areas = String(row.areas ?? '');
		if (values.area && !areas.split(/\s+/).includes(values.area)) continue;
		if (values.topic && !areas.split(/\s+/).some((slug) => slug.endsWith(`/${values.topic}`)))
			continue;
		console.log(`${row.id}\t${row.repo}#${row.number}\t${row.areas}\t${row.title}`);
	}
}

try {
	if (command === 'create') {
		if (!values.area || !values.topic || !values.label) {
			throw new Error('create requires --area, --topic and --label');
		}
		addCluster(values.area, values.topic, values.label, values.signature, ids(values.issues));
	} else if (command === 'assign') {
		assign(Number(values.cluster), ids(values.issues));
	} else if (command === 'merge') {
		merge(Number(values.from), Number(values.into));
	} else if (command === 'unclustered') {
		unclustered();
	} else {
		console.error(`usage:
  organize unclustered [--area kernel] [--topic booleans] [--limit N]
  organize create --area kernel --topic booleans --label TEXT [--signature TEXT] [--issues 1,2]
  organize assign --cluster N --issues 1,2
  organize merge --from N --into M`);
		process.exitCode = 1;
	}
} finally {
	db.close();
}
