#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StreamableHTTPServerTransport } from '@modelcontextprotocol/sdk/server/streamableHttp.js';
import { createMcpExpressApp } from '@modelcontextprotocol/sdk/server/express.js';
import { registerReadonlyTools } from './readonly-tools.mjs';

const port = Number(process.env.MCP_PORT ?? 3001);

function makeServer() {
	const server = new McpServer({ name: 'solidcraft-issue-graph', version: '1.0.0' });
	registerReadonlyTools(server);
	return server;
}

const allowedHosts = (
	process.env.MCP_ALLOWED_HOSTS ?? 'solidcraft.neilclack.com,127.0.0.1,localhost'
)
	.split(',')
	.map((host) => host.trim())
	.filter(Boolean);
const app = createMcpExpressApp({ host: '0.0.0.0', allowedHosts });

app.post('/mcp', async (req, res) => {
	const server = makeServer();
	try {
		const transport = new StreamableHTTPServerTransport({
			sessionIdGenerator: undefined
		});
		await server.connect(transport);
		await transport.handleRequest(req, res, req.body);
		res.on('close', () => {
			void transport.close();
			void server.close();
		});
	} catch (error) {
		console.error('MCP request failed:', error);
		if (!res.headersSent) {
			res.status(500).json({
				jsonrpc: '2.0',
				error: { code: -32603, message: 'Internal server error' },
				id: null
			});
		}
	}
});

for (const method of ['get', 'delete']) {
	app[method]('/mcp', (_req, res) => {
		res.status(405).json({
			jsonrpc: '2.0',
			error: { code: -32000, message: 'Method not allowed.' },
			id: null
		});
	});
}

app.listen(port, '0.0.0.0', () => {
	console.error(`solidcraft issue graph MCP listening on http://0.0.0.0:${port}/mcp (read-only)`);
});
