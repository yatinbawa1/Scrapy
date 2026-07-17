<script>
  import { appState, downloadStart, downloadDone, downloadFail, downloadIsActive, downloadGetOrigin, downloadRemove, showFlatResults, clearFlat } from '../global-state.svelte.js';
  import { BrowseSortedFiltered, BrowseTotalCount, GetDownloadedWallpapers, ToggleFavorite, DownloadWallpaper, CancelDownload, SetWallpaper, SemanticSearch, SearchByColor } from '../../../wailsjs/go/main/App.js';
  import { EventsOn } from '../../../wailsjs/runtime/runtime.js';
  import { onMount } from 'svelte';
  import { click } from '../actions.js';

  let loading = $state(false);
  let hoveredId = $state(null);
  let hoverTimer = $state(null);
  let searchQuery = $state('');
  let searchSource = $state('');
  let debounceTimer;
  let confirmTarget = $state(null);
  let totalCount = $state(0);
  let currentPage = $state(1);
  let searchMode = $state('text'); // 'text' | 'ai'
  let aiQuery = $state('');
  let colorHex = $state('#3b82f6');
  let aiBusy = $state(false);

  const pageSize = 50;
  let totalPages = $derived(Math.ceil(totalCount / pageSize) || 1);

  const HOVER_DELAY = 3000;

  const sortOptions = [
    { value: 'random', label: 'Random' },
    { value: 'latest', label: 'Latest' },
    { value: 'source', label: 'Source' },
  ];

  async function loadPage(page) {
    if (loading) return;
    if (appState.flatMode) return; // flat results are managed externally
    appState.isLoading = true;
    loading = true;
    try {
      let data, total;
      if (appState.view === 'downloads') {
        data = await GetDownloadedWallpapers(page, pageSize, appState.sortBy);
        total = 0;
      } else {
        const res = await BrowseSortedFiltered(page, pageSize, appState.view === 'favorites', appState.sortBy, appState.category, searchQuery, searchSource);
        data = res.wallpapers;
        total = res.total;
      }
      appState.wallpapers = data || [];
      appState.currentPage = page;
      currentPage = page;
      totalCount = total || 0;
    } catch(e) {
      console.error('[Grid] load error:', e);
    }
    appState.isLoading = false;
    loading = false;
    appState.loaded = true;
  }

  // searchVisually runs a semantic (image-level) search for the given query and
  // shows the results as a flat list. This is the "check every image" path used
  // both by the AI search mode and as a fallback when keyword search finds
  // nothing textually.
  async function searchVisually(q) {
    q = (q || '').trim();
    if (!q) return;
    aiBusy = true;
    try {
      const data = await SemanticSearch(q, 120);
      showFlatResults(data, 'Visual matches for "' + q + '"');
    } catch(e) { console.error('[Grid] semantic:', e); }
    aiBusy = false;
  }

  async function runSemantic() {
    await searchVisually(aiQuery);
  }

  async function runColorSearch() {
    aiBusy = true;
    try {
      const data = await SearchByColor(colorHex, 120);
      showFlatResults(data, 'Color ' + colorHex);
    } catch(e) { console.error('[Grid] color search:', e); }
    aiBusy = false;
  }

  function setMode(m) {
    if (searchMode === m) return;
    searchMode = m;
    if (m === 'text') {
      clearFlat();
      reload();
    }
  }

  function clearFlatView() {
    clearFlat();
    reload();
  }

  function goToPage(p) {
    if (p < 1 || p > totalPages || p === currentPage) return;
    loadPage(p);
  }

  async function reload() { clearFlat(); await loadPage(1); }

  function pageRange() {
    const pages = [];
    const start = Math.max(1, currentPage - 2);
    const end = Math.min(totalPages, currentPage + 2);
    if (start > 1) pages.push(1);
    if (start > 2) pages.push(null);
    for (let i = start; i <= end; i++) pages.push(i);
    if (end < totalPages - 1) pages.push(null);
    if (end < totalPages) pages.push(totalPages);
    return pages;
  }

  async function doDownload(w, origin) {
    if (w.status === 'downloaded' || downloadIsActive(w.id)) return;
    downloadStart(w.id, origin);
    try { await DownloadWallpaper(w.id); }
    catch(e) { console.error('download:', e); downloadFail(w.id, e.message || 'download failed'); }
  }

  function handleCardClick(w) {
    if (appState.view === 'downloads') { confirmTarget = w; return; }
    appState.currentWallpaper = w;
    appState.previewOpen = true;
  }

  function confirmSet() {
    if (!confirmTarget) return;
    SetWallpaper(confirmTarget.id).catch(e => console.error('[Grid] set wallpaper:', e));
    confirmTarget = null;
  }
  function cancelConfirm() { confirmTarget = null; }

  function handleCardHover(w) {
    hoveredId = w.id;
    if (w.status === 'downloaded' || downloadIsActive(w.id)) return;
    clearTimeout(hoverTimer);
    hoverTimer = setTimeout(() => { if (hoveredId === w.id) doDownload(w, 'hover'); }, HOVER_DELAY);
  }

  function handleCardLeave(w) {
    if (hoveredId === w.id) hoveredId = null;
    clearTimeout(hoverTimer);
    if (downloadIsActive(w.id) && downloadGetOrigin(w.id) === 'hover') {
      CancelDownload(w.id); downloadRemove(w.id);
    }
  }

  async function toggleFav(e, w) {
    e.stopPropagation();
    try {
      await ToggleFavorite(w.id);
      appState.wallpapers = appState.wallpapers.map(item =>
        item.id === w.id ? { ...item, isFavorite: !item.isFavorite } : item
      );
    } catch(e) { console.error(e); }
  }

  function handleSortChange(value) { appState.sortBy = value; reload(); }

  // When a keyword query yields no text matches, fall back to a visual search
  // across every image using the same query.
  async function textSearchWithFallback() {
    await reload();
    if (searchQuery.trim() && appState.wallpapers.length === 0) {
      await searchVisually(searchQuery);
    }
  }

  function onSearchInput() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(textSearchWithFallback, 700);
  }
  function onSearchKeydown(e) { if (e.key === 'Enter') { clearTimeout(debounceTimer); textSearchWithFallback(); } }
  function onSourceChange() { reload(); }

  function thumbUrl(w) {
    if (w.status === 'downloaded' && w.thumbnailPath) return '/cache/thumbnails/' + w.thumbnailPath.replace(/^.*thumbnails\//, '');
    if (w.thumbnailUrl) return w.thumbnailUrl;
    if (w.thumbnailPath) return '/cache/' + w.thumbnailPath.replace(/^.*cache\//, '');
    return '';
  }

  onMount(() => {
    loadPage(1);

    const unsubs = [
      EventsOn('wallpaper:downloaded', (data) => {
        downloadDone(data.wallpaperId);
        appState.wallpapers = appState.wallpapers.map(item =>
          item.id === data.wallpaperId ? { ...item, status: 'downloaded' } : item
        );
        if (appState.previewOpen && appState.currentWallpaper?.id === data.wallpaperId)
          appState.currentWallpaper = { ...appState.currentWallpaper, status: 'downloaded' };
      }),
      EventsOn('scrape:complete', () => { reload(); }),
      EventsOn('thumbnail:batch', () => setTimeout(reload, 2000)),
      EventsOn('wallpaper:deleted', () => { clearFlat(); reload(); }),
      EventsOn('download:failed', (data) => downloadFail(data.wallpaperId, data.error)),
    ];

    return () => {
      unsubs.forEach(u => u());
      clearTimeout(hoverTimer);
    };
  });
</script>

<div class="flex flex-col h-full overflow-hidden">

  <!-- Toolbar (always visible) -->
  <div class="shrink-0 border-b border-zinc-800 bg-zinc-900/50 px-4 py-3">
    {#if appState.flatMode}
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-xs px-2 py-1 rounded bg-zinc-800 text-zinc-300">Showing: {appState.flatLabel}</span>
        <button type="button" class="px-2.5 py-1 rounded-md text-xs text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 cursor-pointer transition-colors" use:click={clearFlatView}>Clear</button>
        <span class="text-xs text-zinc-500 ml-auto">{appState.wallpapers.length} results</span>
      </div>
    {:else}
      <div class="flex items-center gap-3 flex-wrap">
        <div class="flex items-center rounded-md overflow-hidden border border-zinc-700">
          <button type="button" class="px-2.5 py-1.5 text-xs transition-colors cursor-pointer {searchMode === 'text' ? 'bg-zinc-700 text-zinc-100' : 'bg-zinc-800 text-zinc-400 hover:text-zinc-200'}" use:click={() => setMode('text')}>Keyword</button>
          <button type="button" class="px-2.5 py-1.5 text-xs transition-colors cursor-pointer {searchMode === 'ai' ? 'bg-zinc-700 text-zinc-100' : 'bg-zinc-800 text-zinc-400 hover:text-zinc-200'}" use:click={() => setMode('ai')}>AI</button>
        </div>
        {#if searchMode === 'ai'}
          <div class="relative flex-1 min-w-[200px] max-w-md">
            <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
            </svg>
            <input type="text" placeholder="Describe a wallpaper: misty forest, neon city..." bind:value={aiQuery}
              oninput={() => { clearTimeout(debounceTimer); debounceTimer = setTimeout(runSemantic, 600); }} onkeydown={(e) => { if (e.key === 'Enter') { clearTimeout(debounceTimer); runSemantic(); } }}
              class="w-full bg-zinc-800 border border-zinc-700 rounded-md pl-8 pr-3 py-1.5 text-sm text-zinc-100 placeholder-zinc-500 outline-none focus:border-zinc-500" />
            {#if aiBusy}
              <span class="absolute right-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 border-2 border-zinc-500 border-t-zinc-200 rounded-full animate-spin"></span>
            {/if}
          </div>
        {:else}
          <div class="relative flex-1 min-w-[200px] max-w-xs">
            <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
            </svg>
            <input type="text" placeholder="Search..." bind:value={searchQuery} oninput={onSearchInput} onkeydown={onSearchKeydown}
              class="w-full bg-zinc-800 border border-zinc-700 rounded-md pl-8 pr-3 py-1.5 text-sm text-zinc-100 placeholder-zinc-500 outline-none focus:border-zinc-500" />
          </div>
        {/if}
        <label class="relative flex items-center gap-1.5 cursor-pointer px-2 py-1.5 rounded-md bg-zinc-800 border border-zinc-700 text-xs text-zinc-300 hover:border-zinc-500 transition-colors">
          <span class="w-3.5 h-3.5 rounded-full border border-zinc-600" style="background:{colorHex}"></span>
          Color
          <input type="color" bind:value={colorHex} onchange={runColorSearch} class="absolute inset-0 opacity-0 cursor-pointer w-full h-full" />
        </label>
        {#if searchMode === 'text'}
          <select bind:value={searchSource} onchange={onSourceChange}
            class="bg-zinc-800 border border-zinc-700 rounded-md px-3 py-1.5 text-sm text-zinc-100 outline-none focus:border-zinc-500">
            <option value="">All Sources</option>
            <option value="wallhaven">Wallhaven</option>
            <option value="unsplash">Unsplash</option>
            <option value="pexels">Pexels</option>
          </select>
          <div class="flex items-center gap-1">
            {#each sortOptions as opt}
              <button type="button" class="px-2.5 py-1 rounded-md text-xs transition-colors cursor-pointer {appState.sortBy === opt.value ? 'bg-zinc-700 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200'}"
                use:click={() => handleSortChange(opt.value)}>{opt.label}</button>
            {/each}
          </div>
        {/if}
        <span class="text-xs text-zinc-500 ml-auto">{appState.wallpapers.length} / {totalCount}</span>
      </div>
    {/if}
  </div>

  <!-- Content -->
  <div class="flex-1 overflow-y-auto">
    {#if appState.isLoading && appState.wallpapers.length === 0}
      <div class="flex flex-col items-center justify-center h-full gap-4">
        <div class="w-8 h-8 border-2 border-zinc-700 border-t-zinc-300 rounded-full animate-spin"></div>
        <p class="text-zinc-400 text-sm">Loading wallpapers...</p>
      </div>
    {:else if appState.wallpapers.length === 0}
      <div class="flex flex-col items-center justify-center h-full gap-3">
        <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="#3f3f46" stroke-width="1.5">
          <rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>
        </svg>
        <p class="text-zinc-400">No wallpapers found</p>
        <p class="text-zinc-600 text-xs">Click "Find New Wallpapers" in the sidebar to get started</p>
      </div>
    {:else}
      <div class="p-4">
        <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
          {#each appState.wallpapers as w (w.id)}
            <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
            <div class="relative group rounded-lg overflow-hidden bg-zinc-900 border border-zinc-800 cursor-pointer transition-all hover:border-zinc-600 {appState.downloads.some(d => d.id === w.id && d.status === 'downloading') ? 'ring-2 ring-blue-500' : ''}"
              use:click={() => handleCardClick(w)}
              onmouseenter={() => handleCardHover(w)} onmouseleave={() => handleCardLeave(w)}
              role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && handleCardClick(w)}>
              <div class="aspect-[4/3] relative overflow-hidden">
                {#if thumbUrl(w)}
                  <img src={thumbUrl(w)} alt="" loading="lazy" class="w-full h-full object-cover" />
                {:else}
                  <div class="flex items-center justify-center w-full h-full bg-zinc-800">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#3f3f46" stroke-width="1.5">
                      <rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>
                    </svg>
                  </div>
                {/if}
                {#if appState.downloads.some(d => d.id === w.id && d.status === 'downloading')}
                  <div class="absolute inset-0 flex items-center justify-center bg-black/40">
                    <svg class="w-8 h-8 animate-spin" viewBox="0 0 36 36">
                      <circle cx="18" cy="18" r="16" fill="none" stroke="rgba(255,255,255,0.1)" stroke-width="3"/>
                      <circle cx="18" cy="18" r="16" fill="none" stroke="#3b82f6" stroke-width="3" stroke-dasharray="100" stroke-dashoffset="60" stroke-linecap="round"/>
                    </svg>
                  </div>
                {/if}
                {#if w.status === 'downloaded'}
                  <div class="absolute top-2 right-2 bg-green-500/20 rounded p-1">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#22c55e" stroke-width="3"><path d="M20 6 9 17l-5-5"/></svg>
                  </div>
                {/if}
                <button type="button" class="absolute top-2 left-2 p-1.5 rounded-md opacity-0 group-hover:opacity-100 transition-opacity {w.isFavorite ? 'bg-red-500/20 opacity-100' : 'bg-black/40 hover:bg-black/60'} cursor-pointer"
                  aria-label="Toggle favorite" use:click={(e) => toggleFav(e, w)}>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill={w.isFavorite ? '#ef4444' : 'none'} stroke={w.isFavorite ? '#ef4444' : '#a1a1aa'} stroke-width="2">
                    <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
                  </svg>
                </button>
                <div class="absolute bottom-2 left-2 bg-black/60 text-zinc-300 text-[10px] px-1.5 py-0.5 rounded">{w.source}</div>
              </div>
            </div>
          {/each}
        </div>

        <!-- pagination -->
        {#if totalPages > 1 && !appState.flatMode}
          <div class="flex items-center justify-center gap-1 mt-6 pb-2">
            <button type="button" disabled={currentPage <= 1}
              class="px-2.5 py-1 rounded text-xs cursor-pointer disabled:opacity-30 disabled:cursor-default {currentPage <= 1 ? 'text-zinc-600' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'}"
              use:click={() => goToPage(currentPage - 1)}>Prev</button>
            {#each pageRange() as p}
              {#if p === null}
                <span class="px-1 text-zinc-600 text-xs">...</span>
              {:else}
                <button type="button"
                  class="px-2.5 py-1 rounded text-xs cursor-pointer transition-colors {p === currentPage ? 'bg-zinc-700 text-zinc-100' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'}"
                  use:click={() => goToPage(p)}>{p}</button>
              {/if}
            {/each}
            <button type="button" disabled={currentPage >= totalPages}
              class="px-2.5 py-1 rounded text-xs cursor-pointer disabled:opacity-30 disabled:cursor-default {currentPage >= totalPages ? 'text-zinc-600' : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800'}"
              use:click={() => goToPage(currentPage + 1)}>Next</button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if confirmTarget}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" use:click={cancelConfirm}>
    <div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5 max-w-sm w-full mx-4 space-y-4" use:click={(e) => e.stopPropagation()}>
      <p class="text-sm text-zinc-200">Set this wallpaper as your desktop background?</p>
      <div class="flex gap-2 justify-end">
        <button type="button" class="px-3 py-1.5 rounded-md bg-zinc-800 text-zinc-300 text-sm hover:bg-zinc-700 cursor-pointer transition-colors" use:click={cancelConfirm}>Cancel</button>
        <button type="button" class="px-3 py-1.5 rounded-md bg-zinc-100 text-zinc-900 text-sm font-medium hover:bg-zinc-200 cursor-pointer transition-colors" use:click={confirmSet}>Set Wallpaper</button>
      </div>
    </div>
  </div>
{/if}
