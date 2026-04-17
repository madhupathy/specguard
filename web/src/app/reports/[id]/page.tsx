"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiFetch, timeAgo } from "@/lib/utils";
import { Loader2, ArrowLeft, FileText, Code } from "lucide-react";
import Link from "next/link";

export default function ReportDetailPage() {
  const { id } = useParams();
  const [report, setReport] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function load() {
      try {
        const data = await apiFetch(`/reports/${id}`);
        setReport(data);
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

  if (!report) {
    return (
      <div className="p-6 lg:p-8">
        <p className="text-muted-foreground">Report not found</p>
      </div>
    );
  }

  const hasMarkdown = !!report.content_md;
  const hasJSON = !!report.content && typeof report.content === "object";

  return (
    <div className="p-6 lg:p-8 space-y-6">
      <div className="flex items-center gap-4">
        <Link href="/reports">
          <Button variant="ghost" size="sm"><ArrowLeft className="h-4 w-4 mr-1" /> Reports</Button>
        </Link>
      </div>

      <div>
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold tracking-tight">{report.title}</h1>
          <Badge variant="outline">{report.report_type?.replace(/_/g, " ")}</Badge>
        </div>
        <p className="text-sm text-muted-foreground mt-1">
          Repo #{report.repo_id} &middot; {timeAgo(report.created_at)}
        </p>
      </div>

      {report.summary && (
        <Card>
          <CardContent className="py-4">
            <pre className="whitespace-pre-wrap text-sm">{report.summary}</pre>
          </CardContent>
        </Card>
      )}

      <Tabs defaultValue={hasMarkdown ? "rendered" : "json"}>
        <TabsList>
          {hasMarkdown && (
            <TabsTrigger value="rendered" className="gap-2">
              <FileText className="h-4 w-4" /> Rendered
            </TabsTrigger>
          )}
          {hasJSON && (
            <TabsTrigger value="json" className="gap-2">
              <Code className="h-4 w-4" /> JSON
            </TabsTrigger>
          )}
          {hasMarkdown && (
            <TabsTrigger value="raw" className="gap-2">
              <Code className="h-4 w-4" /> Raw Markdown
            </TabsTrigger>
          )}
        </TabsList>

        {hasMarkdown && (
          <TabsContent value="rendered">
            <Card>
              <CardContent className="py-6 prose prose-sm max-w-none">
                <MarkdownRenderer content={report.content_md} />
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {hasJSON && (
          <TabsContent value="json">
            <Card>
              <CardContent className="py-4">
                <pre className="whitespace-pre-wrap text-xs font-mono bg-muted/30 rounded p-4 max-h-[600px] overflow-auto">
                  {JSON.stringify(report.content, null, 2)}
                </pre>
              </CardContent>
            </Card>
          </TabsContent>
        )}

        {hasMarkdown && (
          <TabsContent value="raw">
            <Card>
              <CardContent className="py-4">
                <pre className="whitespace-pre-wrap text-xs font-mono bg-muted/30 rounded p-4 max-h-[600px] overflow-auto">
                  {report.content_md}
                </pre>
              </CardContent>
            </Card>
          </TabsContent>
        )}
      </Tabs>
    </div>
  );
}

function MarkdownRenderer({ content }: { content: string }) {
  // Simple markdown to HTML renderer for tables, headings, bold, code, lists
  const lines = content.split("\n");
  const elements: JSX.Element[] = [];
  let i = 0;
  let key = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Table
    if (line.includes("|") && i + 1 < lines.length && lines[i + 1]?.match(/^\|[\s-|]+\|$/)) {
      const headers = line.split("|").filter(Boolean).map((h) => h.trim());
      i += 2; // skip header separator
      const rows: string[][] = [];
      while (i < lines.length && lines[i].includes("|")) {
        rows.push(lines[i].split("|").filter(Boolean).map((c) => c.trim()));
        i++;
      }
      elements.push(
        <div key={key++} className="overflow-x-auto my-4">
          <table className="w-full text-sm border-collapse">
            <thead>
              <tr className="border-b bg-muted/30">
                {headers.map((h, j) => (
                  <th key={j} className="px-3 py-2 text-left text-xs font-medium text-muted-foreground">{inlineFormat(h)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, ri) => (
                <tr key={ri} className="border-b last:border-0 hover:bg-muted/20">
                  {row.map((cell, ci) => (
                    <td key={ci} className="px-3 py-2 text-xs">{inlineFormat(cell)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
      continue;
    }

    // Headings
    if (line.startsWith("# ")) {
      elements.push(<h1 key={key++} className="text-xl font-bold mt-6 mb-2">{inlineFormat(line.slice(2))}</h1>);
      i++;
      continue;
    }
    if (line.startsWith("## ")) {
      elements.push(<h2 key={key++} className="text-lg font-semibold mt-5 mb-2">{inlineFormat(line.slice(3))}</h2>);
      i++;
      continue;
    }
    if (line.startsWith("### ")) {
      elements.push(<h3 key={key++} className="text-base font-semibold mt-4 mb-1">{inlineFormat(line.slice(4))}</h3>);
      i++;
      continue;
    }

    // List items
    if (line.match(/^[-*] /)) {
      elements.push(<li key={key++} className="ml-4 text-sm list-disc">{inlineFormat(line.slice(2))}</li>);
      i++;
      continue;
    }

    // Empty line
    if (line.trim() === "") {
      i++;
      continue;
    }

    // Regular paragraph
    elements.push(<p key={key++} className="text-sm mb-2">{inlineFormat(line)}</p>);
    i++;
  }

  return <>{elements}</>;
}

function inlineFormat(text: string): React.ReactNode {
  // Bold **text**
  const parts = text.split(/(\*\*[^*]+\*\*|`[^`]+`)/g);
  return parts.map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**")) {
      return <strong key={i}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith("`") && part.endsWith("`")) {
      return <code key={i} className="bg-muted px-1 py-0.5 rounded text-xs">{part.slice(1, -1)}</code>;
    }
    return part;
  });
}
