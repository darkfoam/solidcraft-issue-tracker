export type NodeKind = 'area' | 'topic' | 'cluster' | 'issue';

export type IssueState = 'open' | 'closed';

export interface GraphNode {
	id: string;
	kind: NodeKind;
	label: string;
	title?: string;
	parent?: string;
	area?: string;
	topic?: string;
	cluster?: string;
	repo?: string;
	number?: number;
	state?: IssueState;
	url?: string;
	createdAt?: string;
	updatedAt?: string;
	issueCount: number;
	openCount: number;
	clusterCount?: number;
	connections: number;
	color: string;
	size: number;
	x: number;
	y: number;
	signature?: string;
	labels?: string[];
	queueStatus?: string;
	priority?: string;
	layer?: string;
	assignee?: string;
	note?: string;
	pending?: boolean;
	dbId?: number;
}

export interface GraphEdge {
	id: string;
	source: string;
	target: string;
	kind: 'area' | 'topic' | 'issue' | 'duplicate' | 'reference';
	weight?: number;
}

export interface GraphMeta {
	generatedAt: string;
	source: string;
	method: string;
	issueCount: number;
	openIssueCount: number;
	areaCount: number;
	topicCount: number;
	clusterCount: number;
	unclusteredIssueCount: number;
	edgeCount: number;
	repositories: Array<{ slug: string; name: string }>;
}

export interface GraphData {
	meta: GraphMeta;
	nodes: GraphNode[];
	edges: GraphEdge[];
}

export interface GraphFilters {
	repo?: string;
	state?: IssueState;
	area?: string;
	topic?: string;
	query?: string;
}
