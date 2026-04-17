"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import {
  Shield,
  LayoutDashboard,
  GitBranch,
  FileCode2,
  AlertTriangle,
  Package,
  Settings,
  FileText,
  ClipboardList,
  FileJson,
} from "lucide-react";

const navItems = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard },
  { href: "/repositories", label: "Repositories", icon: GitBranch },
  { href: "/specs", label: "Specs", icon: FileCode2 },
  { href: "/reports", label: "Reports", icon: ClipboardList },
  { href: "/documents", label: "Documents", icon: FileText },
  { href: "/swagger", label: "Swagger", icon: FileJson },
  { href: "/changes", label: "Changes", icon: AlertTriangle },
  { href: "/artifacts", label: "Artifacts", icon: Package },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="hidden md:flex md:w-64 md:flex-col border-r bg-white">
      {/* Logo */}
      <div className="flex h-16 items-center gap-2 border-b px-6">
        <Shield className="h-7 w-7 text-primary" />
        <span className="text-xl font-bold tracking-tight">
          <span className="bg-gradient-to-r from-primary via-blue-500 to-indigo-600 bg-clip-text text-transparent">
            SpecGuard
          </span>
        </span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-1 p-4">
        {navItems.map((item) => {
          const isActive =
            item.href === "/"
              ? pathname === "/"
              : pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                isActive
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Footer */}
      <div className="border-t p-4">
        <Link
          href="/settings"
          className="flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
        >
          <Settings className="h-4 w-4" />
          Settings
        </Link>
        <div className="mt-3 px-3 text-xs text-muted-foreground">
          SpecGuard v1.0.0
        </div>
      </div>
    </aside>
  );
}
