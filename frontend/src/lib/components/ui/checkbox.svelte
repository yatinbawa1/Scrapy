<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    checked = $bindable(false),
    disabled = false,
    label = "",
    ...restProps
  } = $props();
</script>

<label class="flex items-center gap-3 cursor-pointer {disabled ? 'opacity-50 cursor-not-allowed' : ''}">
  <div class="relative">
    <input
      type="checkbox"
      bind:checked
      {disabled}
      class="peer sr-only"
      {...restProps}
    />
    <div class="h-5 w-5 shrink-0 rounded-md border border-zinc-700 bg-zinc-800 peer-focus-visible:ring-2 peer-focus-visible:ring-zinc-400 peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-zinc-900 peer-checked:bg-zinc-100 peer-checked:border-zinc-100 transition-colors">
      {#if checked}
        <svg class="h-full w-full text-zinc-900" viewBox="0 0 16 16" fill="none">
          <path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2-2a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z" fill="currentColor"/>
        </svg>
      {/if}
    </div>
  </div>
  {#if label}
    <span class="text-sm text-zinc-300 select-none">{label}</span>
  {/if}
</label>
