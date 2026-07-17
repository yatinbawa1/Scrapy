<script>
  import { cn } from "../../utils.js";

  let {
    class: className = "",
    variant = "default",
    children,
    ...restProps
  } = $props();

  const variants = {
    default: "border-transparent bg-zinc-800 text-zinc-100",
    secondary: "border-transparent bg-zinc-700 text-zinc-200",
    destructive: "border-transparent bg-red-900 text-red-100",
    outline: "text-zinc-300 border-zinc-700",
  };
</script>

<div
  class={cn(
    "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold transition-colors",
    variants[variant],
    className
  )}
  {...restProps}
>
  {@render children()}
</div>
