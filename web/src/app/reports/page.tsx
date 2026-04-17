"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { apiFetch, timeAgo } from "@/lib/utils";
import {
  Loader2, ShieldAlert, Scale, FileCheck, Waypoints, Sparkles,
  ClipboardList, Brain, Search,
} from "lucide-react";
import { Input } from "@/components/ui/input";

const reportTypeConfig: Record<string, { icon: any; color: string; bg: string }> = {
  risk: { icon: ShieldAlert, color: "text-red-600", bg: "bg-red-50" },
  standards: { icon: Scale, color: "text-amber-600", bg: "bg-amber-50" },
  doc_consistency: { icon: FileCheck, color: "text-blue-600", bg: "bg-blue-50" },
  protocol_recommendation: { icon: Waypoints, color: "text-purple-600", bg: "bg-purple-50" },
  enrichment: { icon: Sparkles, color: "text-emerald-600", bg: "bg-emerald-50" },
  summary: { icon: ClipboardList, color: "text-gray-600", bg: "bg-gray-50" },
  knowledge_model: { icon: Brain, color: "text-indigo-600", bg: "bg-indigo-50" },
};

export default function ReportsPage() {
  const [reports, setReports] = useState<any[]>([]);
  const [repos, setRepos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [generating, setGenerating] = useState<number | null>(null);
  const [filter, setFilter] = useState("");

  const load = async () => {
    try {
      const [rptData, repoData] = await Promise.all([
        apiFetch<{ reports: any[] }>("/reports"),
        apiFetch<{ repositories: any[] }>("/repositories"),
      ]);
      setReports(rptData.reports || []);
      setRepos(repoData.repositories || []);
    } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const handleGenerate = async (repoId: number) => {
    setGenerating(repoId);
    try {
      const res = await apiFetch<any>(`/repositories/${repoId}/generate-reports`, { method: "POST" });
      const { toast } = await import("sonner");
      toast.success(`Imported ${res.count} reports for ${repos.find((r) => r.id === repoId)?.name}`);
      load();
    } catch (err: any) {
      const { toast } = await import("sonner");
      toast.error(err.message);
    } finally {
      setGenerating(null);
    }
  };

  const repoName = (id: number) => repos.find((r) => r.id === id)?.name || `#${id}`;

  const filtered = reports.filter(
    (r) =>
      !filter ||
      r.title?.toLowerCase().includes(filter.toLowerCase()) ||
      r.report_type?.toLowerCase().includes(filter.toLowerCase())
  );

  // Group by report type for stats
  const typeCounts: Record<string, number> = {};
  for (const r of reports) {
    typeCounts[r.report_type] = (typeCounts[r.report_type] || 0) + 1;
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Reports</h1>
          <p className="text-muted-foreground mt-1">Risk, standards, recommendations, doc consistency, enrichment reports</p>
        </div>
      </div>

      {/* Generate buttons per repo */}
      {repos.filter((r) => r.local_path).length > 0 && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Generate Reports</CardTitle>
            <CardDescription>Import reports from local repository .specguard/out/ directories</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {repos.filter((r) => r.local_path).map((repo) => (
                <Button
                  key={repo.id}
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  onClick={() => handleGenerate(repo.id)}
                  disabled={generating === repo.id}
                >
                  {generating === repo.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <Sparkles className="h-3 w-3" />}
                  {repo.name}
                </Button>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Type stat badges */}
      {Object.keys(typeCounts).length > 0 && (
        <div className="flex flex-wrap gap-2">
          {Object.entries(typeCounts).map(([type, count]) => {
            const cfg = reportTypeConfig[type] || reportTypeConfig.summary;
            const Icon = cfg.icon;
            return (
              <Badge key={type} variant="outline" className={`gap-1.5 px-3 py-1.5 ${cfg.color}`}>
                <Icon className="h-3.5 w-3.5" /> {type.replace(/_/g, " ")} ({count})
              </Badge>
            );
          })}
        </div>
      )}

      {/* Filter */}
      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input placeholder="Filter by title or type..." className="pl-9" value={filter} onChange={(e) => setFilter(e.target.value)} />
      </div>

      {/* Report list */}
      {filtered.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <ClipboardList className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">
              {reports.length === 0
                ? "No reports yet. Add a repository with a local path and click Generate Reports."
                : "No reports match your filter"}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((report) => {
            const cfg = reportTypeConfig[report.report_type] || reportTypeConfig.summary;
            const Icon = cfg.icon;
            return (
              <Link key={report.id} href={`/reports/${report.id}`}>
                <Card className="group hover:shadow-md hover:-translate-y-0.5 transition-all duration-200 cursor-pointer h-full">
                  <CardHeader className="pb-3">
                    <div className="flex items-start justify-between">
                      <div className={`flex h-10 w-10 items-center justify-center rounded-xl ${cfg.bg} ${cfg.color} transition-transform group-hover:scale-110`}>
                        <Icon className="h-5 w-5" />
                      </div>
                      <Badge variant="outline" className="text-xs">{report.report_type.replace(/_/g, " ")}</Badge>
                    </div>
                    <CardTitle className="text-sm mt-2">{report.title}</CardTitle>
                    <CardDescription className="text-xs line-clamp-2">{report.summary || "View full report"}</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span>Repo: {repoName(report.repo_id)}</span>
                      <span>{timeAgo(report.created_at)}</span>
                    </div>
                  </CardContent>
                </Card>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
