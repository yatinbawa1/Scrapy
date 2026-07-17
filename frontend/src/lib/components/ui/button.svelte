<script>
  import { cn } from "../../utils.js";
  import { click } from "../../actions.js";

  let {
    class: className = "",
    variant = "default",
    size = "default",
    disabled = false,
    type = "button",
    onclick = undefined,
    children,
    ...restProps
  } = $props();

  const variants = {
    default: "bg-zinc-100 text-zinc-900 hover:bg-zinc-200",
    destructive: "bg-red-900 text-red-50 hover:bg-red-800",
    outline: "border border-zinc-700 bg-transparent hover:bg-zinc-800 text-zinc-100",
    secondary: "bg-zinc-800 text-zinc-100 hover:bg-zinc-700",
    ghost: "hover:bg-zinc-800 text-zinc-400 hover:text-zinc-100",
    link: "text-zinc-400 underline-offset-4 hover:underline text-zinc-100",
  };

  const sizes = {
    default: "h-10 px-4 py-2",
    sm: "h-9 rounded-md px-3",
    lg: "h-11 rounded-md px-8",
    icon: "h-10 w-10",
  };
</script>

<button
  {type}
  {disabled}
  class={cn(
    "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 cursor-pointer",
    variants[variant],
    sizes[size],
    className
  )}
  use:click={(e) => {
    console.log('[UI Button] click fired, variant:', variant, 'disabled:', disabled);
    if (onclick) onclick(e);
  }}
  {...restProps}
>
  {@render children()}
</button>
