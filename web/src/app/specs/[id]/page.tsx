"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiFetch, timeAgo, severityColor } from "@/lib/utils";
import { ArrowLeft, FileCode2, AlertTriangle, Loader2 } from "lucide-react";

export default function SpecDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [spec, setSpec] = useState<any>(null);
  const [changes, setChanges] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [s, c] = await Promise.all([
          apiFetch(`/specs/${id}`),
          apiFetch<{ changes: any[] }>(`/specs/${id}/changes`),
        ]);
        setSpec(s);
        setChanges(c.changes || []);
      } catch {}
      setLoading(false);
    }
    load();
  }, [id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!spec) {
    return <div className="p-8 text-center text-muted-foreground">Spec not found</div>;
  }

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/specs"><ArrowLeft className="h-4 w-4" /></Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <FileCode2 className="h-5 w-5 text-blue-600" />
            {spec.spec_type} — v{spec.version}
          </h1>
          <p className="text-sm text-muted-foreground">
            {spec.branch} · {spec.commit_hash?.slice(0, 12)} · Repo #{spec.repo_id}
          </p>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-4">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-xs text-muted-foreground">Type</CardTitle></CardHeader>
          <CardContent><Badge variant="outline">{spec.spec_type}</Badge></CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-xs text-muted-foreground">Branch</CardTitle></CardHeader>
          <CardContent><p className="text-sm font-medium">{spec.branch || "—"}</p></CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-xs text-muted-foreground">Content Hash</CardTitle></CardHeader>
          <CardContent><p className="text-xs font-mono truncate">{spec.content_hash}</p></CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-xs text-muted-foreground">Changes</CardTitle></CardHeader>
          <CardContent><p className="text-2xl font-bold">{changes.length}</p></CardContent>
        </Card>
      </div>

      <Tabs defaultValue="changes">
        <TabsList>
          <TabsTrigger value="changes" className="gap-1">
            <AlertTriangle className="h-3.5 w-3.5" /> Changes ({changes.length})
          </TabsTrigger>
          <TabsTrigger value="content" className="gap-1">
            <FileCode2 className="h-3.5 w-3.5" /> Content
          </TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
        </TabsList>

        <TabsContent value="changes">
          {changes.length === 0 ? (
            <Card className="border-dashed mt-4">
              <CardContent className="py-8 text-center text-muted-foreground">
                No changes detected for this spec
              </CardContent>
            </Card>
          ) : (
            <div className="space-y-2 mt-4">
              {changes.map((c) => (
                <Link
                  key={c.id}
                  href={`/changes/${c.id}`}
                  className="flex items-start justify-between gap-3 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium">{c.path}</p>
                    <p className="text-xs text-muted-foreground truncate">{c.description}</p>
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    <Badge className={severityColor(c.change_type)} variant="outline">{c.change_type}</Badge>
                    {c.impact_score > 0 && (
                      <Badge variant="secondary" className="text-xs">Score: {c.impact_score}</Badge>
                    )}
                  </div>
                </Link>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="content">
          <Card className="mt-4">
            <CardContent className="p-0">
              <pre className="text-xs p-4 overflow-x-auto max-h-[600px] overflow-y-auto bg-muted/30 rounded-lg">
                {spec.content ? JSON.stringify(spec.content, null, 2) : "Content not loaded"}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="metadata">
          <Card className="mt-4">
            <CardContent className="p-4">
              <pre className="text-xs bg-muted p-4 rounded-lg overflow-x-auto">
                {spec.metadata ? JSON.stringify(spec.metadata, null, 2) : "No metadata"}
              </pre>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
