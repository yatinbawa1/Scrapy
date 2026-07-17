<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    children,
    ...restProps
  } = $props();
</script>

<div
  class={cn(
    "rounded-xl border border-zinc-800 bg-zinc-900/50 text-zinc-100 shadow-sm backdrop-blur-sm",
    className
  )}
  {...restProps}
>
  {@render children()}
</div>
