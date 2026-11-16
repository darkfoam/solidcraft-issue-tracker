#!/usr/bin/env node
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { registerReadonlyTools } from './readonly-tools.mjs';

const baseUrl = (process.env.ISSUES_API_URL ?? 'http://127.0.0.1:3000').replace(/\/$/, '');
const server = new McpServer({ name: 'solidcraft-issue-graph', version: '1.0.0' });
registerReadonlyTools(server);
await server.connect(new StdioServerTransport());
console.error(`solidcraft issue graph MCP connected to ${baseUrl} (read-only)`);
