"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiFetch } from "@/lib/utils";
import { Settings, CheckCircle2, XCircle, Loader2, Brain, GitBranch, Unplug, Plug, FlaskConical } from "lucide-react";
import { toast } from "sonner";

export default function SettingsPage() {
  const [health, setHealth] = useState<any>(null);
  const [connections, setConnections] = useState<any>({});
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState<string | null>(null);
  const [saving, setSaving] = useState<string | null>(null);

  // LLM form
  const [llmKey, setLlmKey] = useState("");
  const [llmModel, setLlmModel] = useState("gpt-4o-mini");
  const [llmBaseUrl, setLlmBaseUrl] = useState("https://api.openai.com/v1");

  // Git form
  const [gitPath, setGitPath] = useState("");
  const [gitToken, setGitToken] = useState("");

  const load = async () => {
    try {
      const [h, c] = await Promise.all([
        apiFetch("/health"),
        apiFetch<{ connections: any }>("/connections"),
      ]);
      setHealth(h);
      setConnections(c.connections || {});
      // Pre-fill from saved config
      const llmCfg = c.connections?.llm?.config;
      if (llmCfg) {
        if (llmCfg.model) setLlmModel(llmCfg.model);
        if (llmCfg.base_url) setLlmBaseUrl(llmCfg.base_url);
      }
      const gitCfg = c.connections?.git?.config;
      if (gitCfg) {
        if (gitCfg.path) setGitPath(gitCfg.path);
      }
    } catch {}
    setLoading(false);
  };

  useEffect(() => { load(); }, []);

  const handleTest = async (connector: string) => {
    setTesting(connector);
    try {
      const body = connector === "llm"
        ? { api_key: llmKey, model: llmModel, base_url: llmBaseUrl }
        : { path: gitPath, token: gitToken };
      const res = await apiFetch<any>(`/connections/${connector}/test`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      if (res.success) {
        toast.success(res.message);
      } else {
        toast.error(res.message);
      }
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setTesting(null);
    }
  };

  const handleSave = async (connector: string) => {
    setSaving(connector);
    try {
      const body = connector === "llm"
        ? { auth_type: "api_key", api_key: llmKey, model: llmModel, base_url: llmBaseUrl }
        : { auth_type: "token", path: gitPath, token: gitToken };
      await apiFetch(`/connections/${connector}`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      toast.success(`${connector.toUpperCase()} connector saved`);
      load();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(null);
    }
  };

  const handleDisconnect = async (connector: string) => {
    try {
      await apiFetch(`/connections/${connector}`, { method: "DELETE" });
      toast.success(`${connector.toUpperCase()} disconnected`);
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const statusBadge = (conn: any) => {
    if (conn?.status === "connected") {
      return (
        <Badge className="gap-1 bg-emerald-50 text-emerald-700 border-emerald-200" variant="outline">
          <CheckCircle2 className="h-3 w-3" /> Connected
        </Badge>
      );
    }
    return (
      <Badge className="gap-1 bg-gray-50 text-gray-500 border-gray-200" variant="outline">
        <XCircle className="h-3 w-3" /> Not Connected
      </Badge>
    );
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
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
        <p className="text-muted-foreground mt-1">Configure connectors for AI enrichment, Git access, and system status</p>
      </div>

      {/* Health */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2">
            <Settings className="h-4 w-4" /> System Status
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm">API Server</span>
            {health?.status === "healthy" ? (
              <Badge className="gap-1 bg-emerald-50 text-emerald-700 border-emerald-200" variant="outline">
                <CheckCircle2 className="h-3 w-3" /> Healthy
              </Badge>
            ) : (
              <Badge className="gap-1 bg-red-50 text-red-700 border-red-200" variant="outline">
                <XCircle className="h-3 w-3" /> Unreachable
              </Badge>
            )}
          </div>
          {health && (
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted-foreground">Version</span>
              <span className="text-sm font-mono">{health.version}</span>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Connector Tabs */}
      <Tabs defaultValue="llm" className="w-full">
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value="llm" className="gap-2"><Brain className="h-4 w-4" /> LLM / AI</TabsTrigger>
          <TabsTrigger value="git" className="gap-2"><GitBranch className="h-4 w-4" /> Git / SCM</TabsTrigger>
        </TabsList>

        {/* LLM Tab */}
        <TabsContent value="llm">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base">LLM / AI Provider</CardTitle>
                  <CardDescription>OpenAI-compatible API for AI-powered recommendations, enrichment, and summaries</CardDescription>
                </div>
                {statusBadge(connections.llm)}
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label>API Key</Label>
                <Input type="password" placeholder="sk-..." value={llmKey} onChange={(e) => setLlmKey(e.target.value)} />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label>Model</Label>
                  <Input placeholder="gpt-4o-mini" value={llmModel} onChange={(e) => setLlmModel(e.target.value)} />
                </div>
                <div className="space-y-2">
                  <Label>Base URL</Label>
                  <Input placeholder="https://api.openai.com/v1" value={llmBaseUrl} onChange={(e) => setLlmBaseUrl(e.target.value)} />
                </div>
              </div>
              <div className="flex gap-2 pt-2">
                <Button variant="outline" className="gap-2" onClick={() => handleTest("llm")} disabled={testing === "llm" || !llmKey}>
                  {testing === "llm" ? <Loader2 className="h-4 w-4 animate-spin" /> : <FlaskConical className="h-4 w-4" />}
                  Test Connection
                </Button>
                <Button className="gap-2" onClick={() => handleSave("llm")} disabled={saving === "llm" || !llmKey}>
                  {saving === "llm" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plug className="h-4 w-4" />}
                  Save & Connect
                </Button>
                {connections.llm?.status === "connected" && (
                  <Button variant="destructive" size="sm" className="gap-2 ml-auto" onClick={() => handleDisconnect("llm")}>
                    <Unplug className="h-4 w-4" /> Disconnect
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Git Tab */}
        <TabsContent value="git">
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base">Git / SCM</CardTitle>
                  <CardDescription>Local repository path or Git access token for cloning and scanning</CardDescription>
                </div>
                {statusBadge(connections.git)}
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label>Default Repository Path</Label>
                <Input placeholder="/path/to/your/api-project" value={gitPath} onChange={(e) => setGitPath(e.target.value)} />
                <p className="text-xs text-muted-foreground">Path to the default repo on disk. You can also set per-repo paths when adding repositories.</p>
              </div>
              <div className="space-y-2">
                <Label>Git Personal Access Token <span className="text-muted-foreground font-normal">(optional)</span></Label>
                <Input type="password" placeholder="ghp_..." value={gitToken} onChange={(e) => setGitToken(e.target.value)} />
                <p className="text-xs text-muted-foreground">Required only for cloning private repos or pushing changes.</p>
              </div>
              <div className="flex gap-2 pt-2">
                <Button variant="outline" className="gap-2" onClick={() => handleTest("git")} disabled={testing === "git" || !gitPath}>
                  {testing === "git" ? <Loader2 className="h-4 w-4 animate-spin" /> : <FlaskConical className="h-4 w-4" />}
                  Test Connection
                </Button>
                <Button className="gap-2" onClick={() => handleSave("git")} disabled={saving === "git" || !gitPath}>
                  {saving === "git" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plug className="h-4 w-4" />}
                  Save & Connect
                </Button>
                {connections.git?.status === "connected" && (
                  <Button variant="destructive" size="sm" className="gap-2 ml-auto" onClick={() => handleDisconnect("git")}>
                    <Unplug className="h-4 w-4" /> Disconnect
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
