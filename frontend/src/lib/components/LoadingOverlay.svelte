<script>
  import { appState } from '../global-state.svelte.js';
  import { click } from '../actions.js';

  function close() {
    appState.isScraping = false;
  }
</script>

{#if appState.isScraping}
  <div class="fixed inset-0 z-40 flex items-center justify-center bg-black/60" use:click={close}>
    <div class="flex flex-col items-center gap-3" use:click={(e) => e.stopPropagation()}>
      <div class="w-10 h-10 border-2 border-zinc-600 border-t-zinc-100 rounded-full animate-spin"></div>
      <p class="text-zinc-300 text-sm">Scraping wallpapers...</p>
      {#if appState.isFirstRun}
        <p class="text-zinc-500 text-xs">First run! This may take a while.</p>
      {/if}
    </div>
  </div>
{/if}
