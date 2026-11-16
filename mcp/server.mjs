#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { z } from 'zod';

const baseUrl = (process.env.ISSUES_API_URL ?? 'http://127.0.0.1:5173').replace(/\/$/, '');
const token = process.env.AGENT_TOKEN ?? '';

async function api(path, options = {}) {
	const headers = { ...(options.headers ?? {}) };
	if (options.body) headers['content-type'] = 'application/json';
	if (token) headers.authorization = `Bearer ${token}`;
	const response = await fetch(`${baseUrl}${path}`, { ...options, headers });
	const body = await response.text();
	let parsed;
	try {
		parsed = body ? JSON.parse(body) : null;
	} catch {
		parsed = body;
	}
	if (!response.ok) {
		const message = typeof parsed === 'object' && parsed && 'error' in parsed ? parsed.error : body;
		throw new Error(`${response.status} ${message || response.statusText}`);
	}
	return parsed;
}

function result(value) {
	return {
		content: [
			{
				type: 'text',
				text: typeof value === 'string' ? value : JSON.stringify(value, null, 2)
			}
		]
	};
}

function failure(error) {
	return {
		isError: true,
		content: [
			{
				type: 'text',
				text: error instanceof Error ? error.message : String(error)
			}
		]
	};
}

const server = new McpServer({
	name: 'solidcraft-issue-queue',
	version: '0.1.0'
});

server.registerTool(
	'list_queue',
	{
		title: 'List triage queue clusters',
		description:
			'List materialized distinct-issue clusters with queue status, open counts, priority, layer and claim state. Filter by area, topic and status.',
		inputSchema: {
			area: z.string().optional().describe('Area slug such as kernel, ui or io'),
			topic: z.string().optional().describe('Topic slug such as booleans'),
			status: z
				.enum(['new', 'triaged', 'claimed', 'in_progress', 'blocked', 'done', 'wont_fix'])
				.optional(),
			assignee: z.string().optional(),
			limit: z.number().int().min(1).max(500).optional()
		}
	},
	async (args) => {
		try {
			const query = new URLSearchParams();
			for (const [key, value] of Object.entries(args)) {
				if (value !== undefined) query.set(key, String(value));
			}
			return result(await api(`/api/queue?${query}`));
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'list_unclustered',
	{
		title: 'List issues that need grouping',
		description:
			'List issues not yet assigned to a distinct-issue cluster. These are the new or ungrouped reports an agent should triage first.',
		inputSchema: {
			area: z.string().optional(),
			topic: z.string().optional(),
			repo: z.string().optional(),
			limit: z.number().int().min(1).max(500).optional()
		}
	},
	async (args) => {
		try {
			const query = new URLSearchParams();
			for (const [key, value] of Object.entries(args)) {
				if (value !== undefined) query.set(key, String(value));
			}
			return result(await api(`/api/unclustered?${query}`));
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'get_cluster',
	{
		title: 'Get a triage cluster',
		description: 'Read one cluster and its current queue state.',
		inputSchema: { clusterId: z.number().int().positive() }
	},
	async ({ clusterId }) => {
		try {
			return result(await api(`/api/clusters/${clusterId}`));
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'claim_cluster',
	{
		title: 'Claim a triage cluster',
		description:
			'Claim a distinct-issue cluster for an agent before working on it. Claims expire after ttlMinutes.',
		inputSchema: {
			clusterId: z.number().int().positive(),
			as: z.string().min(1),
			ttlMinutes: z.number().int().min(5).max(1440).optional()
		}
	},
	async ({ clusterId, as, ttlMinutes }) => {
		try {
			return result(
				await api(`/api/clusters/${clusterId}/claim`, {
					method: 'POST',
					body: JSON.stringify({ as, ttlMinutes })
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'release_cluster',
	{
		title: 'Release a triage cluster',
		description: 'Release a claim so another agent can work on the cluster.',
		inputSchema: { clusterId: z.number().int().positive(), actor: z.string().optional() }
	},
	async ({ clusterId, actor }) => {
		try {
			return result(
				await api(`/api/clusters/${clusterId}/release`, {
					method: 'POST',
					body: JSON.stringify({ actor })
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'update_cluster',
	{
		title: 'Update a triage cluster',
		description:
			'Update queue status, priority, layer, assignee or note. Pass the current version to avoid overwriting another agent.',
		inputSchema: {
			clusterId: z.number().int().positive(),
			status: z
				.enum(['new', 'triaged', 'claimed', 'in_progress', 'blocked', 'done', 'wont_fix'])
				.optional(),
			priority: z.enum(['none', 'p0', 'p1', 'p2', 'p3']).optional(),
			layer: z.enum(['app', 'ops', 'kernel', 'unknown']).optional(),
			assignee: z.string().optional(),
			note: z.string().optional(),
			version: z.number().int().min(0).optional(),
			actor: z.string().optional()
		}
	},
	async ({ clusterId, ...patch }) => {
		try {
			return result(
				await api(`/api/clusters/${clusterId}`, {
					method: 'PATCH',
					body: JSON.stringify(patch)
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'assign_issues',
	{
		title: 'Assign issues to a cluster',
		description:
			'Move issue IDs into a cluster. This is how an agent groups new reports under an existing distinct issue.',
		inputSchema: {
			clusterId: z.number().int().positive(),
			issueIds: z.array(z.number().int().positive()).min(1).max(500),
			actor: z.string().optional()
		}
	},
	async ({ clusterId, issueIds, actor }) => {
		try {
			return result(
				await api(`/api/clusters/${clusterId}/issues`, {
					method: 'POST',
					body: JSON.stringify({ issueIds, actor })
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'merge_clusters',
	{
		title: 'Merge two distinct-issue clusters',
		description:
			'Move every issue from one cluster into another and remove the source cluster. Use when two groups are the same bug.',
		inputSchema: {
			clusterId: z.number().int().positive(),
			into: z.number().int().positive(),
			actor: z.string().optional()
		}
	},
	async ({ clusterId, into, actor }) => {
		try {
			return result(
				await api(`/api/clusters/${clusterId}/merge`, {
					method: 'POST',
					body: JSON.stringify({ into, actor })
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'create_cluster',
	{
		title: 'Create a distinct-issue cluster',
		description:
			'Create a new cluster under an area/topic, optionally assigning existing issue IDs immediately.',
		inputSchema: {
			area: z.string().min(1),
			topic: z.string().min(1),
			label: z.string().min(1),
			signature: z.string().optional(),
			issueIds: z.array(z.number().int().positive()).max(500).optional(),
			actor: z.string().optional()
		}
	},
	async (args) => {
		try {
			return result(
				await api('/api/clusters', {
					method: 'POST',
					body: JSON.stringify(args)
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'sync_status',
	{
		title: 'Get database sync status',
		description: 'Read the last GitHub sync run and per-repository watermarks.'
	},
	async () => {
		try {
			return result(await api('/api/sync'));
		} catch (error) {
			return failure(error);
		}
	}
);

server.registerTool(
	'run_sync',
	{
		title: 'Run a database sync',
		description:
			'Start an incremental GitHub sync and retag. Requires MCP_ALLOW_SYNC=1 in the MCP server environment.',
		inputSchema: { full: z.boolean().optional() }
	},
	async ({ full }) => {
		if (process.env.MCP_ALLOW_SYNC !== '1') {
			return failure(new Error('MCP_ALLOW_SYNC is not set to 1'));
		}
		try {
			return result(
				await api('/api/sync', {
					method: 'POST',
					body: JSON.stringify({ full: Boolean(full) })
				})
			);
		} catch (error) {
			return failure(error);
		}
	}
);

await server.connect(new StdioServerTransport());
console.error(`solidcraft issue queue MCP connected to ${baseUrl}`);
