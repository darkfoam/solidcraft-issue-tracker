import { createHash } from 'node:crypto';
import { statSync } from 'node:fs';
import { homedir } from 'node:os';
import { join, resolve } from 'node:path';
import { DatabaseSync } from 'node:sqlite';
import {
	forceCollide,
	forceLink,
	forceManyBody,
	forceSimulation,
	forceX,
	forceY,
	type SimulationLinkDatum,
	type SimulationNodeDatum
} from 'd3-force';
import type { GraphData, GraphEdge, GraphMeta, GraphNode, IssueState } from '../graph/types';

interface RepoRow {
	id: number;
	slug: string;
	owner: string;
	name: string;
}

interface IssueRow {
	id: number;
	repo_id: number;
	number: number;
	title: string;
	body: string;
	state: string;
	url: string;
	html_url: string;
	created_at: string;
	updated_at: string;
	comments_count: number;
}

interface TagRow {
	id: number;
	slug: string;
	name: string;
	parent_id: number | null;
}

interface IssueTagRow {
	issue_id: number;
	tag_id: number;
	confidence: number;
	source: string;
}

interface LabelRow {
	issue_id: number;
	name: string;
}

interface RefRow {
	kind: string;
	source_id: number;
	source_repo_id: number;
	target_repo: string;
	target_number: number;
}

interface AreaInfo {
	rootSlug: string;
	leafSlug: string;
	rootLabel: string;
	leafLabel: string;
}

interface ClusterDraft {
	key: string;
	area: AreaInfo;
	issues: IssueRow[];
	signature: string;
}

type LayoutNode = GraphNode &
	SimulationNodeDatum & {
		seedX: number;
		seedY: number;
	};

type LayoutEdge = SimulationLinkDatum<LayoutNode> & {
	kind: GraphEdge['kind'];
};

const DB_PATH = resolve(
	process.env.TRIAGE_DB_PATH ?? join(homedir(), 'solidcraft-triage', 'analysis', 'triage.sqlite')
);

const CACHE_TTL_MS = Number(process.env.GRAPH_CACHE_TTL_MS ?? 60_000);

const ROOT_COLORS: Record<string, string> = {
	kernel: '#ff6b6b',
	ui: '#4dabf7',
	infra: '#868e96',
	doc: '#fcc419',
	sketch: '#38d9a9',
	engine: '#9775fa',
	io: '#ff922b',
	platform: '#66d9e8',
	render: '#f06595',
	assemblies: '#20c997',
	'sheet-metal': '#da77f2',
	performance: '#ffa94d',
	drawings: '#74c0fc',
	frames: '#a9e34b',
	plastic: '#ffd43b',
	cam: '#ff8787',
	'point-clouds': '#63e6be',
	plugins: '#b197fc',
	board: '#ffc078',
	unclassified: '#adb5bd'
};

const ROOT_LABELS: Record<string, string> = {
	kernel: 'Kernel',
	ui: 'UI',
	infra: 'Infra',
	doc: 'Document',
	sketch: 'Sketch',
	engine: 'Engine',
	io: 'I/O',
	platform: 'Platform',
	render: 'Render',
	assemblies: 'Assemblies',
	'sheet-metal': 'Sheet metal',
	performance: 'Performance',
	drawings: 'Drawings',
	frames: 'Frames',
	plastic: 'Plastic',
	cam: 'CAM',
	'point-clouds': 'Point clouds',
	plugins: 'Plugins',
	board: 'Board',
	unclassified: 'Unclassified'
};

const TOPIC_LABELS: Record<string, string> = {
	'kernel/sweeps': 'Sweeps',
	'kernel/blends': 'Blends',
	'kernel/shell-offset': 'Shell & offset',
	'kernel/threads-holes': 'Threads & holes',
	'kernel/patterns-split': 'Patterns & split',
	'kernel/booleans': 'Booleans',
	'kernel/surfaces': 'Surfaces',
	'kernel/topology': 'Topology',
	'kernel/tessellation': 'Tessellation',
	'ui/viewport-navigation': 'Viewport & navigation',
	'ui/ribbon-dialogs': 'Ribbon & dialogs',
	'ui/theme-prefs': 'Theme & preferences',
	'ui/browser-timeline': 'Browser & tree',
	'ui/agent-chat': 'Agent chat',
	'doc/timeline': 'Timeline',
	'doc/naming-format': 'Naming & format',
	'doc/parameters-configurations': 'Parameters & configs',
	'doc/recovery': 'Undo & recovery',
	'io/mesh-formats': 'Mesh formats',
	'io/step-iges': 'STEP & IGES',
	'io/design-file': 'Design files',
	'io/drawings-dxf': 'DXF & drawings',
	'io/urdf': 'URDF',
	'infra/tests-ci': 'Tests & CI',
	'infra/docs': 'Docs',
	'infra/release-security': 'Release & security',
	'infra/localization': 'Localization',
	'engine/commands': 'Commands',
	'engine/control-mcp-cli': 'Control / MCP / CLI',
	'engine/crash': 'Crash reports',
	'platform/desktop': 'Desktop',
	'platform/web-wasm': 'Web & WASM',
	'platform/packaging': 'Packaging',
	'platform/3d-mouse': '3D mouse',
	'render/gpu': 'GPU',
	'render/materials-lighting': 'Materials & lighting',
	'render/workspace': 'Workspace',
	'render/textures': 'Textures',
	'assemblies/components': 'Components',
	'assemblies/joints-motion': 'Joints & motion',
	'assemblies/interference-contact': 'Interference & contact',
	'sheet-metal/flanges': 'Flanges',
	'sheet-metal/unfold-flat': 'Unfold & flat',
	'sketch/dimensions': 'Dimensions',
	'sketch/3d': '3D sketches',
	'sketch/solver-constraints': 'Solver & constraints'
};

const GENERIC_TITLE = /^[A-Z][A-Z0-9 +/&.()_-]{1,56}:\s*(operation failed|unexpected result)$/i;
const ERROR_WORDS =
	/fail|error|invalid|refus|not supported|no result|unable|cannot|can't|panic|exception|timeout|exceed|missing|open|shell|self.?intersect|degenerate|zero|empty|lost|wrong/i;
const BOILERPLATE = /failed preview may display|describe the intended|describe the steps/i;

let cache: { builtAt: number; mtimeMs: number; data: GraphData } | null = null;

function asNumber(value: unknown): number {
	return typeof value === 'number' ? value : Number(value ?? 0);
}

function asString(value: unknown): string {
	return typeof value === 'string' ? value : String(value ?? '');
}

function rows<T>(statement: ReturnType<DatabaseSync['prepare']>): T[] {
	return statement.all() as T[];
}

function normalize(value: string): string {
	return value
		.normalize('NFKD')
		.toLowerCase()
		.replace(/https?:\/\/\S+/g, ' ')
		.replace(/\b[0-9a-f]{8,}\b/g, ' ')
		.replace(/\d+(?:\.\d+)?(?:e-?\d+)?/g, '#')
		.replace(/[^a-z0-9]+/g, ' ')
		.trim()
		.replace(/\s+/g, ' ');
}

function normalizeSignature(value: string): string {
	return normalize(value)
		.replace(/^[^a-z]+/, '')
		.slice(0, 150);
}

function extractSignature(body: string): string {
	if (!body) return '';
	const actual =
		body.match(/(?:##?\s*Actual behavior|Actual behavior)[\s\S]{0,1800}/i)?.[0] ?? body;
	const quoted = [...actual.matchAll(/[""]([^""]{12,320})[""]|`([^`]{12,320})`/g)].map(
		(match) => match[1] ?? match[2] ?? ''
	);
	for (const quote of quoted) {
		const normalized = normalizeSignature(quote);
		if (normalized.length < 12) continue;
		if (BOILERPLATE.test(normalized)) continue;
		if (ERROR_WORDS.test(normalized)) return normalized;
	}
	return '';
}

function isGenericTitle(title: string): boolean {
	return GENERIC_TITLE.test(title.trim());
}

function titleFromSlug(slug: string): string {
	return slug
		.split('-')
		.map((part) => (part ? part[0].toUpperCase() + part.slice(1) : part))
		.join(' ');
}

function shortAreaName(tag: TagRow | undefined, fallback: string): string {
	if (!tag) return fallback;
	const base = tag.name
		.replace(/\([^)]*\)/g, '')
		.split(/[:,]/)[0]
		.trim();
	if (base.length >= 3 && base.length <= 34) return base;
	return titleFromSlug(tag.slug.split('/').at(-1) ?? fallback);
}

function shortTitle(title: string, max = 64): string {
	const value = title.replace(/\s+/g, ' ').trim();
	if (!value) return 'Untitled issue';
	return value.length <= max ? value : `${value.slice(0, max - 1).trimEnd()}…`;
}

function hashUnit(value: string): number {
	const digest = createHash('sha1').update(value).digest();
	return digest.readUInt32BE(0) / 0xffffffff;
}

function colorFor(rootSlug: string): string {
	const known = ROOT_COLORS[rootSlug];
	if (known) return known;
	let hash = 0;
	for (const character of rootSlug) hash = (hash * 31 + character.charCodeAt(0)) >>> 0;
	const hue = hash % 360;
	return `hsl(${hue} 68% 62%)`;
}

function shade(hex: string, amount: number): string {
	if (!hex.startsWith('#')) return hex;
	const value = hex.slice(1);
	const number = Number.parseInt(value, 16);
	const red = Math.min(255, Math.max(0, ((number >> 16) & 255) + amount));
	const green = Math.min(255, Math.max(0, ((number >> 8) & 255) + amount));
	const blue = Math.min(255, Math.max(0, (number & 255) + amount));
	return `#${((red << 16) | (green << 8) | blue).toString(16).padStart(6, '0')}`;
}

function clusterLabel(issue: IssueRow, signature: string): string {
	const title = shortTitle(issue.title, 58);
	if (isGenericTitle(issue.title) && signature) {
		return `${title} — ${shortTitle(signature, 46)}`;
	}
	return title;
}

class UnionFind {
	private parent = new Map<number, number>();
	private rank = new Map<number, number>();

	constructor(values: number[]) {
		for (const value of values) {
			this.parent.set(value, value);
			this.rank.set(value, 0);
		}
	}

	find(value: number): number {
		const parent = this.parent.get(value);
		if (parent === undefined || parent === value) return value;
		const root = this.find(parent);
		this.parent.set(value, root);
		return root;
	}

	union(left: number, right: number): void {
		const leftRoot = this.find(left);
		const rightRoot = this.find(right);
		if (leftRoot === rightRoot) return;
		const leftRank = this.rank.get(leftRoot) ?? 0;
		const rightRank = this.rank.get(rightRoot) ?? 0;
		if (leftRank < rightRank) {
			this.parent.set(leftRoot, rightRoot);
		} else if (leftRank > rightRank) {
			this.parent.set(rightRoot, leftRoot);
		} else {
			this.parent.set(rightRoot, leftRoot);
			this.rank.set(leftRoot, leftRank + 1);
		}
	}
}

function loadRows(): {
	repos: RepoRow[];
	issues: IssueRow[];
	tags: TagRow[];
	issueTags: IssueTagRow[];
	labels: LabelRow[];
	refs: RefRow[];
} {
	const db = new DatabaseSync(DB_PATH, { readOnly: true });
	try {
		const repos = rows<RepoRow>(db.prepare('select id, slug, owner, name from repos order by id'));
		const issues = rows<IssueRow>(
			db.prepare(
				`select id, repo_id, number, title, body, state, url, html_url, created_at, updated_at, comments_count
				 from issues order by repo_id, number`
			)
		);
		const tags = db
			.prepare('select id, slug, name, parent_id from tags where kind = ? order by slug')
			.all('area') as unknown as TagRow[];
		const issueTags = rows<IssueTagRow>(
			db.prepare('select issue_id, tag_id, confidence, source from issue_tags order by issue_id')
		);
		const labels = rows<LabelRow>(
			db.prepare(
				`select il.issue_id, l.name
				 from issue_labels il
				 join labels l on l.id = il.label_id
				 order by il.issue_id, l.name`
			)
		);
		const refs = rows<RefRow>(
			db.prepare(
				`select r.kind,
				        coalesce(r.from_issue_id, c.issue_id) as source_id,
				        si.repo_id as source_repo_id,
				        r.target_repo,
				        r.target_number
				 from refs r
				 left join comments c on c.id = r.from_comment_id
				 left join issues si on si.id = coalesce(r.from_issue_id, c.issue_id)
				 where r.kind in ('duplicate_of', 'mention')
				   and coalesce(r.from_issue_id, c.issue_id) is not null`
			)
		);
		return { repos, issues, tags, issueTags, labels, refs };
	} finally {
		db.close();
	}
}

function buildGraph(): GraphData {
	const { repos, issues, tags, issueTags, labels, refs } = loadRows();

	const repoById = new Map(repos.map((repo) => [repo.id, repo]));
	const issueById = new Map(issues.map((issue) => [issue.id, issue]));
	const issueByRepoNumber = new Map(
		issues.map((issue) => [`${repoById.get(issue.repo_id)?.slug ?? ''}#${issue.number}`, issue])
	);
	const tagById = new Map(tags.map((tag) => [tag.id, tag]));
	const areaBySlug = new Map(tags.map((tag) => [tag.slug, tag]));
	const areasByIssue = new Map<
		number,
		Array<{ tag: TagRow; confidence: number; source: string }>
	>();
	const labelsByIssue = new Map<number, string[]>();

	for (const row of issueTags) {
		const tag = tagById.get(asNumber(row.tag_id));
		if (!tag) continue;
		const list = areasByIssue.get(asNumber(row.issue_id)) ?? [];
		list.push({
			tag,
			confidence: asNumber(row.confidence),
			source: asString(row.source)
		});
		areasByIssue.set(asNumber(row.issue_id), list);
	}
	for (const row of labels) {
		const issueId = asNumber(row.issue_id);
		const list = labelsByIssue.get(issueId) ?? [];
		if (list.length < 6) list.push(asString(row.name));
		labelsByIssue.set(issueId, list);
	}

	const sourceRank: Record<string, number> = { label: 4, rule: 3, path: 2, symbol: 1 };
	const areaForIssue = (issue: IssueRow): AreaInfo => {
		const candidates = (areasByIssue.get(issue.id) ?? []).slice();
		if (candidates.length === 0) {
			return {
				rootSlug: 'unclassified',
				leafSlug: 'general',
				rootLabel: 'Unclassified',
				leafLabel: 'General'
			};
		}
		candidates.sort((left, right) => {
			const leftDepth = left.tag.slug.split('/').length;
			const rightDepth = right.tag.slug.split('/').length;
			if (leftDepth !== rightDepth) return rightDepth - leftDepth;
			if (left.confidence !== right.confidence) return right.confidence - left.confidence;
			const sourceDiff = (sourceRank[right.source] ?? 0) - (sourceRank[left.source] ?? 0);
			if (sourceDiff !== 0) return sourceDiff;
			return left.tag.slug.localeCompare(right.tag.slug);
		});
		const tag = candidates[0].tag;
		const parts = tag.slug.split('/');
		const rootSlug = parts[0];
		const leafSlug = parts.length > 1 ? parts.slice(1).join('/') : 'general';
		const rootTag = areaBySlug.get(rootSlug);
		const leafTag = parts.length > 1 ? tag : undefined;
		return {
			rootSlug,
			leafSlug,
			rootLabel: ROOT_LABELS[rootSlug] ?? shortAreaName(rootTag, titleFromSlug(rootSlug)),
			leafLabel:
				TOPIC_LABELS[`${rootSlug}/${leafSlug}`] ??
				(leafTag ? shortAreaName(leafTag, titleFromSlug(leafSlug)) : 'General')
		};
	};

	const areaByIssue = new Map(issues.map((issue) => [issue.id, areaForIssue(issue)]));
	const signatureByIssue = new Map(issues.map((issue) => [issue.id, extractSignature(issue.body)]));
	const keyByIssue = new Map<number, string>();
	for (const issue of issues) {
		const area = areaByIssue.get(issue.id)!;
		const signature = signatureByIssue.get(issue.id) ?? '';
		const titleKey = normalize(issue.title) || `issue ${issue.number}`;
		const key =
			isGenericTitle(issue.title) && signature ? `signature:${signature}` : `title:${titleKey}`;
		keyByIssue.set(issue.id, `${area.rootSlug}/${area.leafSlug}/${key}`);
	}

	const uf = new UnionFind(issues.map((issue) => issue.id));
	const duplicatePairs: Array<{ source: number; target: number }> = [];
	const referenceWeights = new Map<string, number>();

	const resolveTarget = (
		source: IssueRow,
		targetRepo: string,
		targetNumber: number
	): IssueRow | undefined => {
		const sourceRepoSlug = repoById.get(source.repo_id)?.slug ?? '';
		const repoSlug = targetRepo || sourceRepoSlug;
		return issueByRepoNumber.get(`${repoSlug}#${targetNumber}`);
	};

	for (const ref of refs) {
		const source = issueById.get(asNumber(ref.source_id));
		if (!source) continue;
		const target = resolveTarget(source, asString(ref.target_repo), asNumber(ref.target_number));
		if (!target || target.id === source.id) continue;
		if (ref.kind === 'duplicate_of') {
			duplicatePairs.push({ source: source.id, target: target.id });
			uf.union(source.id, target.id);
		} else {
			const left = Math.min(source.id, target.id);
			const right = Math.max(source.id, target.id);
			const key = `${left}|${right}`;
			referenceWeights.set(key, (referenceWeights.get(key) ?? 0) + 1);
		}
	}

	const byKey = new Map<string, number[]>();
	for (const issue of issues) {
		const key = keyByIssue.get(issue.id)!;
		const list = byKey.get(key) ?? [];
		list.push(issue.id);
		byKey.set(key, list);
	}
	for (const ids of byKey.values()) {
		for (let index = 1; index < ids.length; index++) uf.union(ids[0], ids[index]);
	}

	const draftByRoot = new Map<string, ClusterDraft>();
	for (const issue of issues) {
		const area = areaByIssue.get(issue.id)!;
		const root = uf.find(issue.id);
		const key = String(root);
		const draft = draftByRoot.get(key);
		if (draft) {
			draft.issues.push(issue);
			continue;
		}
		draftByRoot.set(key, {
			key,
			area,
			issues: [issue],
			signature: signatureByIssue.get(issue.id) ?? ''
		});
	}

	const clusters = [...draftByRoot.values()].map((draft) => {
		draft.issues.sort(
			(left, right) =>
				right.comments_count - left.comments_count ||
				left.number - right.number ||
				left.id - right.id
		);
		const representative = draft.issues[0];
		const signature =
			isGenericTitle(representative.title) && draft.signature
				? draft.signature
				: (draft.issues.map((issue) => signatureByIssue.get(issue.id)).find(Boolean) ?? '');
		const id =
			'cluster:' +
			createHash('sha1')
				.update(
					`${draft.area.rootSlug}/${draft.area.leafSlug}/` +
						draft.issues
							.map((issue) => `${repoById.get(issue.repo_id)?.slug ?? 'unknown'}#${issue.number}`)
							.sort()
							.join(',')
				)
				.digest('hex')
				.slice(0, 14);
		const openCount = draft.issues.filter((issue) => issue.state === 'open').length;
		const reposInCluster = new Set(
			draft.issues.map((issue) => repoById.get(issue.repo_id)?.slug ?? 'unknown')
		);
		return {
			id,
			area: draft.area,
			representative,
			issues: draft.issues,
			signature,
			label: clusterLabel(representative, signature),
			openCount,
			repos: [...reposInCluster]
		};
	});

	clusters.sort(
		(left, right) =>
			left.area.rootSlug.localeCompare(right.area.rootSlug) ||
			left.area.leafSlug.localeCompare(right.area.leafSlug) ||
			left.representative.number - right.representative.number
	);

	const nodes: GraphNode[] = [];
	const edges: GraphEdge[] = [];
	const nodeById = new Map<string, GraphNode>();
	const topicId = (rootSlug: string, leafSlug: string) => `topic:${rootSlug}/${leafSlug}`;
	const rootId = (rootSlug: string) => `area:${rootSlug}`;
	const issueNodeId = (issue: IssueRow) =>
		`issue:${repoById.get(issue.repo_id)?.slug ?? 'unknown'}#${issue.number}`;

	const rootNodes = new Map<string, GraphNode>();
	const topicNodes = new Map<string, GraphNode>();
	const clusterNodes = new Map<string, GraphNode>();

	for (const cluster of clusters) {
		const rootSlug = cluster.area.rootSlug;
		const leafSlug = cluster.area.leafSlug;
		const color = colorFor(rootSlug);

		let root = rootNodes.get(rootSlug);
		if (!root) {
			root = {
				id: rootId(rootSlug),
				kind: 'area',
				label: cluster.area.rootLabel,
				area: rootSlug,
				issueCount: 0,
				openCount: 0,
				connections: 0,
				color,
				size: 0,
				x: 0,
				y: 0
			};
			rootNodes.set(rootSlug, root);
			nodes.push(root);
		}

		let topic = topicNodes.get(`${rootSlug}/${leafSlug}`);
		if (!topic) {
			topic = {
				id: topicId(rootSlug, leafSlug),
				kind: 'topic',
				label: cluster.area.leafLabel,
				parent: root.id,
				area: rootSlug,
				topic: leafSlug,
				issueCount: 0,
				openCount: 0,
				clusterCount: 0,
				connections: 0,
				color: shade(color, 12),
				size: 0,
				x: 0,
				y: 0
			};
			topicNodes.set(`${rootSlug}/${leafSlug}`, topic);
			nodes.push(topic);
			edges.push({
				id: `edge:${root.id}->${topic.id}`,
				source: root.id,
				target: topic.id,
				kind: 'area'
			});
		}

		const clusterNode: GraphNode = {
			id: cluster.id,
			kind: 'cluster',
			label: shortTitle(cluster.label, 64),
			title: cluster.label,
			parent: topic.id,
			area: rootSlug,
			topic: leafSlug,
			cluster: cluster.id,
			issueCount: cluster.issues.length,
			openCount: cluster.openCount,
			connections: 0,
			color: shade(color, 26),
			size: 0,
			x: 0,
			y: 0,
			signature: cluster.signature || undefined
		};
		clusterNodes.set(cluster.id, clusterNode);
		nodes.push(clusterNode);
		edges.push({
			id: `edge:${topic.id}->${cluster.id}`,
			source: topic.id,
			target: cluster.id,
			kind: 'topic'
		});

		for (const issue of cluster.issues) {
			const id = issueNodeId(issue);
			const repo = repoById.get(issue.repo_id);
			const issueNode: GraphNode = {
				id,
				kind: 'issue',
				label: `#${issue.number}`,
				title: issue.title,
				parent: cluster.id,
				area: rootSlug,
				topic: leafSlug,
				cluster: cluster.id,
				repo: repo?.slug,
				number: issue.number,
				state: issue.state === 'closed' ? 'closed' : 'open',
				url: issue.html_url || issue.url,
				createdAt: issue.created_at,
				updatedAt: issue.updated_at,
				issueCount: 1,
				openCount: issue.state === 'open' ? 1 : 0,
				connections: 1,
				color: shade(color, -28),
				size: 0,
				x: 0,
				y: 0,
				labels: labelsByIssue.get(issue.id) ?? []
			};
			nodes.push(issueNode);
			nodeById.set(id, issueNode);
			edges.push({
				id: `edge:${cluster.id}->${id}`,
				source: cluster.id,
				target: id,
				kind: 'issue'
			});
			topic.issueCount++;
			root.issueCount++;
			if (issue.state === 'open') {
				topic.openCount++;
				root.openCount++;
			}
		}
		topic.clusterCount = (topic.clusterCount ?? 0) + 1;
	}

	// Add duplicate/reference links after all issue nodes exist. These are what make
	// the graph useful beyond the strict hierarchy.
	const pairCounts = new Map<
		string,
		{ source: number; target: number; kind: 'duplicate' | 'reference' }
	>();
	for (const pair of duplicatePairs) {
		const left = Math.min(pair.source, pair.target);
		const right = Math.max(pair.source, pair.target);
		const key = `duplicate:${left}|${right}`;
		pairCounts.set(key, { source: left, target: right, kind: 'duplicate' });
	}
	for (const [key, weight] of referenceWeights) {
		const [left, right] = key.split('|').map(Number);
		const pairKey = `reference:${left}|${right}`;
		pairCounts.set(pairKey, { source: left, target: right, kind: 'reference' });
		void weight;
	}

	const issueNodeById = new Map<number, GraphNode>();
	for (const issue of issues) {
		const node = nodeById.get(issueNodeId(issue));
		if (node) issueNodeById.set(issue.id, node);
	}
	for (const [key, pair] of pairCounts) {
		const sourceIssue = issueById.get(pair.source);
		const targetIssue = issueById.get(pair.target);
		if (!sourceIssue || !targetIssue) continue;
		const sourceNode = issueNodeById.get(pair.source);
		const targetNode = issueNodeById.get(pair.target);
		if (!sourceNode || !targetNode) continue;
		edges.push({
			id: `edge:${sourceNode.id}->${targetNode.id}:${pair.kind}`,
			source: sourceNode.id,
			target: targetNode.id,
			kind: pair.kind,
			weight: referenceWeights.get(`${pair.source}|${pair.target}`) ?? 1
		});
		void key;
	}

	for (const node of nodes) nodeById.set(node.id, node);

	// Degree is the direct connection count used to size nodes.
	for (const edge of edges) {
		const source = nodeById.get(edge.source);
		const target = nodeById.get(edge.target);
		if (source) source.connections++;
		if (target) target.connections++;
	}

	for (const node of nodes) {
		if (node.kind === 'area') {
			node.size = 15 + Math.sqrt(node.issueCount) * 0.45;
		} else if (node.kind === 'topic') {
			node.size = 8 + Math.min(19, Math.sqrt(node.clusterCount ?? 0) * 1.15);
		} else if (node.kind === 'cluster') {
			node.size = 2.8 + Math.sqrt(node.issueCount) * 1.7;
		} else {
			node.size = 1.1 + Math.sqrt(node.connections) * 0.55;
		}
	}

	layout(nodes, edges);

	const meta: GraphMeta = {
		generatedAt: new Date().toISOString(),
		source: 'solidcraft-triage SQLite database',
		method:
			'Area taxonomy from tags; distinct issues use duplicate links plus normalized error signature or title; layout precomputed with d3-force',
		issueCount: issues.length,
		openIssueCount: issues.filter((issue) => issue.state === 'open').length,
		areaCount: rootNodes.size,
		topicCount: topicNodes.size,
		clusterCount: clusters.length,
		edgeCount: edges.length,
		repositories: repos.map((repo) => ({ slug: repo.slug, name: repo.name }))
	};

	return { meta, nodes, edges };
}

function layout(nodes: GraphNode[], edges: GraphEdge[]): void {
	const nodeById = new Map(nodes.map((node) => [node.id, node]));
	const roots = nodes.filter((node) => node.kind === 'area');
	const topics = [...nodeById.values()].filter((node) => node.kind === 'topic');
	const clusters = nodes.filter((node) => node.kind === 'cluster');
	const issues = nodes.filter((node) => node.kind === 'issue');
	const topicsByRoot = new Map<string, GraphNode[]>();
	const clustersByTopic = new Map<string, GraphNode[]>();
	const issuesByCluster = new Map<string, GraphNode[]>();

	for (const topic of topics) {
		const list = topicsByRoot.get(topic.parent ?? '') ?? [];
		list.push(topic);
		topicsByRoot.set(topic.parent ?? '', list);
	}
	for (const cluster of clusters) {
		const list = clustersByTopic.get(cluster.parent ?? '') ?? [];
		list.push(cluster);
		clustersByTopic.set(cluster.parent ?? '', list);
	}
	for (const issue of issues) {
		const list = issuesByCluster.get(issue.parent ?? '') ?? [];
		list.push(issue);
		issuesByCluster.set(issue.parent ?? '', list);
	}

	const rootWeightTotal = roots.reduce(
		(sum, root) => sum + Math.sqrt(Math.max(1, root.issueCount)),
		0
	);
	const rootRadius = 650;
	const goldenAngle = Math.PI * (3 - Math.sqrt(5));
	const layoutNodes = nodes.filter((node) => node.kind !== 'issue') as LayoutNode[];
	for (const node of layoutNodes) {
		node.seedX = 0;
		node.seedY = 0;
		node.vx = 0;
		node.vy = 0;
	}

	let angle = -Math.PI / 2;
	for (const root of roots.sort(
		(left, right) => right.issueCount - left.issueCount || left.id.localeCompare(right.id)
	)) {
		const rootSector =
			(Math.PI * 2 * Math.sqrt(Math.max(1, root.issueCount))) / Math.max(1, rootWeightTotal);
		const rootAngle = angle + rootSector / 2;
		const rootX = Math.cos(rootAngle) * rootRadius;
		const rootY = Math.sin(rootAngle) * rootRadius;
		const layoutRoot = root as LayoutNode;
		layoutRoot.seedX = rootX;
		layoutRoot.seedY = rootY;
		layoutRoot.fx = rootX;
		layoutRoot.fy = rootY;

		const children = (topicsByRoot.get(root.id) ?? []).sort(
			(left, right) => right.issueCount - left.issueCount || left.id.localeCompare(right.id)
		);
		const childTotal = Math.max(
			1,
			children.reduce((sum, child) => sum + child.issueCount, 0)
		);
		let childAngle = angle;
		for (const topic of children) {
			const topicSector = rootSector * (Math.max(1, topic.issueCount) / childTotal);
			const topicAngle = childAngle + topicSector / 2;
			const topicClusters = clustersByTopic.get(topic.id) ?? [];
			const topicRadius = 900 + Math.min(1000, Math.sqrt(topicClusters.length) * 18);
			const layoutTopic = topic as LayoutNode;
			layoutTopic.seedX = rootX + Math.cos(topicAngle) * topicRadius;
			layoutTopic.seedY = rootY + Math.sin(topicAngle) * topicRadius;
			layoutTopic.fx = layoutTopic.seedX;
			layoutTopic.fy = layoutTopic.seedY;

			const topicClusterList = topicClusters.sort(
				(left, right) => right.issueCount - left.issueCount || left.id.localeCompare(right.id)
			);
			const clusterCloud = 25 + Math.sqrt(topicClusterList.length) * 16;
			topicClusterList.forEach((cluster, index) => {
				const clusterAngle = index * goldenAngle + hashUnit(cluster.id) * 0.35;
				const clusterDistance =
					clusterCloud * Math.sqrt((index + 0.5) / Math.max(1, topicClusterList.length));
				const clusterX = layoutTopic.seedX + Math.cos(clusterAngle) * clusterDistance;
				const clusterY = layoutTopic.seedY + Math.sin(clusterAngle) * clusterDistance;
				const layoutCluster = cluster as LayoutNode;
				layoutCluster.seedX = clusterX;
				layoutCluster.seedY = clusterY;

				const childIssues = (issuesByCluster.get(cluster.id) ?? []).sort(
					(left, right) => left.number! - right.number!
				);
				const issueCloud = 4 + Math.sqrt(childIssues.length) * 2.4;
				childIssues.forEach((issue, issueIndex) => {
					const issueAngle = issueIndex * goldenAngle + hashUnit(issue.id) * 0.5;
					const issueDistance =
						issueCloud * Math.sqrt((issueIndex + 0.5) / Math.max(1, childIssues.length));
					const layoutIssue = issue as LayoutNode;
					layoutIssue.seedX = clusterX + Math.cos(issueAngle) * issueDistance;
					layoutIssue.seedY = clusterY + Math.sin(issueAngle) * issueDistance;
				});
			});
			childAngle += topicSector;
		}
		angle += rootSector;
	}

	const layoutEdges: LayoutEdge[] = edges
		.filter((edge) => edge.kind === 'area' || edge.kind === 'topic')
		.map((edge) => ({
			source: edge.source,
			target: edge.target,
			kind: edge.kind
		}));

	const simulation = forceSimulation<LayoutNode>(layoutNodes)
		.force(
			'link',
			forceLink<LayoutNode, LayoutEdge>(layoutEdges)
				.id((node) => node.id)
				.distance((edge) => {
					if (edge.kind === 'issue') return 9;
					if (edge.kind === 'topic') return 54;
					if (edge.kind === 'area') return 180;
					return 120;
				})
				.strength((edge) => (edge.kind === 'duplicate' ? 0.25 : 0.7))
		)
		.force(
			'charge',
			forceManyBody<LayoutNode>()
				.strength((node) => {
					if (node.kind === 'area') return -700;
					if (node.kind === 'topic') return -280;
					if (node.kind === 'cluster') return -12;
					return -0.5;
				})
				.theta(0.9)
		)
		.force(
			'collide',
			forceCollide<LayoutNode>()
				.radius((node) => (node.kind === 'issue' ? 0.9 : node.size + 2.5))
				.strength(0.6)
				.iterations(1)
		)
		.force(
			'x',
			forceX<LayoutNode>((node) => node.seedX).strength((node) => {
				if (node.kind === 'area') return 0.35;
				if (node.kind === 'topic') return 0.18;
				if (node.kind === 'cluster') return 0.04;
				return 0.008;
			})
		)
		.force(
			'y',
			forceY<LayoutNode>((node) => node.seedY).strength((node) => {
				if (node.kind === 'area') return 0.35;
				if (node.kind === 'topic') return 0.18;
				if (node.kind === 'cluster') return 0.04;
				return 0.008;
			})
		)
		.stop();

	for (let index = 0; index < 70; index++) simulation.tick();

	for (const node of layoutNodes) {
		node.x = Number((node.x ?? node.seedX).toFixed(2));
		node.y = Number((node.y ?? node.seedY).toFixed(2));
	}

	// Leaf issues are laid out around their final cluster position after the
	// force pass. Keeping them out of the simulation makes the first build much
	// faster and prevents thousands of leaves from distorting the topic layout.
	for (const cluster of clusters) {
		const childIssues = (issuesByCluster.get(cluster.id) ?? []).sort(
			(left, right) => (left.number ?? 0) - (right.number ?? 0)
		);
		const issueCloud = 3.5 + Math.sqrt(childIssues.length) * 2.1;
		childIssues.forEach((issue, index) => {
			const issueAngle = index * goldenAngle + hashUnit(issue.id) * 0.6;
			const issueDistance = issueCloud * Math.sqrt((index + 0.5) / Math.max(1, childIssues.length));
			issue.x = Number((cluster.x + Math.cos(issueAngle) * issueDistance).toFixed(2));
			issue.y = Number((cluster.y + Math.sin(issueAngle) * issueDistance).toFixed(2));
		});
	}
}

export function getGraph(): GraphData {
	const stat = statSync(DB_PATH);
	const now = Date.now();
	if (cache && now - cache.builtAt < CACHE_TTL_MS && cache.mtimeMs === stat.mtimeMs) {
		return cache.data;
	}
	const data = buildGraph();
	cache = { builtAt: now, mtimeMs: stat.mtimeMs, data };
	return data;
}

export function getGraphMeta(): GraphMeta {
	return getGraph().meta;
}

export function filterGraph(
	data: GraphData,
	filters: { repo?: string; state?: IssueState; area?: string; topic?: string }
): GraphData {
	const { repo, state, area, topic } = filters;
	if (!repo && !state && !area && !topic) return data;

	const keep = new Set<string>();
	for (const node of data.nodes) {
		if (node.kind === 'issue') {
			if (repo && node.repo !== repo) continue;
			if (state && node.state !== state) continue;
			if (area && node.area !== area) continue;
			if (topic && node.topic !== topic) continue;
			keep.add(node.id);
		} else if (node.kind === 'area') {
			if (area && node.area === area) keep.add(node.id);
		} else if (node.kind === 'topic') {
			if (area && node.area !== area) continue;
			if (topic && node.topic !== topic) continue;
			if (area || topic) keep.add(node.id);
		} else if (node.kind === 'cluster') {
			if (area && node.area !== area) continue;
			if (topic && node.topic !== topic) continue;
			if (area || topic) keep.add(node.id);
		}
	}

	// Pull in every ancestor of a matching issue.
	let changed = true;
	while (changed) {
		changed = false;
		for (const node of data.nodes) {
			if (!keep.has(node.id) || !node.parent) continue;
			if (!keep.has(node.parent)) {
				keep.add(node.parent);
				changed = true;
			}
		}
	}

	// An explicit area/topic selection means the whole subtree, but leaf issues
	// still obey the repository/state filters.
	if (area || topic) {
		changed = true;
		while (changed) {
			changed = false;
			for (const node of data.nodes) {
				if (node.parent && keep.has(node.parent) && !keep.has(node.id)) {
					if (node.kind === 'issue') {
						if (repo && node.repo !== repo) continue;
						if (state && node.state !== state) continue;
					}
					keep.add(node.id);
					changed = true;
				}
			}
		}
	}

	// Remove clusters and topics that lost every issue to the repo/state filter.
	const clustersWithIssues = new Set<string>();
	for (const node of data.nodes) {
		if (node.kind === 'issue' && keep.has(node.id) && node.cluster) {
			clustersWithIssues.add(node.cluster);
		}
	}
	const topicsWithClusters = new Set<string>();
	for (const node of data.nodes) {
		if (node.kind === 'cluster' && keep.has(node.id) && clustersWithIssues.has(node.id)) {
			if (node.parent) topicsWithClusters.add(node.parent);
		} else if (node.kind === 'cluster') {
			keep.delete(node.id);
		}
	}
	const areasWithTopics = new Set<string>();
	for (const node of data.nodes) {
		if (node.kind === 'topic' && keep.has(node.id) && topicsWithClusters.has(node.id)) {
			if (node.parent) areasWithTopics.add(node.parent);
		} else if (node.kind === 'topic') {
			keep.delete(node.id);
		}
	}
	for (const node of data.nodes) {
		if (node.kind === 'area' && keep.has(node.id) && !areasWithTopics.has(node.id)) {
			keep.delete(node.id);
		}
	}

	return {
		...data,
		nodes: data.nodes.filter((node) => keep.has(node.id)),
		edges: data.edges.filter((edge) => keep.has(edge.source) && keep.has(edge.target))
	};
}
