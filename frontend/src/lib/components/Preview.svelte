<script>
  import { appState, downloadStart, downloadDone, downloadFail, downloadIsActive } from '../global-state.svelte.js';
  import { DownloadWallpaper, SetWallpaper, ToggleFavorite } from '../../../wailsjs/go/main/App.js';
  import { click } from '../actions.js';
  import { onMount } from 'svelte';

  let imgLoaded = $state(false);
  let imgError = $state(false);
  let navLoading = $state(false);
  let imgEl;

  function currentIndex() {
    return appState.wallpapers.findIndex(w => w.id === appState.currentWallpaper?.id);
  }

  function loadImage(url) {
    imgLoaded = false;
    imgError = false;
    navLoading = true;
    if (imgEl) {
      imgEl.onload = null;
      imgEl.onerror = null;
      imgEl.src = '';
    }
    requestAnimationFrame(() => {
      if (imgEl) {
        imgEl.onload = () => { imgLoaded = true; imgError = false; navLoading = false; };
        imgEl.onerror = () => { imgError = true; navLoading = false; };
        imgEl.src = url;
      }
    });
  }

  function prev() {
    const idx = currentIndex();
    if (idx <= 0) return;
    appState.currentWallpaper = appState.wallpapers[idx - 1];
  }

  function next() {
    const idx = currentIndex();
    if (idx >= appState.wallpapers.length - 1) return;
    appState.currentWallpaper = appState.wallpapers[idx + 1];
  }

  function close() {
    appState.previewOpen = false;
  }

  async function doSave() {
    const w = appState.currentWallpaper;
    if (!w || w.status === 'downloaded' || downloadIsActive(w.id)) return;
    downloadStart(w.id, 'preview');
    try {
      await DownloadWallpaper(w.id);
    } catch(e) {
      console.error('[Preview] download:', e);
      downloadFail(w.id, e.message || 'download failed');
    }
  }

  async function doDownloadAndSet() {
    const w = appState.currentWallpaper;
    if (!w) return;
    if (w.status !== 'downloaded' && !downloadIsActive(w.id)) {
      downloadStart(w.id, 'set');
    }
    try {
      await SetWallpaper(w.id);
    } catch(e) {
      console.error('[Preview] set:', e);
    }
  }

  async function toggleFav() {
    const w = appState.currentWallpaper;
    if (!w) return;
    try {
      await ToggleFavorite(w.id);
      appState.wallpapers = appState.wallpapers.map(item =>
        item.id === w.id ? { ...item, isFavorite: !item.isFavorite } : item
      );
      appState.currentWallpaper = { ...appState.currentWallpaper, isFavorite: !appState.currentWallpaper.isFavorite };
    } catch(e) { console.error(e); }
  }

  function handleKeydown(e) {
    if (e.key === 'ArrowLeft') { e.preventDefault(); prev(); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); next(); }
    else if (e.key === 'Escape') { e.preventDefault(); close(); }
  }

  $effect(() => {
    if (appState.previewOpen && appState.currentWallpaper) {
      const url = appState.currentWallpaper.url || appState.currentWallpaper.thumbnailUrl;
      if (url) loadImage(url);
    }
  });

  onMount(() => {
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  });
</script>

{#if appState.previewOpen && appState.currentWallpaper}
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="fixed inset-0 z-50 flex flex-col bg-black/80" use:click={close} role="presentation">
    <div class="absolute inset-0 -z-10 bg-cover bg-center blur-xl opacity-30"
      style="background-image: url({appState.currentWallpaper.thumbnailUrl || appState.currentWallpaper.url})">
    </div>

    <!-- top bar -->
    <div class="flex items-center justify-between shrink-0 px-4 h-14 bg-black/40 backdrop-blur-sm border-b border-zinc-800/50">
      <div class="flex items-center gap-3">
        <button type="button" class="p-1.5 rounded-md text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 cursor-pointer transition-colors" aria-label="Close" use:click={(e) => { e.stopPropagation(); close(); }}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18M6 6l12 12"/></svg>
        </button>
        <span class="text-zinc-400 text-sm">{appState.currentWallpaper.source || 'unknown'}</span>
      </div>

      <div class="flex items-center gap-2">
        <button type="button"
          class="p-1.5 rounded-md transition-colors cursor-pointer {appState.currentWallpaper.isFavorite ? 'text-red-400 hover:text-red-300' : 'text-zinc-400 hover:text-zinc-100'}"
          aria-label="Toggle favorite" use:click={(e) => { e.stopPropagation(); toggleFav(); }}>
          <svg width="16" height="16" viewBox="0 0 24 24" fill={appState.currentWallpaper.isFavorite ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="2">
            <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
          </svg>
        </button>

        {#if appState.currentWallpaper.status === 'downloaded'}
          <span class="flex items-center gap-1 text-xs text-green-400 bg-green-500/10 px-2 py-1 rounded">Downloaded</span>
        {:else if downloadIsActive(appState.currentWallpaper.id)}
          <span class="flex items-center gap-1 text-xs text-blue-400 bg-blue-500/10 px-2 py-1 rounded">
            <span class="w-2.5 h-2.5 border-2 border-blue-400 border-t-transparent rounded-full animate-spin"></span>
            Saving
          </span>
        {:else}
          <button type="button"
            class="px-3 py-1.5 rounded-md bg-zinc-800 text-zinc-200 text-sm hover:bg-zinc-700 cursor-pointer transition-colors"
            use:click={(e) => { e.stopPropagation(); doSave(); }}>Save</button>
        {/if}

        <button type="button"
          class="px-3 py-1.5 rounded-md bg-zinc-100 text-zinc-900 text-sm font-medium hover:bg-zinc-200 cursor-pointer transition-colors"
          use:click={(e) => { e.stopPropagation(); doDownloadAndSet(); }}>Set as Wallpaper</button>
      </div>
    </div>

    <!-- main image area -->
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="flex-1 flex items-center justify-center relative min-h-0" use:click={(e) => e.stopPropagation()}>
      {#if currentIndex() > 0}
        <button type="button"
          class="absolute left-4 top-1/2 -translate-y-1/2 p-2 rounded-full bg-black/40 text-zinc-300 hover:bg-black/60 hover:text-zinc-100 cursor-pointer transition-colors z-20"
          aria-label="Previous" use:click={(e) => { e.stopPropagation(); prev(); }}>
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg>
        </button>
      {/if}

      <!-- thumbnail placeholder -->
      <img
        src={appState.currentWallpaper.thumbnailUrl}
        alt=""
        class="absolute inset-0 w-full h-full object-contain px-16 transition-opacity duration-300 {imgLoaded ? 'opacity-0' : 'opacity-100'}"
        draggable="false"
      />

      <!-- full image -->
      <img
        bind:this={imgEl}
        alt=""
        class="relative max-h-full max-w-full object-contain px-16 select-none transition-opacity duration-300 {imgLoaded ? 'opacity-100' : 'opacity-0'}"
        draggable="false"
      />

      <!-- loading spinner -->
      {#if navLoading && !imgLoaded && !imgError}
        <div class="absolute inset-0 flex items-center justify-center">
          <div class="flex flex-col items-center gap-2">
            <div class="w-8 h-8 border-2 border-zinc-500 border-t-zinc-100 rounded-full animate-spin"></div>
            <span class="text-xs text-zinc-400">Loading image...</span>
          </div>
        </div>
      {/if}

      <!-- error fallback -->
      {#if imgError}
        <div class="absolute inset-0 flex items-center justify-center">
          <p class="text-sm text-zinc-500">Failed to load full image</p>
        </div>
      {/if}

      {#if currentIndex() < appState.wallpapers.length - 1}
        <button type="button"
          class="absolute right-4 top-1/2 -translate-y-1/2 p-2 rounded-full bg-black/40 text-zinc-300 hover:bg-black/60 hover:text-zinc-100 cursor-pointer transition-colors z-20"
          aria-label="Next" use:click={(e) => { e.stopPropagation(); next(); }}>
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
        </button>
      {/if}
    </div>

    <!-- bottom bar -->
    <div class="flex items-center justify-center gap-4 shrink-0 px-4 h-12 bg-black/40 backdrop-blur-sm border-t border-zinc-800/50 text-xs text-zinc-500">
      <span>#{appState.currentWallpaper.id}</span>
      <span>{appState.currentWallpaper.width || '?'}x{appState.currentWallpaper.height || '?'}</span>
      {#if appState.currentWallpaper.category}
        <span>{appState.currentWallpaper.category}</span>
      {/if}
      <span>{currentIndex() + 1} / {appState.wallpapers.length}</span>
    </div>
  </div>
{/if}
