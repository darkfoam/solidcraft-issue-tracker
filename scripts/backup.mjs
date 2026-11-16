#!/usr/bin/env node
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { DatabaseSync } from 'node:sqlite';

const dbPath = process.env.TRIAGE_DB_PATH ?? '/data/triage.sqlite';
const backupDir = process.env.BACKUP_DIR ?? '/data/backups';
const stamp = new Date().toISOString().replace(/[:.]/g, '-');
const output = join(backupDir, `triage-${stamp}.sqlite`);

mkdirSync(backupDir, { recursive: true });
const db = new DatabaseSync(dbPath);
db.exec('PRAGMA busy_timeout=10000');
db.exec(`VACUUM INTO '${output.replaceAll("'", "''")}'`);
db.close();
console.log(output);
