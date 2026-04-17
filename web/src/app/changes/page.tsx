"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { apiFetch, timeAgo, severityColor } from "@/lib/utils";
import { AlertTriangle, Loader2, Search, ShieldAlert, ShieldCheck, ShieldMinus } from "lucide-react";

export default function ChangesPage() {
  const [changes, setChanges] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    async function load() {
      try {
        const data = await apiFetch<{ changes: any[] }>("/changes");
        setChanges(data.changes || []);
      } catch {}
      setLoading(false);
    }
    load();
  }, []);

  const filtered = changes.filter(
    (c) =>
      !filter ||
      c.path?.toLowerCase().includes(filter.toLowerCase()) ||
      c.change_type?.toLowerCase().includes(filter.toLowerCase()) ||
      c.classification?.toLowerCase().includes(filter.toLowerCase()) ||
      c.description?.toLowerCase().includes(filter.toLowerCase())
  );

  const breaking = changes.filter((c) => c.change_type === "breaking").length;
  const deprecation = changes.filter((c) => c.change_type === "deprecation").length;
  const nonBreaking = changes.filter((c) => c.change_type !== "breaking" && c.change_type !== "deprecation").length;

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
        <h1 className="text-3xl font-bold tracking-tight">Changes</h1>
        <p className="text-muted-foreground mt-1">API drift detection — breaking, deprecation, and additions</p>
      </div>

      {/* Summary stats */}
      <div className="grid gap-3 grid-cols-3 max-w-lg">
        <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50/50 p-3">
          <ShieldAlert className="h-4 w-4 text-red-600" />
          <div>
            <p className="text-lg font-bold text-red-700">{breaking}</p>
            <p className="text-xs text-red-600">Breaking</p>
          </div>
        </div>
        <div className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50/50 p-3">
          <ShieldMinus className="h-4 w-4 text-amber-600" />
          <div>
            <p className="text-lg font-bold text-amber-700">{deprecation}</p>
            <p className="text-xs text-amber-600">Deprecation</p>
          </div>
        </div>
        <div className="flex items-center gap-2 rounded-lg border border-emerald-200 bg-emerald-50/50 p-3">
          <ShieldCheck className="h-4 w-4 text-emerald-600" />
          <div>
            <p className="text-lg font-bold text-emerald-700">{nonBreaking}</p>
            <p className="text-xs text-emerald-600">Non-breaking</p>
          </div>
        </div>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Filter changes..."
          className="pl-9"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {filtered.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <AlertTriangle className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">
              {changes.length === 0 ? "No changes detected yet" : "No changes match your filter"}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border bg-white">
          <div className="grid grid-cols-[1fr_120px_120px_80px_80px] gap-4 p-3 border-b bg-muted/30 text-xs font-medium text-muted-foreground uppercase tracking-wider">
            <span>Path / Description</span>
            <span>Type</span>
            <span>Classification</span>
            <span>Impact</span>
            <span>Time</span>
          </div>
          {filtered.map((change) => (
            <Link
              key={change.id}
              href={`/changes/${change.id}`}
              className="grid grid-cols-[1fr_120px_120px_80px_80px] gap-4 p-3 border-b last:border-0 hover:bg-muted/30 transition-colors items-center"
            >
              <div className="min-w-0">
                <p className="text-sm font-medium truncate">{change.path}</p>
                <p className="text-xs text-muted-foreground truncate">{change.description}</p>
              </div>
              <Badge className={severityColor(change.change_type)} variant="outline">
                {change.change_type}
              </Badge>
              <span className="text-xs text-muted-foreground truncate">{change.classification}</span>
              <span className="text-sm font-semibold">{change.impact_score}</span>
              <span className="text-xs text-muted-foreground">{timeAgo(change.created_at)}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
