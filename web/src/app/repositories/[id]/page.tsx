"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { apiFetch, timeAgo } from "@/lib/utils";
import { ArrowLeft, GitBranch, FileCode2, Loader2 } from "lucide-react";

export default function RepositoryDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [repo, setRepo] = useState<any>(null);
  const [specs, setSpecs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const [r, s] = await Promise.all([
          apiFetch(`/repositories/${id}`),
          apiFetch<{ specs: any[] }>(`/specs?repo_id=${id}`),
        ]);
        setRepo(r);
        setSpecs(s.specs || []);
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

  if (!repo) {
    return (
      <div className="p-8 text-center text-muted-foreground">Repository not found</div>
    );
  }

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/repositories"><ArrowLeft className="h-4 w-4" /></Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <GitBranch className="h-5 w-5 text-emerald-600" />
            {repo.name}
          </h1>
          <p className="text-sm text-muted-foreground">{repo.url}</p>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">Created</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm font-medium">{new Date(repo.created_at).toLocaleString()}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm text-muted-foreground">Specs</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{specs.length}</p>
          </CardContent>
        </Card>
      </div>

      {repo.config && Object.keys(repo.config).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Configuration</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="text-xs bg-muted p-4 rounded-lg overflow-x-auto">
              {JSON.stringify(repo.config, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <FileCode2 className="h-4 w-4 text-blue-500" />
            Specs ({specs.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {specs.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4 text-center">No specs for this repository</p>
          ) : (
            <div className="space-y-2">
              {specs.map((spec) => (
                <Link
                  key={spec.id}
                  href={`/specs/${spec.id}`}
                  className="flex items-center justify-between gap-2 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                >
                  <div>
                    <p className="text-sm font-medium">{spec.spec_type} — v{spec.version}</p>
                    <p className="text-xs text-muted-foreground">{spec.branch} · {spec.commit_hash?.slice(0, 8)}</p>
                  </div>
                  <span className="text-xs text-muted-foreground">{timeAgo(spec.created_at)}</span>
                </Link>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
