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
    case "potential_breaking":
      return "text-orange-600 bg-orange-50 border-orange-200";
    case "deprecation":
    case "documentation_only":
      return "text-amber-600 bg-amber-50 border-amber-200";
    case "non_breaking":
    case "addition":
      return "text-emerald-600 bg-emerald-50 border-emerald-200";
    default:
      return "text-gray-600 bg-gray-50 border-gray-200";
  }
}

export function severityIcon(severity: string): string {
  switch (severity?.toLowerCase()) {
    case "breaking": return "🔴";
    case "potential_breaking": return "🟠";
    case "deprecation": return "🟡";
    case "non_breaking": return "🟢";
    default: return "⚪";
  }
}

export function changeKindLabel(kind: string): string {
  const labels: Record<string, string> = {
    "endpoint.removed": "Endpoint Removed",
    "endpoint.added": "Endpoint Added",
    "method.removed": "HTTP Method Removed",
    "method.added": "HTTP Method Added",
    "parameter.removed": "Parameter Removed",
    "parameter.added": "Parameter Added",
    "parameter.location_changed": "Parameter Location Changed",
    "parameter.type_changed": "Parameter Type Changed",
    "parameter.became_required": "Parameter Now Required",
    "request_body.removed": "Request Body Removed",
    "request_body.became_required": "Request Body Now Required",
    "property.removed": "Property Removed",
    "property.added": "Property Added",
    "property.type_changed": "Property Type Changed",
    "field.became_required": "Field Now Required",
    "field.became_optional": "Field Now Optional",
    "enum.value_removed": "Enum Value Removed",
    "enum.value_added": "Enum Value Added",
    "enum.constraint_removed": "Enum Constraint Removed",
    "security.removed": "Security Scheme Removed",
    "security.added": "Security Scheme Added",
    "proto.field_number_reused": "Proto Field Number Reused",
    "proto.field_removed": "Proto Field Removed",
    "proto.field_type_changed": "Proto Field Type Changed",
    "service.removed": "gRPC Service Removed",
    "service.added": "gRPC Service Added",
    "schema.removed": "Schema Removed",
    "schema.added": "Schema Added",
  };
  return labels[kind] ?? kind;
}
