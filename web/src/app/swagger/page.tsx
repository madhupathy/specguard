"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiFetch } from "@/lib/utils";
import { Loader2, Search, ChevronDown, ChevronUp, FileJson } from "lucide-react";

const methodColors: Record<string, string> = {
  get: "bg-emerald-100 text-emerald-700",
  post: "bg-blue-100 text-blue-700",
  put: "bg-amber-100 text-amber-700",
  patch: "bg-orange-100 text-orange-700",
  delete: "bg-red-100 text-red-700",
};

export default function SwaggerPage() {
  const [repos, setRepos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedRepo, setSelectedRepo] = useState<number | null>(null);
  const [swagger, setSwagger] = useState<any>(null);
  const [loadingSwagger, setLoadingSwagger] = useState(false);
  const [filter, setFilter] = useState("");
  const [expandedPath, setExpandedPath] = useState<string | null>(null);

  useEffect(() => {
    async function load() {
      try {
        const data = await apiFetch<{ repositories: any[] }>("/repositories");
        setRepos(data.repositories || []);
        if (data.repositories?.length > 0) {
          loadSwagger(data.repositories[0].id);
          setSelectedRepo(data.repositories[0].id);
        }
      } catch {}
      setLoading(false);
    }
    load();
  }, []);

  const loadSwagger = async (repoId: number) => {
    setLoadingSwagger(true);
    setSwagger(null);
    setSelectedRepo(repoId);
    try {
      const data = await apiFetch<any>(`/repositories/${repoId}/swagger`);
      setSwagger(data.swagger);
    } catch {}
    setLoadingSwagger(false);
  };

  // Extract paths from OpenAPI spec
  const paths = swagger?.paths || {};
  const info = swagger?.info || {};
  const servers = swagger?.servers || [];

  // Flatten into a list of {path, method, operation}
  const endpoints: { path: string; method: string; op: any }[] = [];
  for (const [path, methods] of Object.entries(paths)) {
    for (const [method, op] of Object.entries(methods as any)) {
      if (["get", "post", "put", "patch", "delete", "options", "head"].includes(method)) {
        endpoints.push({ path, method, op: op as any });
      }
    }
  }

  const filtered = endpoints.filter(
    (e) =>
      !filter ||
      e.path.toLowerCase().includes(filter.toLowerCase()) ||
      e.method.toLowerCase().includes(filter.toLowerCase()) ||
      (e.op?.summary || "").toLowerCase().includes(filter.toLowerCase()) ||
      (e.op?.operationId || "").toLowerCase().includes(filter.toLowerCase())
  );

  // Group by tag
  const grouped: Record<string, typeof endpoints> = {};
  for (const ep of filtered) {
    const tag = ep.op?.tags?.[0] || "default";
    if (!grouped[tag]) grouped[tag] = [];
    grouped[tag].push(ep);
  }

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
        <h1 className="text-3xl font-bold tracking-tight">Swagger / OpenAPI</h1>
        <p className="text-muted-foreground mt-1">Browse the normalized OpenAPI specification</p>
      </div>

      {/* Repo selector */}
      <div className="flex items-center gap-3">
        <span className="text-sm font-medium">Repository:</span>
        <div className="flex gap-2">
          {repos.map((r) => (
            <Button
              key={r.id}
              variant={selectedRepo === r.id ? "default" : "outline"}
              size="sm"
              onClick={() => loadSwagger(r.id)}
            >
              {r.name}
            </Button>
          ))}
        </div>
      </div>

      {loadingSwagger && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      )}

      {!loadingSwagger && !swagger && (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <FileJson className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">No OpenAPI spec found. Add a repo and scan it first.</p>
          </CardContent>
        </Card>
      )}

      {swagger && (
        <>
          {/* API info */}
          <Card>
            <CardContent className="py-4">
              <div className="flex items-center justify-between">
                <div>
                  <h2 className="text-lg font-semibold">{info.title || "API"}</h2>
                  {info.description && <p className="text-sm text-muted-foreground mt-1">{info.description}</p>}
                </div>
                <div className="flex items-center gap-2">
                  {info.version && <Badge variant="outline">v{info.version}</Badge>}
                  <Badge variant="secondary">{endpoints.length} endpoints</Badge>
                </div>
              </div>
              {servers.length > 0 && (
                <div className="mt-2">
                  {servers.map((s: any, i: number) => (
                    <code key={i} className="text-xs bg-muted px-2 py-1 rounded mr-2">{s.url}</code>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Filter */}
          <div className="relative max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Filter endpoints by path, method, summary..."
              className="pl-9"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
          </div>

          {/* Endpoints grouped by tag */}
          <div className="space-y-4">
            {Object.entries(grouped).map(([tag, eps]) => (
              <Card key={tag}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium capitalize">{tag}</CardTitle>
                  <CardDescription className="text-xs">{eps.length} endpoint{eps.length > 1 ? "s" : ""}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-1 pt-0">
                  {eps.map((ep) => {
                    const key = `${ep.method}:${ep.path}`;
                    const isExpanded = expandedPath === key;
                    return (
                      <div key={key}>
                        <button
                          className="w-full flex items-center gap-3 px-3 py-2 rounded hover:bg-muted/30 transition-colors text-left"
                          onClick={() => setExpandedPath(isExpanded ? null : key)}
                        >
                          <Badge className={`${methodColors[ep.method] || "bg-gray-100 text-gray-700"} font-mono text-xs uppercase w-16 justify-center`}>
                            {ep.method}
                          </Badge>
                          <code className="text-sm flex-1 truncate">{ep.path}</code>
                          <span className="text-xs text-muted-foreground truncate max-w-xs hidden sm:block">
                            {ep.op?.summary || ep.op?.operationId || ""}
                          </span>
                          {isExpanded ? <ChevronUp className="h-4 w-4 shrink-0" /> : <ChevronDown className="h-4 w-4 shrink-0" />}
                        </button>
                        {isExpanded && (
                          <div className="ml-20 mb-3 p-3 bg-muted/20 rounded text-xs space-y-2">
                            {ep.op?.summary && <p className="font-medium">{ep.op.summary}</p>}
                            {ep.op?.description && <p className="text-muted-foreground">{ep.op.description}</p>}
                            {ep.op?.operationId && (
                              <div><span className="text-muted-foreground">operationId:</span> <code className="bg-muted px-1 rounded">{ep.op.operationId}</code></div>
                            )}
                            {ep.op?.parameters && ep.op.parameters.length > 0 && (
                              <div>
                                <p className="font-medium mb-1">Parameters:</p>
                                <table className="w-full">
                                  <thead>
                                    <tr className="border-b">
                                      <th className="text-left py-1 px-2">Name</th>
                                      <th className="text-left py-1 px-2">In</th>
                                      <th className="text-left py-1 px-2">Required</th>
                                      <th className="text-left py-1 px-2">Type</th>
                                    </tr>
                                  </thead>
                                  <tbody>
                                    {ep.op.parameters.map((p: any, pi: number) => (
                                      <tr key={pi} className="border-b last:border-0">
                                        <td className="py-1 px-2 font-mono">{p.name}</td>
                                        <td className="py-1 px-2">{p.in}</td>
                                        <td className="py-1 px-2">{p.required ? "yes" : "no"}</td>
                                        <td className="py-1 px-2 font-mono">{p.schema?.type || "—"}</td>
                                      </tr>
                                    ))}
                                  </tbody>
                                </table>
                              </div>
                            )}
                            {ep.op?.responses && (
                              <div>
                                <p className="font-medium mb-1">Responses:</p>
                                <div className="flex flex-wrap gap-1">
                                  {Object.entries(ep.op.responses).map(([code, resp]: [string, any]) => (
                                    <Badge key={code} variant="outline" className="text-xs">
                                      {code}: {resp?.description || ""}
                                    </Badge>
                                  ))}
                                </div>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </CardContent>
              </Card>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
