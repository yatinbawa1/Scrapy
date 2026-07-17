<script>
  import { appState } from '../global-state.svelte.js';
  import { click } from '../actions.js';
  import { formatBytes } from '../utils.js';
  import { GetStats, GetConfig, CleanupCache, GetCategoryStats, SelectFolder, GetSourceStats, GetSearchTerms, AddSearchTerm, RemoveSearchTerm, GetProviders, ToggleSource, SetMaxCacheSizeMB, SetConcurrentDownloads, ResetDatabase, GetStorageInfo, ClearStorage, ClearAllStorage } from '../../../wailsjs/go/main/App.js';

  let config = $state({});
  let searchTerms = $state([]);
  let providers = $state([]);
  let newTerm = $state('');
  let storageItems = $state([]);
  let clearingKey = $state(null);

  async function loadAll() {
    config = await GetConfig();
    providers = await GetProviders() || [];
    searchTerms = await GetSearchTerms() || [];
    const s = await GetStats();
    appState.stats = s;
    appState.categoriesStats = await GetCategoryStats();
    appState.sourceUsage = await GetSourceStats();
    await loadStorage();
  }

  async function loadStorage() {
    try {
      storageItems = await GetStorageInfo() || [];
    } catch(e) { console.error(e); }
  }

  let totalStorage = $derived(storageItems.reduce((sum, i) => sum + (i.sizeBytes || 0), 0));

  async function clearItem(item) {
    if (!confirm(`Remove ${item.label}? This cannot be undone.`)) return;
    clearingKey = item.key;
    try {
      await ClearStorage(item.key);
      await loadStorage();
      const s = await GetStats();
      appState.stats = s;
      appState.wallpapers = [];
    } catch(e) { console.error(e); }
    clearingKey = null;
  }

  async function clearAllStorage() {
    if (!confirm('Remove ALL stored data (database, cache, thumbnails, downloads and config)? This cannot be undone.')) return;
    clearingKey = 'all';
    try {
      await ClearAllStorage();
      await loadStorage();
      const s = await GetStats();
      appState.stats = s;
      appState.wallpapers = [];
    } catch(e) { console.error(e); }
    clearingKey = null;
  }

  async function clearAllCache() {
    await CleanupCache();
    const s = await GetStats();
    appState.stats = s;
  }

  async function changeCacheDir() {
    const dir = await SelectFolder();
    if (dir) loadAll();
  }

  async function addTerm() {
    const t = newTerm.trim().toLowerCase();
    if (!t || searchTerms.includes(t)) return;
    const ok = await AddSearchTerm(t);
    if (ok) { searchTerms = await GetSearchTerms(); newTerm = ''; }
  }

  async function removeTerm(term) {
    const ok = await RemoveSearchTerm(term);
    if (ok) searchTerms = await GetSearchTerms();
  }

  async function toggleProvider(name) {
    await ToggleSource(name);
    loadAll();
  }

  async function handleReset() {
    if (!confirm('Reset database and config to defaults? This will delete ALL wallpapers, cache, and downloads.')) return;
    await ResetDatabase();
    loadAll();
    appState.wallpapers = [];
  }

  async function handleMaxCacheChange(e) {
    const v = parseInt(e.target.value);
    if (v > 0) { await SetMaxCacheSizeMB(v); loadAll(); }
  }

  async function handleConcurrencyChange(e) {
    const v = parseInt(e.target.value);
    if (v > 0) { await SetConcurrentDownloads(v); loadAll(); }
  }

  loadAll();
</script>

<div class="p-6 overflow-y-auto h-full">
  <h2 class="text-lg font-semibold text-zinc-100 mb-5">Settings</h2>

  <div class="space-y-8">

    <!-- Stats -->
    <div class="space-y-3">
      <h3 class="text-sm font-medium text-zinc-300 uppercase tracking-wide">Stats</h3>
      <div class="grid grid-cols-2 gap-3">
        <div class="bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-3">
          <div class="text-2xl font-semibold text-zinc-100">{appState.stats.total || 0}</div>
          <div class="text-xs text-zinc-500">Total</div>
        </div>
        <div class="bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-3">
          <div class="text-2xl font-semibold text-zinc-100">{appState.stats.scraped || 0}</div>
          <div class="text-xs text-zinc-500">Scraped</div>
        </div>
        <div class="bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-3">
          <div class="text-2xl font-semibold text-zinc-100">{appState.stats.downloaded || 0}</div>
          <div class="text-xs text-zinc-500">Downloaded</div>
        </div>
        <div class="bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-3">
          <div class="text-2xl font-semibold text-zinc-100">{appState.stats.favorites || 0}</div>
          <div class="text-xs text-zinc-500">Favorites</div>
        </div>
      </div>
    </div>

    <!-- Cache -->
    <div class="space-y-3">
      <h3 class="text-sm font-medium text-zinc-300 uppercase tracking-wide">Cache</h3>
      <p class="text-sm text-zinc-400">Location: {config.cacheDir || 'default'}</p>
      <p class="text-sm text-zinc-400">Size: {Math.round(appState.stats.cacheSizeMB || 0)} MB</p>
      <div class="flex items-center gap-3">
        <label class="text-xs text-zinc-500">Max (MB):</label>
        <input type="number" value={config.maxCacheSizeMB || 5000} onchange={handleMaxCacheChange}
          class="w-24 bg-zinc-800 border border-zinc-700 rounded px-2 py-1 text-sm text-zinc-100 outline-none focus:border-zinc-500" />
      </div>
      <div class="flex gap-2">
        <button type="button" class="px-3 py-1.5 rounded-md bg-zinc-800 text-zinc-300 text-sm hover:bg-zinc-700 cursor-pointer transition-colors" use:click={changeCacheDir}>Change Cache Dir</button>
        <button type="button" class="px-3 py-1.5 rounded-md bg-red-900 text-red-100 text-sm hover:bg-red-800 cursor-pointer transition-colors" use:click={clearAllCache}>Clear All Cache</button>
      </div>
    </div>

    <!-- Storage -->
    <div class="space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-sm font-medium text-zinc-300 uppercase tracking-wide">Storage</h3>
        <span class="text-xs text-zinc-500">Total: {formatBytes(totalStorage)}</span>
      </div>
      <div class="space-y-1.5">
        {#each storageItems as item}
          <div class="flex items-center gap-3 bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-2.5">
            <div class="min-w-0 flex-1">
              <div class="text-sm text-zinc-200 truncate">{item.label}</div>
              <div class="text-xs text-zinc-500 truncate">{item.path}</div>
            </div>
            <div class="text-right shrink-0">
              <div class="text-sm text-zinc-100">{formatBytes(item.sizeBytes)}</div>
              <div class="text-xs text-zinc-500">{item.count} item{item.count === 1 ? '' : 's'}</div>
            </div>
            <button type="button" disabled={clearingKey !== null || (item.sizeBytes || 0) === 0}
              class="px-2.5 py-1 rounded-md bg-red-900/70 text-red-100 text-xs hover:bg-red-800 cursor-pointer transition-colors disabled:opacity-30 disabled:cursor-default"
              use:click={() => clearItem(item)}>{clearingKey === item.key ? '...' : 'Remove'}</button>
          </div>
        {/each}
      </div>
      <div class="flex justify-end">
        <button type="button" disabled={clearingKey !== null || storageItems.length === 0}
          class="px-3 py-1.5 rounded-md bg-red-900 text-red-100 text-sm font-medium hover:bg-red-800 cursor-pointer transition-colors disabled:opacity-30 disabled:cursor-default"
          use:click={clearAllStorage}>{clearingKey === 'all' ? 'Clearing...' : 'Remove All'}</button>
      </div>
    </div>

    <!-- Sources -->
    <div class="space-y-3">
      <h3 class="text-sm font-medium text-zinc-300 uppercase tracking-wide">Sources</h3>
      <div class="space-y-1.5">
        {#each providers as name}
          <label class="flex items-center gap-2.5 cursor-pointer">
            <input type="checkbox" checked={(config.enabledSources || []).includes(name)} onchange={() => toggleProvider(name)}
              class="accent-zinc-100 w-3.5 h-3.5" />
            <span class="text-sm text-zinc-300 capitalize">{name}</span>
          </label>
        {/each}
      </div>
      <div class="flex items-center gap-3">
        <label class="text-xs text-zinc-500">Concurrent:</label>
        <input type="number" value={config.concurrentDl || 10} onchange={handleConcurrencyChange}
          class="w-20 bg-zinc-800 border border-zinc-700 rounded px-2 py-1 text-sm text-zinc-100 outline-none focus:border-zinc-500" />
      </div>
    </div>

    <!-- Categories / Search Terms -->
    <div class="space-y-3">
      <h3 class="text-sm font-medium text-zinc-300 uppercase tracking-wide">Search Terms</h3>
      <p class="text-xs text-zinc-500">These terms are used when scraping for wallpapers.</p>
      <div class="flex gap-2">
        <input type="text" placeholder="Add term..." bind:value={newTerm}
          onkeydown={(e) => e.key === 'Enter' && addTerm()}
          class="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-1.5 text-sm text-zinc-100 placeholder-zinc-500 outline-none focus:border-zinc-500" />
        <button type="button" class="px-3 py-1.5 rounded-md bg-zinc-800 text-zinc-300 text-sm hover:bg-zinc-700 cursor-pointer transition-colors" use:click={addTerm}>Add</button>
      </div>
      <div class="flex flex-wrap gap-1.5">
        {#each searchTerms as term}
          <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-zinc-800 text-zinc-300 text-xs">
            {term}
            <button type="button" class="text-zinc-500 hover:text-red-400 cursor-pointer" use:click={() => removeTerm(term)} aria-label="Remove {term}">
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M18 6 6 18M6 6l12 12"/></svg>
            </button>
          </span>
        {/each}
      </div>
    </div>

    <!-- Danger Zone -->
    <div class="space-y-3 border border-red-900/40 rounded-lg p-4">
      <h3 class="text-sm font-medium text-red-400 uppercase tracking-wide">Danger Zone</h3>
      <p class="text-xs text-zinc-500">Reset everything to factory defaults. This cannot be undone.</p>
      <button type="button" class="px-4 py-2 rounded-md bg-red-900 text-red-100 text-sm font-medium hover:bg-red-800 cursor-pointer transition-colors" use:click={handleReset}>Reset Database &amp; Config</button>
    </div>

  </div>
</div>
