"use client";

import { useEffect, useState, useRef } from "react";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { FileCode2, Loader2, Search, Upload } from "lucide-react";
import { toast } from "sonner";

export default function SpecsPage() {
  const [specs, setSpecs] = useState<any[]>([]);
  const [repos, setRepos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = async () => {
    try {
      const [specData, repoData] = await Promise.all([
        apiFetch<{ specs: any[] }>("/specs"),
        apiFetch<{ repositories: any[] }>("/repositories"),
      ]);
      setSpecs(specData.specs || []);
      setRepos(repoData.repositories || []);
    } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const handleUpload = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const file = fileRef.current?.files?.[0];
    if (!file) {
      toast.error("Please select a file");
      return;
    }
    setUploading(true);
    try {
      const body = new FormData();
      body.append("file", file);
      body.append("repo_id", fd.get("repo_id") as string);
      body.append("version", (fd.get("version") as string) || "1.0.0");
      body.append("branch", (fd.get("branch") as string) || "");
      body.append("spec_type", (fd.get("spec_type") as string) || "");
      const res = await fetch("/api/v1/specs/upload", { method: "POST", body });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text);
      }
      toast.success(`Uploaded ${file.name}`);
      setUploadOpen(false);
      load();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setUploading(false);
    }
  };

  const filtered = specs.filter(
    (s) =>
      !filter ||
      s.spec_type?.toLowerCase().includes(filter.toLowerCase()) ||
      s.branch?.toLowerCase().includes(filter.toLowerCase()) ||
      s.version?.toLowerCase().includes(filter.toLowerCase())
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Specs</h1>
          <p className="text-muted-foreground mt-1">Normalized API specification snapshots</p>
        </div>
        <Dialog open={uploadOpen} onOpenChange={setUploadOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2">
              <Upload className="h-4 w-4" /> Upload Spec
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Upload Spec File</DialogTitle>
              <DialogDescription>Upload an OpenAPI JSON or Proto definition file.</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleUpload} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="file">Spec File</Label>
                <Input id="file" type="file" ref={fileRef} accept=".json,.yaml,.yml,.proto" required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="repo_id">Repository</Label>
                <select
                  name="repo_id"
                  required
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="">Select a repository...</option>
                  {repos.map((r) => (
                    <option key={r.id} value={r.id}>{r.name}</option>
                  ))}
                </select>
                {repos.length === 0 && (
                  <p className="text-xs text-amber-600">Create a repository first on the Repositories page.</p>
                )}
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="version">Version</Label>
                  <Input id="version" name="version" placeholder="1.0.0" defaultValue="1.0.0" />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="branch">Branch</Label>
                  <Input id="branch" name="branch" placeholder="main" />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="spec_type">Spec Type</Label>
                <select
                  name="spec_type"
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                >
                  <option value="">Auto-detect</option>
                  <option value="openapi">OpenAPI</option>
                  <option value="proto">Protobuf</option>
                  <option value="asyncapi">AsyncAPI</option>
                </select>
              </div>
              <Button type="submit" className="w-full" disabled={uploading}>
                {uploading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Upload
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Filter by type, branch, version..."
          className="pl-9"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {filtered.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <FileCode2 className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">
              {specs.length === 0 ? "No specs loaded yet" : "No specs match your filter"}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="rounded-lg border bg-white">
          <div className="grid grid-cols-[1fr_100px_120px_100px_80px] gap-4 p-3 border-b bg-muted/30 text-xs font-medium text-muted-foreground uppercase tracking-wider">
            <span>Type / Version</span>
            <span>Branch</span>
            <span>Commit</span>
            <span>Created</span>
            <span>Repo</span>
          </div>
          {filtered.map((spec) => (
            <Link
              key={spec.id}
              href={`/specs/${spec.id}`}
              className="grid grid-cols-[1fr_100px_120px_100px_80px] gap-4 p-3 border-b last:border-0 hover:bg-muted/30 transition-colors items-center"
            >
              <div className="flex items-center gap-2">
                <FileCode2 className="h-4 w-4 text-blue-500 shrink-0" />
                <div>
                  <span className="text-sm font-medium">{spec.spec_type}</span>
                  <span className="text-xs text-muted-foreground ml-2">v{spec.version}</span>
                </div>
              </div>
              <Badge variant="outline" className="text-xs w-fit">{spec.branch || "—"}</Badge>
              <span className="text-xs font-mono text-muted-foreground truncate">
                {spec.commit_hash?.slice(0, 12) || "—"}
              </span>
              <span className="text-xs text-muted-foreground">{timeAgo(spec.created_at)}</span>
              <Badge variant="secondary" className="text-xs w-fit">#{spec.repo_id}</Badge>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
