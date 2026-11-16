import { spawn } from 'node:child_process';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import { DB_PATH, invalidateGraphCache } from './graph';

export interface SyncStatus {
	running: boolean;
	startedAt: string;
	finishedAt: string;
	lastError: string;
	lastOutput: string;
	lastRun: {
		id: number;
		startedAt: string;
		finishedAt: string;
		status: string;
		mode: string;
		error: string;
	} | null;
	repositories: Array<{
		slug: string;
		lastSync: string;
		lastFullSync: string;
		watermark: string;
		error: string;
	}>;
}

let running = false;
let startedAt = '';
let finishedAt = '';
let lastError = '';
let output = '';
let scheduler: NodeJS.Timeout | null = null;

function append(chunk: Buffer): void {
	output = `${output}${chunk.toString()}`.slice(-24_000);
}

export function runSync(args: string[] = []): { accepted: boolean; message: string } {
	if (running) return { accepted: false, message: 'sync already running' };
	running = true;
	startedAt = new Date().toISOString();
	finishedAt = '';
	lastError = '';
	output = '';

	const child = spawn(process.execPath, [join(process.cwd(), 'scripts', 'sync.mjs'), ...args], {
		cwd: process.cwd(),
		env: process.env,
		stdio: ['ignore', 'pipe', 'pipe']
	});
	child.stdout?.on('data', append);
	child.stderr?.on('data', append);
	child.on('error', (error) => {
		lastError = error.message;
	});
	child.on('close', (code) => {
		running = false;
		finishedAt = new Date().toISOString();
		if (code !== 0 && !lastError) lastError = `sync exited with code ${code ?? 1}`;
		invalidateGraphCache();
	});
	return { accepted: true, message: 'sync started' };
}

export function getSyncStatus(): SyncStatus {
	let lastRun: SyncStatus['lastRun'] = null;
	const repositories: SyncStatus['repositories'] = [];
	try {
		const db = new DatabaseSync(DB_PATH, { readOnly: true });
		try {
			const row = db
				.prepare(
					`select id, started_at, finished_at, status, mode, error
					 from sync_runs order by id desc limit 1`
				)
				.get() as
				| {
						id: number;
						started_at: string;
						finished_at: string;
						status: string;
						mode: string;
						error: string;
				  }
				| undefined;
			if (row) {
				lastRun = {
					id: row.id,
					startedAt: row.started_at,
					finishedAt: row.finished_at,
					status: row.status,
					mode: row.mode,
					error: row.error
				};
			}
			const repoRows = db
				.prepare(
					`select r.slug, s.last_sync_completed_at, s.last_full_sync_completed_at,
					        s.since, s.last_error
					 from repos r left join sync_state s on s.repo_id = r.id
					 order by r.slug`
				)
				.all() as unknown as Array<Record<string, unknown>>;
			for (const repo of repoRows) {
				repositories.push({
					slug: String(repo.slug ?? ''),
					lastSync: String(repo.last_sync_completed_at ?? ''),
					lastFullSync: String(repo.last_full_sync_completed_at ?? ''),
					watermark: String(repo.since ?? ''),
					error: String(repo.last_error ?? '')
				});
			}
		} finally {
			db.close();
		}
	} catch (error) {
		lastError = lastError || (error instanceof Error ? error.message : String(error));
	}

	return {
		running,
		startedAt,
		finishedAt,
		lastError,
		lastOutput: output,
		lastRun,
		repositories
	};
}

export function startSyncScheduler(): void {
	if (scheduler) return;
	const interval = Number(process.env.SYNC_INTERVAL_MS ?? 0);
	if (!Number.isFinite(interval) || interval <= 0) return;
	scheduler = setInterval(
		() => {
			runSync([]);
		},
		Math.max(60_000, interval)
	);
	scheduler.unref();
	if (process.env.SYNC_ON_START === '1') {
		setTimeout(() => runSync([]), 5_000).unref();
	}
}
