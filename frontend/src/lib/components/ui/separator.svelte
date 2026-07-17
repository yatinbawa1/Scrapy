<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    orientation = "horizontal",
    decorative = true,
    ...restProps
  } = $props();
</script>

<div
  role={decorative ? "none" : "separator"}
  aria-orientation={orientation}
  class={cn(
    "shrink-0 bg-zinc-800",
    orientation === "horizontal" ? "h-[1px] w-full" : "h-full w-[1px]",
    className
  )}
  {...restProps}
></div>
