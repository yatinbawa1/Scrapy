<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    type = "text",
    value = $bindable(""),
    placeholder = "",
    disabled = false,
    ...restProps
  } = $props();
</script>

<input
  {type}
  bind:value
  {placeholder}
  {disabled}
  class={cn(
    "flex h-10 w-full rounded-md border border-zinc-700 bg-zinc-800/50 px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-900 disabled:cursor-not-allowed disabled:opacity-50",
    className
  )}
  {...restProps}
/>
