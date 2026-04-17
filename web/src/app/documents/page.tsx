"use client";

import { useEffect, useState, useRef } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogDescription,
} from "@/components/ui/dialog";
import { apiFetch, timeAgo } from "@/lib/utils";
import { FileText, Upload, Loader2, Trash2, ChevronDown, ChevronUp, Search } from "lucide-react";
import { toast } from "sonner";

export default function DocumentsPage() {
  const [docs, setDocs] = useState<any[]>([]);
  const [repos, setRepos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [expandedDoc, setExpandedDoc] = useState<number | null>(null);
  const [chunks, setChunks] = useState<any[]>([]);
  const [filter, setFilter] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);

  const load = async () => {
    try {
      const [docData, repoData] = await Promise.all([
        apiFetch<{ documents: any[] }>("/documents"),
        apiFetch<{ repositories: any[] }>("/repositories"),
      ]);
      setDocs(docData.documents || []);
      setRepos(repoData.repositories || []);
    } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const handleUpload = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const file = fileRef.current?.files?.[0];
    if (!file) { toast.error("Select a file"); return; }
    setUploading(true);
    try {
      const body = new FormData();
      body.append("file", file);
      body.append("repo_id", fd.get("repo_id") as string);
      body.append("doc_type", (fd.get("doc_type") as string) || "");
      const res = await fetch("/api/v1/documents/upload", { method: "POST", body });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      toast.success(`Uploaded ${file.name} — ${data.chunk_count} chunks created`);
      setUploadOpen(false);
      load();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await apiFetch(`/documents/${id}`, { method: "DELETE" });
      toast.success("Document deleted");
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const toggleChunks = async (docId: number) => {
    if (expandedDoc === docId) {
      setExpandedDoc(null);
      setChunks([]);
      return;
    }
    try {
      const data = await apiFetch<{ chunks: any[] }>(`/documents/${docId}/chunks`);
      setChunks(data.chunks || []);
      setExpandedDoc(docId);
    } catch {
      toast.error("Failed to load chunks");
    }
  };

  const filtered = docs.filter(
    (d) => !filter || d.filename?.toLowerCase().includes(filter.toLowerCase()) || d.doc_type?.toLowerCase().includes(filter.toLowerCase())
  );

  const repoName = (id: number) => repos.find((r) => r.id === id)?.name || `#${id}`;

  const formatBytes = (b: number) => {
    if (b < 1024) return `${b} B`;
    if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
    return `${(b / (1024 * 1024)).toFixed(1)} MB`;
  };

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
          <h1 className="text-3xl font-bold tracking-tight">Documents</h1>
          <p className="text-muted-foreground mt-1">Upload domain docs (PDFs, markdown, text) for RAG context enrichment</p>
        </div>
        <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2"><Upload className="h-4 w-4" /> Upload Document</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Upload Document</DialogTitle>
              <DialogDescription>Upload a PDF, markdown, or text file. It will be chunked and stored for RAG context.</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleUpload} className="space-y-4">
              <div className="space-y-2">
                <Label>File</Label>
                <Input type="file" ref={fileRef} accept=".pdf,.md,.txt,.json,.yaml,.yml" required />
              </div>
              <div className="space-y-2">
                <Label>Repository</Label>
                <select name="repo_id" required className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
                  <option value="">Select a repository...</option>
                  {repos.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
                </select>
              </div>
              <div className="space-y-2">
                <Label>Document Type</Label>
                <select name="doc_type" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm">
                  <option value="">Auto-detect</option>
                  <option value="pdf">PDF</option>
                  <option value="markdown">Markdown</option>
                  <option value="text">Plain Text</option>
                  <option value="json">JSON</option>
                  <option value="yaml">YAML</option>
                </select>
              </div>
              <Button type="submit" className="w-full" disabled={uploading}>
                {uploading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Upload & Chunk
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {/* Stats */}
      <div className="grid gap-4 grid-cols-3">
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{docs.length}</div>
            <p className="text-xs text-muted-foreground">Documents</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{docs.reduce((sum, d) => sum + (d.chunk_count || 0), 0)}</div>
            <p className="text-xs text-muted-foreground">Total Chunks</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="text-2xl font-bold">{formatBytes(docs.reduce((sum, d) => sum + (d.size_bytes || 0), 0))}</div>
            <p className="text-xs text-muted-foreground">Total Size</p>
          </CardContent>
        </Card>
      </div>

      {/* Filter */}
      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input placeholder="Filter by filename or type..." className="pl-9" value={filter} onChange={(e) => setFilter(e.target.value)} />
      </div>

      {/* Document list */}
      {filtered.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <FileText className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">{docs.length === 0 ? "No documents uploaded yet. Upload PDFs, design docs, or API guides for RAG context." : "No documents match your filter"}</p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {filtered.map((doc) => (
            <Card key={doc.id}>
              <CardContent className="py-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-blue-50 text-blue-600">
                      <FileText className="h-5 w-5" />
                    </div>
                    <div>
                      <p className="font-medium text-sm">{doc.filename}</p>
                      <div className="flex items-center gap-2 mt-0.5">
                        <Badge variant="outline" className="text-xs">{doc.doc_type}</Badge>
                        <span className="text-xs text-muted-foreground">{formatBytes(doc.size_bytes)}</span>
                        <span className="text-xs text-muted-foreground">{doc.chunk_count} chunks</span>
                        <span className="text-xs text-muted-foreground">Repo: {repoName(doc.repo_id)}</span>
                        <span className="text-xs text-muted-foreground">{timeAgo(doc.created_at)}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="ghost" size="sm" onClick={() => toggleChunks(doc.id)}>
                      {expandedDoc === doc.id ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
                      Chunks
                    </Button>
                    <Button variant="ghost" size="sm" className="text-red-500 hover:text-red-700" onClick={() => handleDelete(doc.id)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {expandedDoc === doc.id && chunks.length > 0 && (
                  <div className="mt-4 border-t pt-4 space-y-2 max-h-80 overflow-y-auto">
                    {chunks.map((chunk) => (
                      <div key={chunk.id} className="bg-muted/30 rounded p-3 text-xs font-mono">
                        <div className="flex items-center justify-between mb-1">
                          <Badge variant="secondary" className="text-xs">Chunk #{chunk.chunk_index}</Badge>
                          <span className="text-muted-foreground">{chunk.char_count} chars</span>
                        </div>
                        <pre className="whitespace-pre-wrap text-xs max-h-32 overflow-y-auto">{chunk.content?.slice(0, 500)}{chunk.content?.length > 500 ? "..." : ""}</pre>
                      </div>
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
