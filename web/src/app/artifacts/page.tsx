"use client";

import { useEffect, useState } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiFetch, timeAgo } from "@/lib/utils";
import { Package, Loader2, Search, Download, FileArchive, FileText, Code2 } from "lucide-react";

function artifactIcon(type: string) {
  switch (type) {
    case "sdk":
      return <Code2 className="h-4 w-4 text-indigo-500" />;
    case "docs":
      return <FileText className="h-4 w-4 text-blue-500" />;
    default:
      return <FileArchive className="h-4 w-4 text-gray-500" />;
  }
}

function formatBytes(bytes: number): string {
  if (!bytes) return "—";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

export default function ArtifactsPage() {
  const [artifacts, setArtifacts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    async function load() {
      try {
        const data = await apiFetch<{ artifacts: any[] }>("/artifacts");
        setArtifacts(data.artifacts || []);
      } catch {}
      setLoading(false);
    }
    load();
  }, []);

  const filtered = artifacts.filter(
    (a) =>
      !filter ||
      a.artifact_type?.toLowerCase().includes(filter.toLowerCase()) ||
      a.language?.toLowerCase().includes(filter.toLowerCase()) ||
      a.format?.toLowerCase().includes(filter.toLowerCase()) ||
      a.version?.toLowerCase().includes(filter.toLowerCase())
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Artifacts</h1>
        <p className="text-muted-foreground mt-1">Generated SDKs, docs, changelogs, and other deliverables</p>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Filter by type, language, format..."
          className="pl-9"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {filtered.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <Package className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">
              {artifacts.length === 0 ? "No artifacts generated yet" : "No artifacts match your filter"}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border bg-white">
          <div className="grid grid-cols-[1fr_100px_80px_80px_100px_60px] gap-4 p-3 border-b bg-muted/30 text-xs font-medium text-muted-foreground uppercase tracking-wider">
            <span>Type / Version</span>
            <span>Language</span>
            <span>Format</span>
            <span>Size</span>
            <span>Created</span>
            <span></span>
          </div>
          {filtered.map((artifact) => (
            <div
              key={artifact.id}
              className="grid grid-cols-[1fr_100px_80px_80px_100px_60px] gap-4 p-3 border-b last:border-0 hover:bg-muted/30 transition-colors items-center"
            >
              <div className="flex items-center gap-2 min-w-0">
                {artifactIcon(artifact.artifact_type)}
                <div className="min-w-0">
                  <span className="text-sm font-medium">{artifact.artifact_type}</span>
                  {artifact.version && (
                    <span className="text-xs text-muted-foreground ml-2">v{artifact.version}</span>
                  )}
                  <p className="text-xs text-muted-foreground truncate">Spec #{artifact.spec_id}</p>
                </div>
              </div>
              <Badge variant="outline" className="text-xs w-fit">{artifact.language || "—"}</Badge>
              <span className="text-xs text-muted-foreground">{artifact.format || "—"}</span>
              <span className="text-xs text-muted-foreground">{formatBytes(artifact.size_bytes)}</span>
              <span className="text-xs text-muted-foreground">{timeAgo(artifact.created_at)}</span>
              <Button variant="ghost" size="icon" className="h-8 w-8" asChild>
                <a href={`/api/v1/artifacts/${artifact.id}/download`} download>
                  <Download className="h-4 w-4" />
                </a>
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
