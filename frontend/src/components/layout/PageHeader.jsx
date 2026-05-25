import {cn} from "../../lib/utils";

export function PageHeader({title, description, eyebrow, action, className, ...props}) {
  return (
    <div className={cn("flex flex-col gap-4 border-b border-border pb-6 lg:flex-row lg:items-end lg:justify-between", className)} {...props}>
      <div className="space-y-2">
        {eyebrow ? <p className="text-xs font-semibold uppercase tracking-normal text-primary">{eyebrow}</p> : null}
        <h2 className="text-2xl font-bold tracking-normal text-foreground">{title}</h2>
        {description ? <p className="max-w-3xl text-sm leading-6 text-muted-foreground">{description}</p> : null}
      </div>
      {action ? <div className="flex shrink-0 items-center gap-2">{action}</div> : null}
    </div>
  );
}

