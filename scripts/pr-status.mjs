#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

const root = join(import.meta.dirname, '..');
const dbPath = process.env.TRIAGE_DB_PATH ?? join(root, 'data', 'triage.sqlite');

function readToken() {
	if (process.env.GITHUB_TOKEN) return Promise.resolve(process.env.GITHUB_TOKEN);
	if (process.env.GH_TOKEN) return Promise.resolve(process.env.GH_TOKEN);
	return new Promise((resolve, reject) => {
		const child = spawn('gh', ['auth', 'token'], { stdio: ['ignore', 'pipe', 'pipe'] });
		let value = '';
		let error = '';
		child.stdout.on('data', (chunk) => (value += chunk));
		child.stderr.on('data', (chunk) => (error += chunk));
		child.on('error', reject);
		child.on('close', (code) =>
			code === 0 && value.trim()
				? resolve(value.trim())
				: reject(new Error(error.trim() || 'gh auth token failed'))
		);
	});
}

async function graphql(token, repo, numbers) {
	const [owner, name] = repo.split('/');
	const fields = numbers
		.map(
			(number, index) =>
				`p${index}: pullRequest(number: ${number}) { isDraft reviewDecision mergeable mergeStateStatus headRefOid statusCheckRollup { state } }`
		)
		.join('\n');
	const query = `query($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			${fields}
		}
	}`;
	const response = await fetch('https://api.github.com/graphql', {
		method: 'POST',
		headers: {
			authorization: `bearer ${token}`,
			'content-type': 'application/json',
			'user-agent': 'solidcraft-issue-tracker'
		},
		body: JSON.stringify({ query, variables: { owner, name } })
	});
	const body = await response.json();
	if (!response.ok || body.errors) {
		throw new Error(JSON.stringify(body.errors ?? body));
	}
	return numbers.map((number, index) => ({
		number,
		value: body.data?.repository?.[`p${index}`] ?? null
	}));
}

async function main() {
	const token = await readToken();
	const db = new DatabaseSync(dbPath);
	db.exec('PRAGMA busy_timeout=10000');
	const rows = db
		.prepare(
			`select distinct p.id, p.number, r.slug repo
			 from pull_requests p
			 join repos r on r.id = p.repo_id
			 join refs x on x.from_pr_id = p.id
			 where p.state = 'open'
			   and x.kind in ('fixes', 'closes', 'resolves', 'linked_pr', 'mention')
			 order by r.slug, p.number`
		)
		.all();

	const byRepo = new Map();
	for (const row of rows) {
		const list = byRepo.get(row.repo) ?? [];
		list.push(row);
		byRepo.set(row.repo, list);
	}

	const update = db.prepare(
		`insert into pull_request_status
		 (pr_id, draft, review_decision, mergeable, mergeable_state, checks_state, head_sha, updated_at)
		 values (?, ?, ?, ?, ?, ?, ?, ?)
		 on conflict(pr_id) do update set
		   draft=excluded.draft, review_decision=excluded.review_decision,
		   mergeable=excluded.mergeable, mergeable_state=excluded.mergeable_state,
		   checks_state=excluded.checks_state, head_sha=excluded.head_sha,
		   updated_at=excluded.updated_at`
	);

	let updated = 0;
	for (const [repo, list] of byRepo) {
		for (let offset = 0; offset < list.length; offset += 20) {
			const batch = list.slice(offset, offset + 20);
			const values = await graphql(
				token,
				repo,
				batch.map((row) => row.number)
			);
			for (const item of values) {
				if (!item.value) continue;
				const row = batch.find((candidate) => candidate.number === item.number);
				if (!row) continue;
				update.run(
					row.id,
					item.value.isDraft ? 1 : 0,
					item.value.reviewDecision ?? '',
					item.value.mergeable ?? '',
					item.value.mergeStateStatus ?? '',
					item.value.statusCheckRollup?.state ?? '',
					item.value.headRefOid ?? '',
					new Date().toISOString()
				);
				updated++;
			}
			console.error(`${repo}: ${Math.min(offset + 20, list.length)}/${list.length}`);
		}
	}
	db.close();
	console.log(`updated ${updated} linked open PR statuses`);
}

main().catch((error) => {
	console.error(`pr-status: ${error instanceof Error ? error.message : String(error)}`);
	process.exitCode = 1;
});
