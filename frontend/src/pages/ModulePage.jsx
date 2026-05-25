import {ChevronRight} from "lucide-react";
import {Card, CardContent, CardHeader, CardTitle} from "../components/ui/Card";
import {Button} from "../components/ui/Button";
import {PageContainer} from "../components/layout/PageContainer";
import {PageHeader} from "../components/layout/PageHeader";
import {pageMeta, placeholderItems} from "../lib/iscc";

export function ModuleContent({pageKey}) {
  const meta = pageMeta[pageKey];

  return (
    <div className="grid gap-6 lg:grid-cols-[1.1fr_0.9fr]">
      <Card>
        <CardHeader>
          <CardTitle>{meta.title}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {placeholderItems.map((item) => (
            <Button key={item} variant="ghost" className="h-auto w-full justify-between rounded-md px-4 py-4">
              <span className="text-left text-sm">{item}</span>
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            </Button>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>数据接入位</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex min-h-[240px] items-center justify-center rounded-md border border-dashed border-border bg-muted/30 p-8 text-center text-sm text-muted-foreground">
            -
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function ModulePage({pageKey}) {
  const meta = pageMeta[pageKey];

  return (
    <PageContainer>
      <PageHeader eyebrow={meta.eyebrow} title={meta.title} />
      <ModuleContent pageKey={pageKey} />
    </PageContainer>
  );
}
