import {cn} from "../../lib/utils";

export function PageContainer({className, children, ...props}) {
  return (
    <div className={cn("mx-auto w-full max-w-7xl space-y-6 p-6 lg:p-8", className)} {...props}>
      {children}
    </div>
  );
}

