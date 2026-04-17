"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogDescription,
} from "@/components/ui/dialog";
import { apiFetch, timeAgo } from "@/lib/utils";
import { GitBranch, Plus, Loader2, ExternalLink, FolderOpen, ScanSearch } from "lucide-react";
import { toast } from "sonner";

export default function RepositoriesPage() {
  const [repos, setRepos] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [scanning, setScanning] = useState<number | null>(null);

  const load = async () => {
    try {
      const data = await apiFetch<{ repositories: any[] }>("/repositories");
      setRepos(data.repositories || []);
    } catch {
      toast.error("Failed to load repositories");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleCreate = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setCreating(true);
    const fd = new FormData(e.currentTarget);
    try {
      const res = await apiFetch<any>("/repositories", {
        method: "POST",
        body: JSON.stringify({
          name: fd.get("name"),
          url: fd.get("url") || "",
          local_path: fd.get("local_path") || "",
        }),
      });
      toast.success("Repository created");
      setOpen(false);
      await load();
      // Auto-scan if local path was provided
      if (fd.get("local_path") && res.id) {
        handleScan(res.id);
      }
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleScan = async (repoId: number) => {
    setScanning(repoId);
    try {
      const res = await apiFetch<any>(`/repositories/${repoId}/scan`, { method: "POST" });
      toast.success(`Imported ${res.count} spec(s) from local path`);
      load();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setScanning(null);
    }
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
          <h1 className="text-3xl font-bold tracking-tight">Repositories</h1>
          <p className="text-muted-foreground mt-1">Manage tracked API repositories</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2">
              <Plus className="h-4 w-4" /> Add Repository
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Repository</DialogTitle>
              <DialogDescription>Register a new API repository for SpecGuard to track.</DialogDescription>
            </DialogHeader>
            <form onSubmit={handleCreate} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="name">Repository Name</Label>
                <Input id="name" name="name" placeholder="my-api-project" required />
              </div>
              <div className="space-y-2">
                <Label htmlFor="local_path">Local Path</Label>
                <Input id="local_path" name="local_path" placeholder="/path/to/your/api-project" />
                <p className="text-xs text-muted-foreground">Path to the repo on disk. If provided, SpecGuard will auto-import specs from <code>.specguard/out/</code></p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="url">Repository URL <span className="text-muted-foreground font-normal">(optional)</span></Label>
                <Input id="url" name="url" placeholder="https://github.com/org/repo" />
              </div>
              <Button type="submit" className="w-full" disabled={creating}>
                {creating ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Create Repository
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </div>

      {repos.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-12">
            <GitBranch className="h-12 w-12 text-muted-foreground/40 mb-4" />
            <p className="text-muted-foreground">No repositories registered yet</p>
            <Button variant="outline" className="mt-4 gap-2" onClick={() => setOpen(true)}>
              <Plus className="h-4 w-4" /> Add your first repository
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {repos.map((repo) => (
            <Link key={repo.id} href={`/repositories/${repo.id}`}>
              <Card className="group hover:shadow-md hover:-translate-y-0.5 transition-all duration-200 cursor-pointer h-full">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 transition-transform group-hover:scale-110">
                      <GitBranch className="h-5 w-5" />
                    </div>
                    <ExternalLink className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                  </div>
                  <CardTitle className="text-base mt-2">{repo.name}</CardTitle>
                  <CardDescription className="text-xs truncate">{repo.local_path || repo.url}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-2">
                  {repo.local_path && (
                    <div className="flex items-center gap-1.5 text-xs text-emerald-600">
                      <FolderOpen className="h-3 w-3" /> Local
                    </div>
                  )}
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span>Created {timeAgo(repo.created_at)}</span>
                    {repo.local_path && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="h-6 text-xs gap-1 px-2"
                        onClick={(e) => { e.preventDefault(); handleScan(repo.id); }}
                        disabled={scanning === repo.id}
                      >
                        {scanning === repo.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <ScanSearch className="h-3 w-3" />}
                        Scan
                      </Button>
                    )}
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
