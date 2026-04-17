"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { apiFetch, timeAgo, severityColor } from "@/lib/utils";
import {
  Activity,
  GitBranch,
  FileCode2,
  AlertTriangle,
  Package,
  CheckCircle2,
  XCircle,
  Loader2,
} from "lucide-react";
import Link from "next/link";

interface HealthData {
  status: string;
  version: string;
  timestamp: string;
}

export default function DashboardPage() {
  const [health, setHealth] = useState<HealthData | null>(null);
  const [repos, setRepos] = useState<any[]>([]);
  const [specs, setSpecs] = useState<any[]>([]);
  const [changes, setChanges] = useState<any[]>([]);
  const [artifacts, setArtifacts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    async function load() {
      try {
        const [h, r, s, c, a] = await Promise.allSettled([
          apiFetch<HealthData>("/health"),
          apiFetch<{ repositories: any[] }>("/repositories"),
          apiFetch<{ specs: any[] }>("/specs"),
          apiFetch<{ changes: any[] }>("/changes"),
          apiFetch<{ artifacts: any[] }>("/artifacts"),
        ]);
        if (h.status === "fulfilled") setHealth(h.value);
        if (r.status === "fulfilled") setRepos(r.value.repositories || []);
        if (s.status === "fulfilled") setSpecs(s.value.specs || []);
        if (c.status === "fulfilled") setChanges(c.value.changes || []);
        if (a.status === "fulfilled") setArtifacts(a.value.artifacts || []);
      } catch (e: any) {
        setError(e.message);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  const breakingCount = changes.filter((c) => c.change_type === "breaking").length;
  const nonBreakingCount = changes.filter((c) => c.change_type !== "breaking").length;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  return (
    <div className="p-6 lg:p-8 space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">
          <span className="bg-gradient-to-r from-primary to-indigo-600 bg-clip-text text-transparent">
            Dashboard
          </span>
        </h1>
        <p className="mt-1 text-muted-foreground">
          API change guardrail overview
        </p>
      </div>

      {/* Health Banner */}
      <Card className={health?.status === "healthy" ? "border-emerald-200 bg-emerald-50/50" : "border-red-200 bg-red-50/50"}>
        <CardContent className="flex items-center gap-3 py-4">
          {health?.status === "healthy" ? (
            <CheckCircle2 className="h-5 w-5 text-emerald-600" />
          ) : (
            <XCircle className="h-5 w-5 text-red-600" />
          )}
          <span className="text-sm font-medium">
            {health ? `Control plane is ${health.status} — v${health.version}` : "Unable to reach backend"}
          </span>
          {error && <span className="text-xs text-red-600 ml-auto">{error}</span>}
        </CardContent>
      </Card>

      {/* Stats Grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatsCard
          title="Repositories"
          value={repos.length}
          icon={GitBranch}
          href="/repositories"
          color="text-emerald-600 bg-emerald-50"
        />
        <StatsCard
          title="Specs"
          value={specs.length}
          icon={FileCode2}
          href="/specs"
          color="text-blue-600 bg-blue-50"
        />
        <StatsCard
          title="Changes"
          value={changes.length}
          icon={AlertTriangle}
          href="/changes"
          color="text-amber-600 bg-amber-50"
          subtitle={breakingCount > 0 ? `${breakingCount} breaking` : undefined}
        />
        <StatsCard
          title="Artifacts"
          value={artifacts.length}
          icon={Package}
          href="/artifacts"
          color="text-purple-600 bg-purple-50"
        />
      </div>

      {/* Recent Activity */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Recent Changes */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-amber-500" />
              Recent Changes
            </CardTitle>
          </CardHeader>
          <CardContent>
            {changes.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">No changes detected yet</p>
            ) : (
              <div className="space-y-3">
                {changes.slice(0, 5).map((change) => (
                  <Link
                    key={change.id}
                    href={`/changes/${change.id}`}
                    className="flex items-start justify-between gap-2 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium truncate">{change.path}</p>
                      <p className="text-xs text-muted-foreground truncate">{change.description}</p>
                    </div>
                    <Badge className={severityColor(change.change_type)} variant="outline">
                      {change.change_type}
                    </Badge>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Recent Specs */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-semibold flex items-center gap-2">
              <FileCode2 className="h-4 w-4 text-blue-500" />
              Recent Specs
            </CardTitle>
          </CardHeader>
          <CardContent>
            {specs.length === 0 ? (
              <p className="text-sm text-muted-foreground py-4 text-center">No specs loaded yet</p>
            ) : (
              <div className="space-y-3">
                {specs.slice(0, 5).map((spec) => (
                  <Link
                    key={spec.id}
                    href={`/specs/${spec.id}`}
                    className="flex items-start justify-between gap-2 rounded-lg border p-3 hover:bg-muted/50 transition-colors"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium">{spec.spec_type} — v{spec.version}</p>
                      <p className="text-xs text-muted-foreground">
                        {spec.branch} · {spec.commit_hash?.slice(0, 8)}
                      </p>
                    </div>
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {timeAgo(spec.created_at)}
                    </span>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function StatsCard({
  title,
  value,
  icon: Icon,
  href,
  color,
  subtitle,
}: {
  title: string;
  value: number;
  icon: any;
  href: string;
  color: string;
  subtitle?: string;
}) {
  return (
    <Link href={href}>
      <Card className="group hover:shadow-md hover:-translate-y-0.5 transition-all duration-200 cursor-pointer">
        <CardContent className="flex items-center gap-4 p-6">
          <div className={`flex h-12 w-12 items-center justify-center rounded-xl ${color} transition-transform group-hover:scale-110`}>
            <Icon className="h-6 w-6" />
          </div>
          <div>
            <p className="text-3xl font-bold">{value}</p>
            <p className="text-sm text-muted-foreground">{title}</p>
            {subtitle && <p className="text-xs text-red-500 mt-0.5">{subtitle}</p>}
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
