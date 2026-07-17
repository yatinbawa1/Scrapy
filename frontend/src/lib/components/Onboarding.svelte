<script>
  import { appState } from '../global-state.svelte.js';
  import { DismissOnboarding } from '../../../wailsjs/go/main/App.js';
  import { click } from '../actions.js';

  let busy = $state(false);

  async function close() {
    busy = true;
    try {
      await DismissOnboarding();
    } catch(e) {
      console.error('[Onboarding]', e);
    }
    appState.isFirstRun = false;
    busy = false;
  }
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/70" use:click={close}>
  <div class="bg-zinc-900 border border-zinc-800 rounded-xl max-w-md w-full mx-4 p-6 space-y-4" use:click={(e) => e.stopPropagation()}>
    <h2 class="text-xl font-semibold text-zinc-100">Welcome to Wallpaper Choser!</h2>
    <p class="text-zinc-400 text-sm">Browse, search, and download beautiful wallpapers.</p>
    <ul class="space-y-2 text-sm text-zinc-400 list-disc list-inside">
      <li>Use the sidebar to find wallpapers from multiple sources</li>
      <li>Click on a wallpaper to preview and download</li>
      <li>Favorite wallpapers to save them for later</li>
      <li>Manage your collection in the Downloads section</li>
    </ul>
    <button type="button" disabled={busy} class="px-4 py-2 rounded-md bg-zinc-100 text-zinc-900 text-sm font-medium hover:bg-zinc-200 cursor-pointer transition-colors disabled:opacity-50 disabled:cursor-default" use:click={close}>Get Started</button>
  </div>
</div>
