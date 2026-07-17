<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    children,
    ...restProps
  } = $props();
</script>

<div class={cn("p-6 pt-0", className)} {...restProps}>
  {@render children()}
</div>
