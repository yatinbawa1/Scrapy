<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    children,
    ...restProps
  } = $props();
</script>

<p class={cn("text-sm text-zinc-400", className)} {...restProps}>
  {@render children()}
</p>
