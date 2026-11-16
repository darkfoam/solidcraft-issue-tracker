import { building } from '$app/env';
import { startSyncScheduler } from './lib/server/sync';

if (!building) startSyncScheduler();
