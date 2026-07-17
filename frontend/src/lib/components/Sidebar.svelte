<script>
  import { appState, downloadRemove, showFlatResults } from '../global-state.svelte.js';
  import { GetStats, GetDownloadQueue, ScrapeAll, GetCollections, CollectionWallpapers, GetDuplicates, GetWallpapersByIDs, AnalysisStats, PauseAnalysis, ResumeAnalysis, DeleteDuplicates, AnalyzeAll, ReanalyzeAll } from '../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
  import { onMount } from 'svelte';
  import { click } from '../actions.js';

  let scraping = $state(false);
  let scrapeResult = $state(null);
  let scrapeProgress = $state([]);
  let totalSources = $state(0);
  let completedSources = $state(0);

  let activeDl = $derived(appState.downloads.filter(d => d.status === 'downloading'));
  let completedDl = $derived(appState.downloads.filter(d => d.status === 'done'));
  let collections = $state([]);
  let dupGroups = $state([]);
  let dupConfirm = $state(false);
  let deletingDups = $state(false);
  let analysis = $state({ submitted: 0, done: 0, active: 0, paused: false });

  async function refreshStats() {
    try {
      const s = await GetStats();
      appState.stats = s;
      const q = await GetDownloadQueue();
      appState.downloadQueue = q;
    } catch(e) { console.error('[Sidebar] refreshStats error:', e); }
  }

  async function loadCollections() {
    try {
      collections = await GetCollections() || [];
      dupGroups = await GetDuplicates() || [];
    } catch(e) { console.error('[Sidebar] collections:', e); }
  }

  async function openCollection(name) {
    try {
      const data = await CollectionWallpapers(name);
      showFlatResults(data, 'Collection: ' + name);
    } catch(e) { console.error('[Sidebar] openCollection:', e); }
  }

  async function openDuplicates(ids) {
    try {
      const data = await GetWallpapersByIDs(ids);
      showFlatResults(data, 'Duplicate group (' + ids.length + ' images)');
    } catch(e) { console.error('[Sidebar] duplicates:', e); }
  }

  async function removeDuplicates() {
    if (deletingDups) return;
    deletingDups = true;
    try {
      const n = await DeleteDuplicates();
      dupConfirm = false;
      dupGroups = await GetDuplicates() || [];
      loadCollections();
    } catch(e) { console.error('[Sidebar] removeDuplicates:', e); }
    deletingDups = false;
  }

  async function loadAnalysis() {
    try {
      analysis = await AnalysisStats();
    } catch(e) { console.error('[Sidebar] analysis:', e); }
  }

  function togglePause() {
    if (analysis.paused) ResumeAnalysis(); else PauseAnalysis();
    loadAnalysis();
  }

  let analyzing = $state(false);
  async function startAnalysis() {
    if (analyzing) return;
    analyzing = true;
    try {
      // Already-analyzed library -> full re-run; otherwise analyze only the
      // wallpapers that haven't been analyzed yet.
      if (analysis.done > 0) {
        await ReanalyzeAll();
      } else {
        await AnalyzeAll();
      }
      loadAnalysis();
    } catch(e) { console.error('[Sidebar] startAnalysis:', e); }
    finally { analyzing = false; }
  }

  async function handleScrape() {
    scraping = true;
    appState.isScraping = true;
    scrapeResult = null;
    scrapeProgress = [];
    totalSources = 0;
    completedSources = 0;
    ScrapeAll(1).catch(e => console.error('[Sidebar] scrape:', e));
  }

  function setView(v) {
    appState.view = v;
    appState.favoritesOnly = v === 'favorites';
  }

  function openDownload(id) {
    const item = appState.wallpapers.find(w => w.id === id);
    if (item) {
      appState.currentWallpaper = item;
      appState.previewOpen = true;
    }
  }

  function dismissCompleted(id) {
    downloadRemove(id);
  }

  refreshStats();
  loadCollections();
  loadAnalysis();
  const interval = setInterval(refreshStats, 3000);
  const collectionInterval = setInterval(loadCollections, 15000);
  const analysisInterval = setInterval(loadAnalysis, 1000);

  onMount(() => {
    const unsubs = [
      EventsOn('scrape:total', (data) => {
        totalSources = data.total || 0;
      }),
      EventsOn('scrape:started', (data) => {
        const src = data.source;
        if (!scrapeProgress.find(p => p.source === src)) {
          scrapeProgress = [...scrapeProgress, { source: src, term: '', added: 0, pages: 0 }];
        }
      }),
      EventsOn('scrape:progress', (data) => {
        const src = data.source;
        scrapeProgress = scrapeProgress.map(p =>
          p.source === src
            ? { ...p, term: data.term || p.term, added: data.added || 0, total: data.total || 0, page: data.page || 0 }
            : p
        );
      }),
      EventsOn('scrape:complete', (data) => {
        const src = data.source;
        completedSources++;
        scrapeProgress = scrapeProgress.map(p =>
          p.source === src ? { ...p, done: true } : p
        );
        if (completedSources >= totalSources && totalSources > 0) {
          const totalAdded = scrapeProgress.reduce((sum, p) => sum + (p.added || 0), 0);
          scrapeResult = `Found ${totalAdded} new wallpapers`;
          scraping = false;
          setTimeout(() => { scrapeResult = null; scrapeProgress = []; }, 5000);
        }
        refreshStats();
      }),
    ];
    return () => {
      unsubs.forEach(u => u());
      clearInterval(interval);
      clearInterval(collectionInterval);
      clearInterval(analysisInterval);
    };
  });
</script>

<aside class="w-64 h-screen flex flex-col bg-zinc-900 border-r border-zinc-800 shrink-0 overflow-y-auto">
  <div class="flex items-center gap-2.5 px-4 h-14 border-b border-zinc-800 shrink-0">
    <div class="text-zinc-100">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2"/>
        <circle cx="8.5" cy="8.5" r="1.5"/>
        <path d="m21 15-5-5L5 21"/>
      </svg>
    </div>
    <span class="font-semibold text-zinc-100 text-sm">Wallpaper Chooser</span>
  </div>

  <nav class="p-2 space-y-0.5">
    <button class="flex items-center gap-2.5 w-full px-3 py-2 rounded-md text-sm transition-colors cursor-pointer {appState.view === 'grid' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}" type="button" use:click={() => setView('grid')}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/>
        <rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>
      </svg>
      <span>All Wallpapers</span>
    </button>

    <button class="flex items-center gap-2.5 w-full px-3 py-2 rounded-md text-sm transition-colors cursor-pointer {appState.view === 'favorites' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}" type="button" use:click={() => setView('favorites')}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
      </svg>
      <span>Favorites</span>
      {#if appState.stats.favorites}
        <span class="ml-auto text-[11px] bg-zinc-700 text-zinc-300 px-1.5 py-0.5 rounded-full">{appState.stats.favorites}</span>
      {/if}
    </button>

    <button class="flex items-center gap-2.5 w-full px-3 py-2 rounded-md text-sm transition-colors cursor-pointer {appState.view === 'downloads' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}" type="button" use:click={() => setView('downloads')}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/>
      </svg>
      <span>Downloaded</span>
      {#if appState.stats.downloaded}
        <span class="ml-auto text-[11px] bg-zinc-700 text-zinc-300 px-1.5 py-0.5 rounded-full">{appState.stats.downloaded}</span>
      {/if}
    </button>

    <button class="flex items-center gap-2.5 w-full px-3 py-2 rounded-md text-sm transition-colors cursor-pointer {appState.view === 'settings' ? 'bg-zinc-800 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}" type="button" use:click={() => setView('settings')}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>
      </svg>
      <span>Settings</span>
    </button>
  </nav>

  {#if collections.length > 0}
    <div class="px-3 py-2 border-t border-zinc-800">
      <div class="flex items-center gap-1.5 text-xs text-zinc-400 mb-2">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 3l1.9 4.8L19 9.3l-4 3.4.9 5.3L12 15.8 8.1 18l.9-5.3-4-3.4 5.1-1.5L12 3z"/>
        </svg>
        <span>AI Collections</span>
      </div>
      <div class="space-y-0.5 max-h-52 overflow-y-auto">
        {#each collections as c (c.name)}
          <button type="button" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-xs text-zinc-300 hover:bg-zinc-800 cursor-pointer transition-colors" use:click={() => openCollection(c.name)}>
            <span class="truncate">{c.name}</span>
            <span class="ml-auto text-zinc-600">{c.count}</span>
          </button>
        {/each}
      </div>
    </div>
  {/if}

  {#if dupGroups.length > 0}
    <div class="px-3 py-2 border-t border-zinc-800">
      <div class="flex items-center gap-1.5 text-xs text-zinc-400 mb-2">
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><path d="M14 3h7v7M3 14h7v7"/>
        </svg>
        <span>Duplicates</span>
        <span class="ml-auto text-zinc-600">{dupGroups.length} groups</span>
      </div>
      <div class="space-y-0.5 max-h-40 overflow-y-auto">
        {#each dupGroups as g, i (i)}
          <button type="button" class="flex items-center gap-2 w-full px-2 py-1.5 rounded text-xs text-zinc-300 hover:bg-zinc-800 cursor-pointer transition-colors" use:click={() => openDuplicates(g.ids)}>
            <span class="truncate">Group #{i + 1}</span>
            <span class="ml-auto text-zinc-600">{g.ids.length} images</span>
          </button>
        {/each}
      </div>
      {#if dupConfirm}
        <div class="flex items-center gap-2 mt-2">
          <button type="button" disabled={deletingDups} class="px-2.5 py-1 rounded-md bg-red-900 text-red-100 text-xs font-medium hover:bg-red-800 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-default" use:click={removeDuplicates}>{deletingDups ? 'Removing…' : 'Confirm'}</button>
          <button type="button" class="px-2.5 py-1 rounded-md bg-zinc-800 text-zinc-300 text-xs hover:bg-zinc-700 cursor-pointer transition-colors" use:click={() => dupConfirm = false}>Cancel</button>
        </div>
      {:else}
        <button type="button" disabled={deletingDups} class="mt-2 w-full px-2.5 py-1.5 rounded-md bg-red-900/70 text-red-100 text-xs font-medium hover:bg-red-800 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-default" use:click={() => dupConfirm = true}>Remove duplicates</button>
      {/if}
    </div>
  {/if}

  {#if activeDl.length > 0 || completedDl.length > 0}
    <div class="px-3 py-2 border-t border-zinc-800">
      <div class="flex items-center gap-1.5 text-xs text-zinc-400 mb-2">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/>
        </svg>
        <span>Downloads</span>
        {#if activeDl.length > 0}
          <span class="text-[10px] bg-blue-500 text-white px-1 rounded-full">{activeDl.length}</span>
        {/if}
      </div>
      <div class="space-y-0.5">
        {#each activeDl as dl (dl.id)}
          <div class="flex items-center gap-2 px-2 py-1.5 rounded text-xs text-zinc-300 hover:bg-zinc-800 cursor-pointer transition-colors" use:click={() => openDownload(dl.id)} role="button" tabindex="0">
            <span class="w-1.5 h-1.5 rounded-full bg-blue-500 shrink-0"></span>
            <span class="truncate">Wallpaper #{dl.id}</span>
            <span class="ml-auto text-zinc-500">downloading</span>
          </div>
        {/each}
        {#each completedDl as dl (dl.id)}
          <div class="flex items-center gap-2 px-2 py-1.5 rounded text-xs text-zinc-300 hover:bg-zinc-800 cursor-pointer transition-colors" use:click={() => openDownload(dl.id)} role="button" tabindex="0">
            <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="3" class="shrink-0">
              <path d="M20 6 9 17l-5-5"/>
            </svg>
            <span class="truncate">Wallpaper #{dl.id}</span>
            <button class="ml-auto p-0.5 rounded hover:bg-zinc-700 text-zinc-500 hover:text-zinc-300 cursor-pointer" type="button" use:click={(e) => { e.stopPropagation(); dismissCompleted(dl.id); }} aria-label="Dismiss">
              <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                <path d="M18 6 6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <div class="px-3 py-2 border-t border-zinc-800">
    {#if analysis.submitted > analysis.done || analysis.paused}
      <div class="flex items-center justify-between text-xs mb-1">
        <span class="text-zinc-400">{analysis.paused ? 'Analysis paused' : 'Analyzing library…'}</span>
        <span class="text-zinc-500">{analysis.done}/{analysis.submitted}</span>
      </div>
      <div class="h-1.5 w-full rounded-full bg-zinc-800 overflow-hidden">
        <div class="h-full bg-zinc-100 transition-all duration-300" style="width:{analysis.submitted > 0 ? Math.round(analysis.done * 100 / analysis.submitted) : 0}%"></div>
      </div>
      <button type="button" class="mt-1.5 text-xs text-zinc-400 hover:text-zinc-100 cursor-pointer transition-colors" use:click={togglePause}>
        {analysis.paused ? 'Resume analysis' : 'Pause analysis'}
      </button>
      {:else}
        <button type="button"
          class="flex items-center justify-center gap-2 w-full px-3 py-2 rounded-md text-sm font-medium bg-zinc-800 text-zinc-100 hover:bg-zinc-700 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          use:click={startAnalysis} disabled={analyzing}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2a10 10 0 1 0 10 10"/><path d="M12 6v6l4 2"/>
          </svg>
          {analyzing ? 'Starting…' : (analysis.done > 0 ? 'Re-run AI Analysis' : 'Start AI Analysis')}
        </button>
        {#if analysis.done > 0}
          <p class="text-[11px] text-zinc-500 mt-1">{analysis.done} wallpaper{analysis.done === 1 ? '' : 's'} analyzed</p>
        {/if}
      {/if}
  </div>

  <div class="mt-auto p-3 border-t border-zinc-800 space-y-2">
    {#if scrapeResult}
      <div class="text-xs text-green-400 bg-green-500/10 px-2.5 py-1.5 rounded">{scrapeResult}</div>
    {/if}
    {#if scraping && scrapeProgress.length > 0}
      <div class="space-y-1">
        {#each scrapeProgress as p}
          <div class="flex items-center gap-1.5 text-xs {p.done ? 'text-zinc-500' : 'text-zinc-300'}">
            {#if p.done}
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="3" class="shrink-0">
                <path d="M20 6 9 17l-5-5"/>
              </svg>
            {:else if p.term}
              <span class="truncate">{p.source}: {p.term}</span>
            {:else}
              <span class="w-2 h-2 rounded-full bg-zinc-500 animate-pulse shrink-0"></span>
            {/if}
            <span class="ml-auto">{p.source}</span>
          </div>
        {/each}
        <div class="text-[11px] text-zinc-500">
          {completedSources}/{totalSources} sources
          {#if scrapeProgress.some(p => p.added > 0)}
            &middot; {scrapeProgress.reduce((s, p) => s + (p.total || 0), 0)} new
          {/if}
        </div>
      </div>
    {/if}

    <button class="flex items-center justify-center gap-2 w-full px-3 py-2 rounded-md text-sm font-medium bg-zinc-100 text-zinc-900 hover:bg-zinc-200 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors" type="button" use:click={handleScrape} disabled={scraping}>
      {#if scraping}
        <span class="w-3.5 h-3.5 border-2 border-zinc-600 border-t-zinc-900 rounded-full animate-spin"></span>
        Finding wallpapers...
      {:else}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
        </svg>
        Find New Wallpapers
      {/if}
    </button>
    <p class="text-[11px] text-zinc-600 text-center">Runs in the background</p>
  </div>
</aside>
