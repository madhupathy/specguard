"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { apiFetch, severityColor } from "@/lib/utils";
import { ArrowLeft, AlertTriangle, Loader2, Sparkles } from "lucide-react";

export default function ChangeDetailPage() {
  const params = useParams();
  const id = params.id as string;
  const [change, setChange] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const data = await apiFetch(`/changes/${id}`);
        setChange(data);
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

  if (!change) {
    return <div className="p-8 text-center text-muted-foreground">Change not found</div>;
  }

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild>
          <Link href="/changes"><ArrowLeft className="h-4 w-4" /></Link>
        </Button>
        <div className="flex-1">
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <AlertTriangle className="h-5 w-5 text-amber-500" />
            Change #{change.id}
          </h1>
          <p className="text-sm text-muted-foreground truncate">{change.path}</p>
        </div>
        <Badge className={`${severityColor(change.change_type)} text-sm px-3 py-1`} variant="outline">
          {change.change_type}
        </Badge>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Classification</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm font-medium">{change.classification}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Impact Score</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">{change.impact_score}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs text-muted-foreground">Spec IDs</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm">
              <span className="text-muted-foreground">From:</span>{" "}
              <Link href={`/specs/${change.from_spec_id}`} className="text-primary hover:underline">#{change.from_spec_id}</Link>
              {" → "}
              <Link href={`/specs/${change.to_spec_id}`} className="text-primary hover:underline">#{change.to_spec_id}</Link>
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Path</CardTitle>
        </CardHeader>
        <CardContent>
          <code className="text-sm bg-muted px-3 py-1.5 rounded-md block">{change.path}</code>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Description</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm leading-relaxed">{change.description || "No description available"}</p>
        </CardContent>
      </Card>

      {change.ai_summary && (
        <Card className="border-purple-200 bg-purple-50/30">
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Sparkles className="h-4 w-4 text-purple-600" />
              AI Summary
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm leading-relaxed">{change.ai_summary}</p>
          </CardContent>
        </Card>
      )}

      {change.metadata && Object.keys(change.metadata).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Metadata</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="text-xs bg-muted p-4 rounded-lg overflow-x-auto">
              {JSON.stringify(change.metadata, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
