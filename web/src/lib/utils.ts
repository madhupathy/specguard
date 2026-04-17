import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

const API_BASE = "/api/v1";

export async function apiFetch<T = any>(
  path: string,
  init?: RequestInit
): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { "Content-Type": "application/json", ...init?.headers },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "Unknown error");
    throw new Error(`API ${res.status}: ${text}`);
  }
  return res.json();
}

export function timeAgo(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (seconds < 60) return "just now";
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 604800) return `${Math.floor(seconds / 86400)}d ago`;
  return date.toLocaleDateString();
}

export function severityColor(severity: string): string {
  switch (severity?.toLowerCase()) {
    case "breaking":
      return "text-red-600 bg-red-50 border-red-200";
    case "deprecation":
      return "text-amber-600 bg-amber-50 border-amber-200";
    case "non_breaking":
    case "addition":
      return "text-emerald-600 bg-emerald-50 border-emerald-200";
    default:
      return "text-gray-600 bg-gray-50 border-gray-200";
  }
}
