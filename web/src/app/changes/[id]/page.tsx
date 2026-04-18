"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { apiFetch, severityColor, changeKindLabel, severityIcon } from "@/lib/utils";

interface ChangeDetail {
  id: number;
  from_spec_id: number | null;
  to_spec_id: number | null;
  change_type: string;
  classification: string;
  path: string;
  description: string | null;
  ai_summary: string | null;
  impact_score: number;
  metadata: string | null;
  created_at: string;
}

export default function ChangeDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [change, setChange] = useState<ChangeDetail | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;
    apiFetch<ChangeDetail>(`/changes/${id}`)
      .then(setChange)
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!change) {
    return (
      <div className="p-8">
        <p className="text-muted-foreground">Change not found.</p>
        <Link href="/changes" className="text-primary text-sm mt-2 inline-block">← Back to Changes</Link>
      </div>
    );
  }

  let metadata: Record<string, string> = {};
  try {
    if (change.metadata) metadata = JSON.parse(change.metadata);
  } catch {}

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div>
        <Link href="/changes" className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground mb-3">
          <ArrowLeft className="h-4 w-4" /> Back to Changes
        </Link>
        <div className="flex items-start gap-3">
          <span className="text-2xl">{severityIcon(change.change_type)}</span>
          <div>
            <h1 className="text-2xl font-bold">{changeKindLabel(change.classification || change.change_type)}</h1>
            <p className="text-muted-foreground font-mono text-sm mt-1">{change.path}</p>
          </div>
        </div>
      </div>

      <div className="flex gap-3 flex-wrap">
        <Badge className={severityColor(change.change_type)} variant="outline">
          {change.change_type}
        </Badge>
        {change.classification && change.classification !== change.change_type && (
          <Badge variant="secondary">{change.classification}</Badge>
        )}
        <Badge variant="outline">Impact: {change.impact_score}</Badge>
      </div>

      {change.description && (
        <Card>
          <CardHeader><CardTitle className="text-sm font-semibold">Description</CardTitle></CardHeader>
          <CardContent><p className="text-sm">{change.description}</p></CardContent>
        </Card>
      )}

      {change.ai_summary && (
        <Card className="border-blue-200 bg-blue-50/30">
          <CardHeader><CardTitle className="text-sm font-semibold text-blue-700">AI Summary</CardTitle></CardHeader>
          <CardContent><p className="text-sm text-blue-800">{change.ai_summary}</p></CardContent>
        </Card>
      )}

      {Object.keys(metadata).length > 0 && (
        <Card>
          <CardHeader><CardTitle className="text-sm font-semibold">Change Details</CardTitle></CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-2">
              {Object.entries(metadata).map(([k, v]) => (
                <div key={k} className="text-sm">
                  <span className="font-medium text-muted-foreground">{k}: </span>
                  <span className="font-mono">{v}</span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="text-xs text-muted-foreground">
        Detected: {new Date(change.created_at).toLocaleString()}
      </div>
    </div>
  );
}
