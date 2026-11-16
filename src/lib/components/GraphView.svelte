<script lang="ts">
	import { onMount } from 'svelte';
	import type GraphType from 'graphology';
	import type SigmaType from 'sigma';
	import type { NodeDisplayData } from 'sigma/types';
	import type { GraphData, GraphNode } from '../graph/types';

	interface Props {
		data: GraphData;
		selectedId?: string | null;
		query?: string;
		showIssues?: boolean;
		showSingletons?: boolean;
		showReferences?: boolean;
		repoFilter?: string;
		stateFilter?: string;
		areaFilter?: string;
		topicFilter?: string;
		onselect: (node: GraphNode | null) => void;
	}

	let {
		data,
		selectedId = null,
		query = '',
		showIssues = false,
		showSingletons = false,
		showReferences = false,
		repoFilter = 'all',
		stateFilter = 'all',
		areaFilter = 'all',
		topicFilter = 'all',
		onselect
	}: Props = $props();

	let container: HTMLDivElement;
	let mounted = $state(false);
	let renderer: SigmaType | null = null;
	let graph: GraphType | null = null;
	let hoveredId = $state<string | null>(null);
	let tooltip = $state<{ x: number; y: number; title: string; detail: string } | null>(null);
	let selectedNeighbors = new Set<string>();

	export function focusNode(id: string): void {
		if (!renderer || !graph || !graph.hasNode(id)) return;
		const attrs = graph.getNodeAttributes(id) as GraphNode;
		const display = renderer.getNodeDisplayData(id);
		if (!display) return;
		const camera = renderer.getCamera();
		const currentRatio = camera.getState().ratio;
		const factor = attrs.kind === 'issue' ? 0.45 : attrs.kind === 'cluster' ? 0.65 : 0.85;
		void camera.animate(
			{
				x: display.x,
				y: display.y,
				ratio: Math.max(0.02, currentRatio * factor)
			},
			{ duration: 550 }
		);
	}

	export function fit(): void {
		if (renderer) void renderer.getCamera().reset({ duration: 450 });
	}

	onMount(() => {
		mounted = true;
		const observer = new ResizeObserver(() => renderer?.resize());
		observer.observe(container);
		return () => observer.disconnect();
	});

	function matchesQuery(node: GraphNode): boolean {
		const value = query.trim().toLowerCase();
		if (!value) return false;
		const haystack = [node.title, node.label, node.repo, node.number?.toString(), node.signature]
			.filter(Boolean)
			.join(' ')
			.toLowerCase();
		return haystack.includes(value);
	}

	function isNodeHidden(node: GraphNode): boolean {
		if (node.id === selectedId) return false;
		if (areaFilter !== 'all' && node.area !== areaFilter) return true;
		if (topicFilter !== 'all' && node.topic !== topicFilter && node.kind !== 'area') return true;
		if (node.kind === 'cluster') {
			if (node.pending) return false;
			if (!showIssues && !showSingletons && node.issueCount === 1) return true;
			return false;
		}
		if (node.kind !== 'issue') return false;
		if (!showIssues) return true;
		if (repoFilter !== 'all' && node.repo !== repoFilter) return true;
		if (stateFilter !== 'all' && node.state !== stateFilter) return true;
		return false;
	}

	$effect(() => {
		if (!mounted || !container || !data) return;

		let disposed = false;
		let builtRenderer: SigmaType | null = null;
		let builtGraph: GraphType | null = null;

		void (async () => {
			const [{ default: Graph }, { default: Sigma }] = await Promise.all([
				import('graphology'),
				import('sigma')
			]);
			if (disposed) return;

			const nextGraph = new Graph({ multi: true, type: 'directed' });
			builtGraph = nextGraph;
			for (const node of data.nodes) {
				if (nextGraph.hasNode(node.id)) continue;
				nextGraph.addNode(node.id, {
					...node
				});
			}
			for (const edge of data.edges) {
				if (!nextGraph.hasNode(edge.source) || !nextGraph.hasNode(edge.target)) continue;
				if (nextGraph.hasEdge(edge.id)) continue;
				nextGraph.addEdgeWithKey(edge.id, edge.source, edge.target, {
					kind: edge.kind,
					weight: edge.weight ?? 1
				});
			}

			graph = nextGraph;
			const nextRenderer = new Sigma(nextGraph, container, {
				settings: {
					renderLabels: true,
					renderEdgeLabels: false,
					enableEdgeEvents: false,
					labelDensity: 0.06,
					labelGridCellSize: 140,
					labelRenderedSizeThreshold: 8,
					minCameraRatio: 0.015,
					maxCameraRatio: 9,
					stagePadding: 24,
					autoRescale: true,
					itemSizesReference: 'positions'
				},
				nodeReducer: (key, _display, attrs) => {
					const node = attrs as unknown as GraphNode;
					const result: Partial<NodeDisplayData> = {
						color: node.color,
						size: node.size,
						labelColor: '#e6edf3',
						labelBackgroundColor: 'rgba(11, 15, 20, 0.82)',
						labelBackgroundPadding: 2,
						labelSize:
							node.kind === 'area'
								? 13
								: node.kind === 'topic'
									? 11
									: node.kind === 'cluster'
										? 9
										: 8,
						labelVisibility:
							node.kind === 'issue'
								? 'hidden'
								: node.kind === 'cluster'
									? node.issueCount >= 30
										? 'auto'
										: 'hidden'
									: node.kind === 'topic'
										? node.issueCount >= 120
											? 'auto'
											: 'hidden'
										: 'visible'
					};

					if (isNodeHidden(node)) {
						result.visibility = 'hidden';
						result.labelVisibility = 'hidden';
						return result;
					}
					if (node.pending) {
						result.color = '#fbbf24';
						result.labelVisibility = 'visible';
					}

					const isSelected = key === selectedId;
					const isNeighbor = selectedNeighbors.has(key);
					const isMatch = matchesQuery(node);

					if (selectedId && !isSelected && !isNeighbor) {
						result.color = '#252d37';
						result.labelVisibility = 'hidden';
					}
					if (isSelected) {
						result.color = '#ffffff';
						result.size = Math.max(node.size * 1.35, 7);
						result.labelVisibility = 'visible';
						result.zIndex = 4;
					}
					if (isNeighbor) {
						result.color = node.kind === 'issue' ? '#d7e3f4' : node.color;
						result.labelVisibility = node.kind === 'issue' ? 'auto' : 'visible';
						result.zIndex = 2;
					}
					if (isMatch) {
						result.color = '#ffffff';
						result.size = Math.max(node.size * 1.3, 5);
						result.labelVisibility = 'visible';
						result.zIndex = 3;
					}
					if (key === hoveredId) {
						result.color = '#ffffff';
						result.size = Math.max(node.size * 1.2, 6);
						result.labelVisibility = 'visible';
						result.zIndex = 5;
					}
					if (node.kind === 'area' || node.kind === 'topic') {
						const horizontal = Math.abs(node.x) >= Math.abs(node.y);
						result.labelPosition = horizontal
							? node.x >= 0
								? 'right'
								: 'left'
							: node.y >= 0
								? 'below'
								: 'above';
					}
					return result;
				},
				edgeReducer: (_key, _display, attrs) => {
					const kind = attrs.kind as string | undefined;
					const sourceNode = nextGraph.getNodeAttributes(nextGraph.source(_key)) as GraphNode;
					const targetNode = nextGraph.getNodeAttributes(nextGraph.target(_key)) as GraphNode;
					if (isNodeHidden(sourceNode) || isNodeHidden(targetNode)) {
						return { visibility: 'hidden' };
					}
					if (kind === 'reference' && !showReferences) {
						return { visibility: 'hidden' };
					}
					if (selectedId) {
						const source = nextGraph.source(_key);
						const target = nextGraph.target(_key);
						if (source === selectedId || target === selectedId) {
							return { color: '#7aa2f7', size: 1.1 };
						}
						return { color: '#1d242c', size: 0.3 };
					}
					if (kind === 'reference') return { color: '#334155', size: 0.35 };
					if (kind === 'duplicate') return { color: '#9d7cff', size: 0.7 };
					if (kind === 'issue') return { color: '#2b3540', size: 0.45 };
					return { color: '#33404c', size: 0.55 };
				}
			});

			nextRenderer.on('enterNode', ({ node, event }) => {
				hoveredId = node;
				const attrs = nextGraph.getNodeAttributes(node) as GraphNode;
				const detail =
					attrs.kind === 'issue'
						? `${attrs.repo ?? 'issue'} · ${attrs.state ?? ''} · ${attrs.connections} links`
						: `${attrs.issueCount.toLocaleString()} issues · ${attrs.connections} links`;
				const original = event.original;
				tooltip = {
					x: 'clientX' in original ? original.clientX : 0,
					y: 'clientY' in original ? original.clientY : 0,
					title: attrs.title ?? attrs.label,
					detail
				};
				nextRenderer.refresh();
			});
			nextRenderer.on('leaveNode', () => {
				hoveredId = null;
				tooltip = null;
				nextRenderer.refresh();
			});
			nextRenderer.on('clickNode', ({ node }) => {
				onselect(nextGraph.getNodeAttributes(node) as GraphNode);
			});
			nextRenderer.on('clickStage', () => {
				onselect(null);
			});

			renderer = nextRenderer;
			builtRenderer = nextRenderer;
			requestAnimationFrame(() => {
				nextRenderer.getCamera().reset({ duration: 0 });
			});
		})();

		return () => {
			disposed = true;
			builtRenderer?.kill();
			if (renderer === builtRenderer) renderer = null;
			if (graph === builtGraph) graph = null;
		};
	});

	$effect(() => {
		const current = {
			selectedId,
			query,
			showIssues,
			showSingletons,
			showReferences,
			repoFilter,
			stateFilter,
			areaFilter,
			topicFilter
		};
		if (!graph || !current.selectedId) {
			selectedNeighbors = new Set();
			renderer?.refresh();
			return;
		}
		selectedNeighbors = new Set(graph.neighbors(current.selectedId));
		renderer?.refresh();
	});
</script>

<div class="graph-stage" bind:this={container}>
	{#if tooltip}
		<div class="tooltip" style:left={`${tooltip.x + 14}px`} style:top={`${tooltip.y + 14}px`}>
			<strong>{tooltip.title}</strong>
			<span>{tooltip.detail}</span>
		</div>
	{/if}
	<div class="hint">scroll to zoom · drag to pan · click a node</div>
</div>

<style>
	.graph-stage {
		position: relative;
		width: 100%;
		height: 100%;
		background:
			radial-gradient(circle at 50% 45%, rgba(56, 189, 248, 0.06), transparent 36%), #0b0f14;
		overflow: hidden;
	}

	.graph-stage :global(canvas) {
		outline: none;
	}

	.tooltip {
		position: fixed;
		z-index: 20;
		max-width: min(380px, calc(100vw - 2rem));
		padding: 0.55rem 0.7rem;
		border: 1px solid #334155;
		border-radius: 0.45rem;
		background: rgba(9, 13, 18, 0.94);
		box-shadow: 0 10px 30px rgba(0, 0, 0, 0.35);
		pointer-events: none;
		display: grid;
		gap: 0.18rem;
		font-size: 0.76rem;
		line-height: 1.35;
	}

	.tooltip strong {
		color: #e6edf3;
		font-weight: 600;
	}

	.tooltip span {
		color: #8b98a5;
	}

	.hint {
		position: absolute;
		bottom: 0.8rem;
		right: 0.9rem;
		color: #64748b;
		font-size: 0.68rem;
		letter-spacing: 0.02em;
		pointer-events: none;
	}
</style>
