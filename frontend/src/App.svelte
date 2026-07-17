<script>
  import Sidebar from './lib/components/Sidebar.svelte';
  import Grid from './lib/components/Grid.svelte';
  import Preview from './lib/components/Preview.svelte';
  import Settings from './lib/components/Settings.svelte';
  import Onboarding from './lib/components/Onboarding.svelte';
  import LoadingOverlay from './lib/components/LoadingOverlay.svelte';
  import { appState } from './lib/global-state.svelte.js';
  import { IsFirstRun } from '../wailsjs/go/main/App.js';
  import { onMount } from 'svelte';

  let ready = $state(false);

  onMount(async () => {
    console.log('[App] onMount');
    if (!window.go?.main?.App) {
      console.warn('[App] Wails Go bridge not found.');
    }
    try {
      const first = await IsFirstRun();
      appState.isFirstRun = first;
    } catch(e) { console.error('[App] IsFirstRun error:', e); }
    ready = true;
  });
</script>

{#if !ready}
  <div class="splash">
    <div class="splash-spinner"></div>
  </div>
{:else if appState.isFirstRun}
  <Onboarding />
{:else}
  <div class="app">
    <Sidebar />
    <div class="main-content">
      {#key appState.view}
        {#if appState.view === 'settings'}
          <Settings />
        {:else}
          <Grid />
        {/if}
      {/key}
    </div>
    {#if appState.previewOpen}
      <Preview />
    {/if}
  </div>
{/if}

<LoadingOverlay />

<style>
  .splash { position: fixed; inset: 0; background: #0a0a0a; display: flex; align-items: center; justify-content: center; }
  .splash-spinner { width: 32px; height: 32px; border: 3px solid #27272a; border-top-color: #d4d4d8; border-radius: 50%; animation: spin 0.8s linear infinite; }
  .app { display: flex; height: 100vh; overflow: hidden; background: #0a0a0a; color: #e4e4e7; }
  .main-content { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .debug-bar { position: fixed; bottom: 0; left: 0; right: 0; background: #111; color: #0f0; font-family: monospace; font-size: 11px; padding: 4px 8px; z-index: 9999; border-top: 1px solid #333; }
  .debug-bar b { color: #fff; }
</style>
