#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

const root = resolve(import.meta.dirname, '..');
const triageDir = join(root, 'triage');
const dbPath = resolve(process.env.TRIAGE_DB_PATH ?? join(root, 'data', 'triage.sqlite'));
const binary =
	process.env.TRIAGE_BIN ??
	join(triageDir, 'bin', process.platform === 'win32' ? 'triage.exe' : 'triage');
const noRetag = process.argv.includes('--no-retag');
const syncArgs = process.argv.slice(2).filter((arg) => arg !== '--no-retag');
let activeRunId = 0;

function run(command, args, options = {}) {
	return new Promise((resolvePromise) => {
		const child = spawn(command, args, {
			cwd: options.cwd ?? root,
			env: { ...process.env, ...(options.env ?? {}) },
			stdio: ['ignore', 'pipe', 'pipe']
		});
		child.stdout?.on('data', (chunk) => options.onOutput?.(chunk));
		child.stderr?.on('data', (chunk) => options.onError?.(chunk));
		child.on('error', (error) => resolvePromise({ code: 1, error }));
		child.on('close', (code) => resolvePromise({ code: code ?? 1 }));
	});
}

async function ensureBinary() {
	if (existsSync(binary)) return;
	process.stderr.write(`building ${binary}\n`);
	const result = await run('go', ['build', '-o', 'bin/triage', './cmd/triage'], {
		cwd: triageDir,
		onOutput: (chunk) => process.stdout.write(chunk),
		onError: (chunk) => process.stderr.write(chunk)
	});
	if (result.code !== 0) throw new Error('failed to build triage binary');
}

function readToken() {
	if (process.env.GITHUB_TOKEN) return process.env.GITHUB_TOKEN;
	if (process.env.GH_TOKEN) return process.env.GH_TOKEN;
	const result = spawn('gh', ['auth', 'token'], { stdio: ['ignore', 'pipe', 'pipe'] });
	let value = '';
	let error = '';
	result.stdout?.on('data', (chunk) => (value += chunk));
	result.stderr?.on('data', (chunk) => (error += chunk));
	return new Promise((resolveToken, rejectToken) => {
		result.on('error', rejectToken);
		result.on('close', (code) => {
			if (code === 0 && value.trim()) resolveToken(value.trim());
			else rejectToken(new Error(error.trim() || 'gh auth token failed'));
		});
	});
}

function markFailed(error) {
	if (!activeRunId) return;
	try {
		const update = new DatabaseSync(dbPath);
		update.exec('PRAGMA busy_timeout=10000');
		update
			.prepare('update sync_runs set finished_at=?, status=?, error=? where id=?')
			.run(
				new Date().toISOString(),
				'failed',
				error instanceof Error ? error.message : String(error),
				activeRunId
			);
		update.close();
	} catch {
		// The original error is more useful than a status-update failure.
	}
}

async function main() {
	await ensureBinary();
	const db = new DatabaseSync(dbPath);
	db.exec('PRAGMA busy_timeout=10000');
	db.exec(`CREATE TABLE IF NOT EXISTS sync_runs (
		id INTEGER PRIMARY KEY,
		repo TEXT NOT NULL DEFAULT '',
		started_at TEXT NOT NULL DEFAULT '',
		finished_at TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'running',
		mode TEXT NOT NULL DEFAULT '',
		issues INTEGER NOT NULL DEFAULT 0,
		comments INTEGER NOT NULL DEFAULT 0,
		refs INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT ''
	)`);
	const startedAt = new Date().toISOString();
	const full = syncArgs.includes('--full');
	const runId = Number(
		db
			.prepare(
				`insert into sync_runs(repo, started_at, status, mode)
				 values('all', ?, 'running', ?)`
			)
			.run(startedAt, full ? 'full' : 'incremental').lastInsertRowid
	);
	activeRunId = runId;
	db.close();

	const ghToken = await readToken();
	const env = { ...process.env, TRIAGE_DB: dbPath, GITHUB_TOKEN: ghToken };
	delete env.GH_TOKEN;

	let failed = false;
	const sync = await run(binary, ['sync', ...syncArgs], {
		cwd: triageDir,
		env,
		onOutput: (chunk) => process.stdout.write(chunk),
		onError: (chunk) => process.stderr.write(chunk)
	});
	if (sync.code !== 0) failed = true;

	if (!failed && !noRetag) {
		const retag = await run(binary, ['retag'], {
			cwd: triageDir,
			env,
			onOutput: (chunk) => process.stdout.write(chunk),
			onError: (chunk) => process.stderr.write(chunk)
		});
		if (retag.code !== 0) failed = true;
	}

	const finishedAt = new Date().toISOString();
	const update = new DatabaseSync(dbPath);
	update.exec('PRAGMA busy_timeout=10000');
	update
		.prepare(`update sync_runs set finished_at=?, status=?, error=? where id=?`)
		.run(finishedAt, failed ? 'failed' : 'ok', failed ? 'sync command failed' : '', runId);
	update.close();

	if (failed) process.exitCode = 1;
}

main().catch(async (error) => {
	markFailed(error);
	process.stderr.write(`sync: ${error instanceof Error ? error.message : String(error)}\n`);
	process.exitCode = 1;
});
