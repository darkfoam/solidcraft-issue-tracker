<script lang="ts">
	import { onMount } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { resolve } from '$app/paths';
	import GraphView from '../lib/components/GraphView.svelte';
	import type { GraphData, GraphNode, IssueState } from '../lib/graph/types';

	interface GraphViewHandle {
		focusNode: (id: string) => void;
		fit: () => void;
	}

	let graphData = $state<GraphData | null>(null);
	let loading = $state(true);
	let error = $state('');
	let selected = $state<GraphNode | null>(null);
	let query = $state('');
	let repoFilter = $state('all');
	let stateFilter = $state<'all' | IssueState>('all');
	let areaFilter = $state('all');
	let topicFilter = $state('all');
	let showIssues = $state(false);
	let showSingletons = $state(false);
	let showReferences = $state(false);
	let graphView = $state<GraphViewHandle>();

	const nodeById = $derived.by(() => {
		const map = new SvelteMap<string, GraphNode>();
		for (const node of graphData?.nodes ?? []) map.set(node.id, node);
		return map;
	});

	const areaOptions = $derived.by(() =>
		(graphData?.nodes ?? [])
			.filter((node) => node.kind === 'area')
			.sort((left, right) => right.issueCount - left.issueCount)
	);

	const topicOptions = $derived.by(() =>
		(graphData?.nodes ?? [])
			.filter((node) => node.kind === 'topic' && (areaFilter === 'all' || node.area === areaFilter))
			.sort((left, right) => right.issueCount - left.issueCount)
	);

	const searchResults = $derived.by(() => {
		const value = query.trim().toLowerCase();
		if (value.length < 2 || !graphData) return [];
		return graphData.nodes
			.filter((node) => {
				const haystack = [
					node.title,
					node.label,
					node.repo,
					node.number?.toString(),
					node.signature
				]
					.filter(Boolean)
					.join(' ')
					.toLowerCase();
				return haystack.includes(value);
			})
			.sort((left, right) => {
				const kindOrder = { area: 0, topic: 1, cluster: 2, issue: 3 };
				const kindDiff = kindOrder[left.kind] - kindOrder[right.kind];
				if (kindDiff !== 0) return kindDiff;
				return right.issueCount - left.issueCount;
			})
			.slice(0, 18);
	});

	const selectedChildren = $derived.by(() => {
		const current = selected;
		if (!current || !graphData) return [];
		if (current.kind === 'issue') return [];
		const children = graphData.nodes.filter((node) => node.parent === current.id);
		if (current.kind === 'area') {
			return children
				.filter((node) => node.kind === 'topic')
				.sort((left, right) => right.issueCount - left.issueCount);
		}
		if (current.kind === 'topic') {
			return children
				.filter((node) => node.kind === 'cluster')
				.sort((left, right) => right.issueCount - left.issueCount)
				.slice(0, 60);
		}
		return children
			.filter((node) => node.kind === 'issue')
			.sort((left, right) => {
				if (left.state !== right.state) return left.state === 'open' ? -1 : 1;
				return (right.number ?? 0) - (left.number ?? 0);
			})
			.slice(0, 100);
	});

	const selectedPath = $derived.by(() => {
		if (!selected) return [];
		const path: GraphNode[] = [];
		let current: GraphNode | undefined = selected;
		while (current) {
			path.unshift(current);
			current = current.parent ? nodeById.get(current.parent) : undefined;
		}
		return path;
	});

	const filteredCount = $derived.by(() => {
		if (!graphData) return 0;
		return graphData.nodes.filter((node) => {
			if (node.kind !== 'issue') return false;
			if (repoFilter !== 'all' && node.repo !== repoFilter) return false;
			if (stateFilter !== 'all' && node.state !== stateFilter) return false;
			if (areaFilter !== 'all' && node.area !== areaFilter) return false;
			if (topicFilter !== 'all' && node.topic !== topicFilter) return false;
			return true;
		}).length;
	});

	const hasFilters = $derived(
		query.trim() !== '' ||
			repoFilter !== 'all' ||
			stateFilter !== 'all' ||
			areaFilter !== 'all' ||
			topicFilter !== 'all' ||
			showReferences
	);

	onMount(async () => {
		try {
			const response = await fetch('/api/graph');
			if (!response.ok) {
				const problem = (await response.json().catch(() => null)) as { error?: string } | null;
				throw new Error(problem?.error ?? `Graph API returned ${response.status}`);
			}
			graphData = (await response.json()) as GraphData;
		} catch (problem) {
			error = problem instanceof Error ? problem.message : 'Unable to load the graph';
		} finally {
			loading = false;
		}
	});

	function selectNode(node: GraphNode | null, focus = false): void {
		selected = node;
		if (node && focus) graphView?.focusNode(node.id);
	}

	function selectSearchResult(node: GraphNode): void {
		selectNode(node, true);
	}

	function focusSelected(): void {
		if (selected) graphView?.focusNode(selected.id);
	}

	function resetFilters(): void {
		query = '';
		repoFilter = 'all';
		stateFilter = 'all';
		areaFilter = 'all';
		topicFilter = 'all';
		showIssues = false;
		showSingletons = false;
		showReferences = false;
	}

	function onAreaChange(): void {
		if (topicFilter !== 'all') {
			const topic = nodeById.get(topicFilter);
			if (!topic || (areaFilter !== 'all' && topic.area !== areaFilter)) topicFilter = 'all';
		}
	}

	function kindLabel(kind: GraphNode['kind']): string {
		if (kind === 'area') return 'topic';
		if (kind === 'topic') return 'subtopic';
		if (kind === 'cluster') return 'distinct issue';
		return 'GitHub issue';
	}

	function short(value: string | undefined, length = 90): string {
		if (!value) return '';
		return value.length <= length ? value : `${value.slice(0, length - 1)}…`;
	}
</script>

<svelte:head>
	<title>SolidCraft Issue Graph</title>
	<meta
		name="description"
		content="A hierarchical graph of SolidCraft and solidcore issues, grouped by area and distinct failure."
	/>
</svelte:head>

<div class="app-shell">
	<header class="topbar">
		<div class="brand">
			<div class="brand-mark" aria-hidden="true">
				<span></span><span></span><span></span><span></span>
			</div>
			<div>
				<h1>SolidCraft Issue Graph</h1>
				<p>Issues, failure clusters and cross-references from the triage database</p>
			</div>
		</div>

		<div class="stats" aria-label="Graph totals">
			<div class="stat">
				<strong>{graphData?.meta.issueCount.toLocaleString() ?? '—'}</strong>
				<span>issues</span>
			</div>
			<div class="stat">
				<strong>{graphData?.meta.openIssueCount.toLocaleString() ?? '—'}</strong>
				<span>open</span>
			</div>
			<div class="stat">
				<strong>{graphData?.meta.clusterCount.toLocaleString() ?? '—'}</strong>
				<span>distinct</span>
			</div>
			<div class="stat">
				<strong>{graphData?.meta.areaCount.toLocaleString() ?? '—'}</strong>
				<span>topics</span>
			</div>
		</div>

		<div class="top-actions">
			<button class="ghost-button" onclick={() => graphView?.fit()}>Fit graph</button>
			<a class="ghost-button" href={resolve('/api/graph')} target="_blank" rel="noreferrer"
				>JSON API</a
			>
		</div>
	</header>

	<main class="workspace">
		<aside class="panel controls">
			<div class="panel-title">
				<h2>Explore</h2>
				{#if graphData}
					<span>{filteredCount.toLocaleString()} matching issues</span>
				{/if}
			</div>

			<label class="search-field">
				<span>Search</span>
				<input
					type="search"
					placeholder="Issue number, title, area…"
					bind:value={query}
					onkeydown={(event) => {
						if (event.key === 'Enter' && searchResults[0]) selectSearchResult(searchResults[0]);
					}}
				/>
			</label>

			{#if searchResults.length > 0}
				<div class="search-results">
					{#each searchResults as result (result.id)}
						<button class="search-result" onclick={() => selectSearchResult(result)}>
							<span class="result-kind">{kindLabel(result.kind)}</span>
							<span class="result-label">{result.label}</span>
							{#if result.title && result.title !== result.label}
								<span class="result-title">{short(result.title, 62)}</span>
							{/if}
						</button>
					{/each}
				</div>
			{:else if query.trim().length >= 2}
				<p class="empty-note">No matching nodes.</p>
			{/if}

			<div class="filter-grid">
				<label>
					<span>Repository</span>
					<select bind:value={repoFilter}>
						<option value="all">All repositories</option>
						<option value="tangency-labs/solidcraft">solidcraft</option>
						<option value="tangency-labs/solidcore">solidcore</option>
					</select>
				</label>
				<label>
					<span>State</span>
					<select bind:value={stateFilter}>
						<option value="all">All states</option>
						<option value="open">Open</option>
						<option value="closed">Closed</option>
					</select>
				</label>
				<label>
					<span>Topic</span>
					<select bind:value={areaFilter} onchange={onAreaChange}>
						<option value="all">All topics</option>
						{#each areaOptions as area (area.id)}
							<option value={area.area}>{area.label}</option>
						{/each}
					</select>
				</label>
				<label>
					<span>Subtopic</span>
					<select bind:value={topicFilter}>
						<option value="all">All subtopics</option>
						{#each topicOptions as topic (topic.id)}
							<option value={topic.topic}>{topic.label}</option>
						{/each}
					</select>
				</label>
			</div>

			<div class="toggle-list">
				<label class="toggle">
					<input type="checkbox" bind:checked={showIssues} />
					<span>Show GitHub issue nodes</span>
				</label>
				<label class="toggle">
					<input type="checkbox" bind:checked={showSingletons} />
					<span>Show single-report groups</span>
				</label>
				<label class="toggle">
					<input type="checkbox" bind:checked={showReferences} />
					<span>Show cross-reference links</span>
				</label>
			</div>

			{#if hasFilters}
				<button class="text-button" onclick={resetFilters}>Reset filters</button>
			{/if}

			<div class="legend">
				<h3>Legend</h3>
				<div class="legend-row"><span class="legend-dot area"></span> Topic</div>
				<div class="legend-row"><span class="legend-dot topic"></span> Subtopic</div>
				<div class="legend-row"><span class="legend-dot cluster"></span> Distinct issue</div>
				<div class="legend-row"><span class="legend-dot issue"></span> GitHub issue</div>
				<div class="legend-row"><span class="legend-line"></span> Cross-reference</div>
			</div>

			<div class="method-note">
				<h3>How grouping works</h3>
				<p>
					Distinct issues are inferred from duplicate links plus normalized error signatures or
					titles. A group is a starting point for triage, not a hand-curated decision.
				</p>
			</div>
		</aside>

		<section class="graph-panel" aria-busy={loading}>
			{#if graphData}
				<GraphView
					bind:this={graphView}
					data={graphData}
					selectedId={selected?.id ?? null}
					{query}
					{showIssues}
					{showSingletons}
					{showReferences}
					{repoFilter}
					{stateFilter}
					{areaFilter}
					{topicFilter}
					onselect={(node) => selectNode(node)}
				/>
			{:else if loading}
				<div class="loading-state">
					<div class="spinner" aria-hidden="true"></div>
					<h2>Building the graph</h2>
					<p>Reading the SQLite triage database and laying out the issue hierarchy.</p>
				</div>
			{:else}
				<div class="error-state">
					<h2>Could not load the graph</h2>
					<p>{error}</p>
					<pre>TRIAGE_DB_PATH=/path/to/triage.sqlite npm run dev</pre>
					<p class="muted">
						The default path is <code>~/solidcraft-triage/analysis/triage.sqlite</code>.
					</p>
				</div>
			{/if}
		</section>

		<aside class="panel details">
			{#if selected}
				<div class="detail-head">
					<div class="detail-kicker">{kindLabel(selected.kind)}</div>
					<h2>{selected.title ?? selected.label}</h2>
					{#if selected.signature}
						<p class="signature">{selected.signature}</p>
					{/if}
					<div class="detail-actions">
						<button class="small-button" onclick={focusSelected}>Focus in graph</button>
						{#if selected.url}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
							<a class="small-button" href={selected.url} target="_blank" rel="noreferrer">
								Open on GitHub
							</a>
						{/if}
					</div>
				</div>

				<div class="metric-grid">
					{#if selected.kind !== 'issue'}
						<div><strong>{selected.issueCount.toLocaleString()}</strong><span>issues</span></div>
						<div><strong>{selected.openCount.toLocaleString()}</strong><span>open</span></div>
					{/if}
					{#if selected.kind === 'issue'}
						<div><strong>{selected.state}</strong><span>state</span></div>
						<div><strong>{selected.connections}</strong><span>connections</span></div>
					{/if}
					{#if selected.clusterCount !== undefined}
						<div>
							<strong>{selected.clusterCount.toLocaleString()}</strong><span>distinct issues</span>
						</div>
					{/if}
				</div>

				<nav class="breadcrumb" aria-label="Selected node path">
					{#each selectedPath as crumb, index (crumb.id)}
						{#if index > 0}<span>/</span>{/if}
						<button onclick={() => selectNode(crumb, true)}>{crumb.label}</button>
					{/each}
				</nav>

				{#if selected.kind === 'issue'}
					<div class="detail-body">
						<div class="issue-meta">
							<span>{selected.repo}</span>
							<span>#{selected.number}</span>
							<span
								class:open={selected.state === 'open'}
								class:closed={selected.state === 'closed'}
							>
								{selected.state}
							</span>
						</div>
						{#if selected.labels?.length}
							<div class="labels">
								{#each selected.labels as label (label)}
									<span>{label}</span>
								{/each}
							</div>
						{/if}
						<dl>
							<dt>Created</dt>
							<dd>{selected.createdAt?.slice(0, 10) ?? '—'}</dd>
							<dt>Updated</dt>
							<dd>{selected.updatedAt?.slice(0, 10) ?? '—'}</dd>
							<dt>Cluster</dt>
							<dd>
								<button
									class="link-button"
									onclick={() => {
										const cluster = selected?.cluster ? nodeById.get(selected.cluster) : undefined;
										if (cluster) selectNode(cluster, true);
									}}>{nodeById.get(selected.cluster ?? '')?.label ?? '—'}</button
								>
							</dd>
						</dl>
					</div>
				{:else}
					<div class="child-list">
						<div class="child-head">
							<span>
								{#if selected.kind === 'area'}Subtopics
								{:else if selected.kind === 'topic'}Distinct issues
								{:else}Reports in this group{/if}
							</span>
							<span
								>{selectedChildren.length}{selected.kind === 'cluster' &&
								selected.issueCount > selectedChildren.length
									? ' shown'
									: ''}</span
							>
						</div>
						{#each selectedChildren as child (child.id)}
							<button class="child-row" onclick={() => selectNode(child, true)}>
								<span class="child-label">{child.label}</span>
								<span class="child-meta">
									{#if child.kind === 'issue'}
										<span
											class:open={child.state === 'open'}
											class:closed={child.state === 'closed'}>{child.state}</span
										>
										<span>{child.connections} links</span>
									{:else}
										<span>{child.issueCount.toLocaleString()} issues</span>
										<span>{child.openCount.toLocaleString()} open</span>
									{/if}
								</span>
								{#if child.title && child.title !== child.label}
									<span class="child-title">{short(child.title, 88)}</span>
								{/if}
							</button>
						{/each}
						{#if selectedChildren.length === 0}
							<p class="empty-note">No child nodes.</p>
						{/if}
					</div>
				{/if}
			{:else}
				<div class="empty-detail">
					<div class="empty-mark" aria-hidden="true"></div>
					<h2>Select a node</h2>
					<p>
						Start at a topic, drill into a subtopic, then open a distinct issue to see every GitHub
						report grouped beneath it.
					</p>
					<div class="reading-guide">
						<div>
							<span>1</span>
							<p><strong>Area</strong> is the part of SolidCraft, such as Kernel.</p>
						</div>
						<div>
							<span>2</span>
							<p><strong>Subtopic</strong> narrows it, such as Booleans.</p>
						</div>
						<div>
							<span>3</span>
							<p><strong>Distinct issue</strong> groups reports that look like the same bug.</p>
						</div>
						<div>
							<span>4</span>
							<p><strong>GitHub issue</strong> is the original report and links to GitHub.</p>
						</div>
					</div>
				</div>
			{/if}
		</aside>
	</main>
</div>

<style>
	:global(body) {
		margin: 0;
		background: #0b0f14;
		color: #c9d1d9;
		font-family:
			Inter,
			ui-sans-serif,
			system-ui,
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			sans-serif;
	}

	:global(*) {
		box-sizing: border-box;
	}

	:global(button),
	:global(input),
	:global(select) {
		font: inherit;
	}

	.app-shell {
		min-height: 100vh;
		display: flex;
		flex-direction: column;
	}

	.topbar {
		min-height: 68px;
		display: flex;
		align-items: center;
		gap: 1.5rem;
		padding: 0.8rem 1.1rem;
		border-bottom: 1px solid #1d2630;
		background: rgba(10, 14, 19, 0.94);
		backdrop-filter: blur(12px);
	}

	.brand {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		min-width: 330px;
	}

	.brand-mark {
		width: 30px;
		height: 30px;
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 3px;
		transform: rotate(12deg);
	}

	.brand-mark span {
		border-radius: 50%;
		background: #ff6b6b;
	}

	.brand-mark span:nth-child(2) {
		background: #4dabf7;
	}

	.brand-mark span:nth-child(3) {
		background: #38d9a9;
	}

	.brand-mark span:nth-child(4) {
		background: #fcc419;
	}

	h1 {
		margin: 0;
		font-size: 1rem;
		letter-spacing: 0.01em;
		color: #f0f6fc;
	}

	.brand p {
		margin: 0.15rem 0 0;
		font-size: 0.7rem;
		color: #7d8998;
	}

	.stats {
		display: flex;
		gap: 1.4rem;
		margin-left: auto;
	}

	.stat {
		display: grid;
		gap: 0.05rem;
		text-align: right;
	}

	.stat strong {
		color: #e6edf3;
		font-size: 0.95rem;
		font-weight: 600;
	}

	.stat span {
		color: #687586;
		font-size: 0.62rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.top-actions {
		display: flex;
		gap: 0.45rem;
	}

	.ghost-button,
	.small-button,
	.text-button,
	.link-button {
		cursor: pointer;
		border: 1px solid #2b3744;
		background: #111820;
		color: #c8d3df;
		border-radius: 0.4rem;
		padding: 0.42rem 0.6rem;
		font-size: 0.72rem;
		text-decoration: none;
		transition:
			border-color 120ms ease,
			background 120ms ease;
	}

	.ghost-button:hover,
	.small-button:hover {
		border-color: #4a6177;
		background: #16212c;
	}

	.workspace {
		flex: 1;
		min-height: 0;
		display: grid;
		grid-template-columns: 278px minmax(0, 1fr) 342px;
	}

	.panel {
		min-height: 0;
		overflow: auto;
		background: #0e1319;
		scrollbar-color: #2e3a47 transparent;
	}

	.controls {
		border-right: 1px solid #1d2630;
		padding: 1rem;
	}

	.details {
		border-left: 1px solid #1d2630;
		padding: 1rem;
	}

	.panel-title {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.5rem;
		margin-bottom: 0.85rem;
	}

	.panel-title h2 {
		margin: 0;
		color: #e6edf3;
		font-size: 0.82rem;
		font-weight: 600;
		letter-spacing: 0.02em;
	}

	.panel-title span {
		color: #687586;
		font-size: 0.64rem;
		white-space: nowrap;
	}

	.search-field,
	.filter-grid label {
		display: grid;
		gap: 0.3rem;
		margin-bottom: 0.7rem;
	}

	.search-field > span,
	.filter-grid label > span {
		color: #7f8c9b;
		font-size: 0.63rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}

	input[type='search'],
	select {
		width: 100%;
		border: 1px solid #273342;
		background: #0b1016;
		color: #dbe5ef;
		border-radius: 0.4rem;
		padding: 0.48rem 0.55rem;
		font-size: 0.76rem;
		outline: none;
	}

	input[type='search']:focus,
	select:focus {
		border-color: #4b7db3;
		box-shadow: 0 0 0 2px rgba(77, 171, 247, 0.12);
	}

	.filter-grid {
		margin-top: 0.35rem;
	}

	.toggle-list {
		display: grid;
		gap: 0.45rem;
		margin: 0.75rem 0;
		padding-top: 0.7rem;
		border-top: 1px solid #1b2430;
	}

	.toggle {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: #a9b6c4;
		font-size: 0.73rem;
		cursor: pointer;
	}

	.toggle input {
		accent-color: #4dabf7;
	}

	.text-button,
	.link-button {
		border: 0;
		background: none;
		padding: 0;
		color: #74a8dc;
		font-size: 0.7rem;
	}

	.text-button:hover,
	.link-button:hover {
		color: #a8cfff;
		text-decoration: underline;
	}

	.search-results {
		display: grid;
		gap: 0.25rem;
		margin: 0 0 0.9rem;
		max-height: 220px;
		overflow: auto;
	}

	.search-result {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 0.1rem 0.45rem;
		width: 100%;
		text-align: left;
		border: 1px solid transparent;
		border-radius: 0.35rem;
		background: #0b1016;
		color: #cbd7e3;
		padding: 0.4rem 0.45rem;
		cursor: pointer;
	}

	.search-result:hover {
		border-color: #2e465f;
		background: #111b25;
	}

	.result-kind {
		grid-row: span 2;
		align-self: center;
		color: #718399;
		font-size: 0.57rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.result-label {
		font-size: 0.73rem;
		color: #d9e4ef;
	}

	.result-title {
		grid-column: 2;
		color: #7f8c9b;
		font-size: 0.64rem;
		line-height: 1.3;
	}

	.legend {
		margin-top: 1.2rem;
		padding-top: 0.85rem;
		border-top: 1px solid #1b2430;
	}

	.legend h3,
	.method-note h3 {
		margin: 0 0 0.55rem;
		color: #7f8c9b;
		font-size: 0.62rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.legend-row {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		color: #91a0b1;
		font-size: 0.68rem;
		margin: 0.35rem 0;
	}

	.legend-dot {
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: #ff6b6b;
	}

	.legend-dot.topic {
		background: #a9b8c9;
		width: 7px;
		height: 7px;
	}

	.legend-dot.cluster {
		background: #63758a;
		width: 5px;
		height: 5px;
	}

	.legend-dot.issue {
		background: #405266;
		width: 4px;
		height: 4px;
	}

	.legend-line {
		width: 18px;
		height: 1px;
		background: #334155;
	}

	.method-note {
		margin-top: 1.2rem;
		padding-top: 0.85rem;
		border-top: 1px solid #1b2430;
	}

	.method-note p {
		margin: 0;
		color: #748394;
		font-size: 0.68rem;
		line-height: 1.55;
	}

	.graph-panel {
		position: relative;
		min-height: 0;
		background: #0b0f14;
	}

	.loading-state,
	.error-state {
		position: absolute;
		inset: 0;
		display: grid;
		place-content: center;
		justify-items: center;
		text-align: center;
		padding: 2rem;
	}

	.loading-state h2,
	.error-state h2 {
		margin: 0.9rem 0 0.2rem;
		color: #dbe5ef;
		font-size: 0.95rem;
	}

	.loading-state p,
	.error-state p {
		margin: 0.2rem 0;
		color: #7f8c9b;
		font-size: 0.76rem;
		max-width: 420px;
		line-height: 1.5;
	}

	.spinner {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		border: 2px solid #1f2b38;
		border-top-color: #4dabf7;
		animation: spin 800ms linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.error-state pre {
		margin-top: 0.8rem;
		padding: 0.5rem 0.65rem;
		border: 1px solid #2b3744;
		border-radius: 0.35rem;
		background: #0b1016;
		color: #b8c6d5;
		font-size: 0.7rem;
		max-width: 100%;
		overflow: auto;
	}

	.muted {
		color: #5f6d7c !important;
	}

	.error-state code {
		color: #a8cfff;
	}

	.detail-head h2 {
		margin: 0.3rem 0 0;
		color: #edf4fb;
		font-size: 1.02rem;
		line-height: 1.32;
		letter-spacing: -0.01em;
	}

	.detail-kicker {
		color: #718399;
		font-size: 0.61rem;
		letter-spacing: 0.09em;
		text-transform: uppercase;
	}

	.signature {
		margin: 0.5rem 0 0;
		color: #8d9db0;
		font-size: 0.7rem;
		line-height: 1.45;
		font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
		overflow-wrap: anywhere;
	}

	.detail-actions {
		display: flex;
		gap: 0.4rem;
		flex-wrap: wrap;
		margin-top: 0.75rem;
	}

	.small-button {
		display: inline-flex;
		align-items: center;
		padding: 0.35rem 0.5rem;
		font-size: 0.68rem;
	}

	.metric-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 0.7rem 1.2rem;
		margin: 1rem 0 0.85rem;
		padding: 0.75rem 0;
		border-top: 1px solid #1b2430;
		border-bottom: 1px solid #1b2430;
	}

	.metric-grid div {
		display: grid;
		gap: 0.05rem;
	}

	.metric-grid strong {
		color: #e6edf3;
		font-size: 0.9rem;
		font-weight: 600;
	}

	.metric-grid span {
		color: #6e7d8d;
		font-size: 0.6rem;
		letter-spacing: 0.05em;
		text-transform: uppercase;
	}

	.breadcrumb {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.25rem;
		margin-bottom: 0.85rem;
		color: #516273;
		font-size: 0.65rem;
	}

	.breadcrumb button {
		border: 0;
		background: none;
		padding: 0;
		color: #7495b5;
		cursor: pointer;
		font-size: inherit;
	}

	.breadcrumb button:hover {
		color: #a8cfff;
	}

	.issue-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.45rem;
		color: #8191a2;
		font-size: 0.68rem;
	}

	.labels {
		display: flex;
		flex-wrap: wrap;
		gap: 0.3rem;
		margin-top: 0.65rem;
	}

	.labels span {
		padding: 0.16rem 0.35rem;
		border: 1px solid #263544;
		border-radius: 999px;
		color: #9fb0c2;
		font-size: 0.6rem;
	}

	.open {
		color: #69db7c !important;
	}

	.closed {
		color: #ff8787 !important;
	}

	dl {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 0.4rem 0.8rem;
		margin: 0.9rem 0 0;
		font-size: 0.7rem;
	}

	dt {
		color: #657586;
	}

	dd {
		margin: 0;
		color: #a9b8c8;
	}

	.child-list {
		display: grid;
		gap: 0.2rem;
	}

	.child-head {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		margin: 0.2rem 0 0.4rem;
		color: #718399;
		font-size: 0.6rem;
		letter-spacing: 0.07em;
		text-transform: uppercase;
	}

	.child-row {
		display: grid;
		grid-template-columns: minmax(0, 1fr) auto;
		gap: 0.1rem 0.5rem;
		width: 100%;
		border: 1px solid transparent;
		border-radius: 0.35rem;
		background: #0b1016;
		padding: 0.45rem 0.5rem;
		text-align: left;
		cursor: pointer;
	}

	.child-row:hover {
		border-color: #2e465f;
		background: #111b25;
	}

	.child-label {
		color: #d2deea;
		font-size: 0.72rem;
		line-height: 1.32;
	}

	.child-meta {
		grid-column: 2;
		grid-row: span 2;
		align-self: center;
		display: grid;
		justify-items: end;
		gap: 0.12rem;
		color: #6f8092;
		font-size: 0.6rem;
		white-space: nowrap;
	}

	.child-title {
		grid-column: 1;
		color: #748394;
		font-size: 0.64rem;
		line-height: 1.3;
	}

	.empty-note {
		color: #718399;
		font-size: 0.7rem;
		line-height: 1.5;
	}

	.empty-detail {
		padding: 0.5rem 0.2rem;
	}

	.empty-mark {
		width: 38px;
		height: 38px;
		border: 1px solid #2b3a4a;
		border-radius: 50%;
		background:
			radial-gradient(circle at 35% 35%, #4dabf7 0 3px, transparent 3.5px),
			radial-gradient(circle at 65% 42%, #ff6b6b 0 4px, transparent 4.5px),
			radial-gradient(circle at 48% 68%, #38d9a9 0 3px, transparent 3.5px);
	}

	.empty-detail h2 {
		margin: 0.85rem 0 0.35rem;
		color: #dbe5ef;
		font-size: 0.92rem;
	}

	.empty-detail > p {
		margin: 0;
		color: #7f8c9b;
		font-size: 0.72rem;
		line-height: 1.55;
	}

	.reading-guide {
		display: grid;
		gap: 0.65rem;
		margin-top: 1.2rem;
	}

	.reading-guide div {
		display: grid;
		grid-template-columns: 18px minmax(0, 1fr);
		gap: 0.45rem;
		align-items: start;
	}

	.reading-guide span {
		display: grid;
		place-items: center;
		width: 18px;
		height: 18px;
		border: 1px solid #2b3a4a;
		border-radius: 50%;
		color: #7aa2f7;
		font-size: 0.6rem;
	}

	.reading-guide p {
		margin: 0;
		color: #7f8c9b;
		font-size: 0.68rem;
		line-height: 1.45;
	}

	.reading-guide strong {
		color: #a9b8c8;
		font-weight: 600;
	}

	@media (max-width: 1120px) {
		.workspace {
			grid-template-columns: 236px minmax(0, 1fr) 300px;
		}

		.brand {
			min-width: 260px;
		}

		.stats {
			gap: 0.8rem;
		}
	}

	@media (max-width: 860px) {
		.topbar {
			flex-wrap: wrap;
			gap: 0.6rem;
		}

		.brand {
			min-width: 0;
		}

		.stats {
			order: 3;
			width: 100%;
			margin-left: 0;
			justify-content: space-between;
		}

		.top-actions {
			margin-left: auto;
		}

		.workspace {
			display: flex;
			flex-direction: column;
		}

		.graph-panel {
			order: -1;
			height: 58vh;
		}

		.controls,
		.details {
			border: 0;
			border-top: 1px solid #1d2630;
			max-height: none;
		}

		.controls {
			order: 0;
		}

		.details {
			order: 1;
		}
	}
</style>
