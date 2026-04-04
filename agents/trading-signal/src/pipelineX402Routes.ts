/**
 * Payment-gated route templates for the Go control plane.
 * Keep in sync with middleware.PipelineRoutes in internal/http/middleware/x402.go
 * (keys use a single "*" path segment wildcard). GET .../status is prepaid-only on the API, not x402.
 */
export const PIPELINE_X402_ROUTE_KEYS = ["POST /v1/pipelines/*/start"] as const;

export type PipelineX402RouteKey = (typeof PIPELINE_X402_ROUTE_KEYS)[number];

function splitPath(pathname: string): string[] {
  const p = pathname.split("?")[0] ?? pathname;
  return p.replace(/\/+$/, "").split("/").filter(Boolean);
}

function matchTemplate(
  method: string,
  pathname: string,
  template: PipelineX402RouteKey,
): boolean {
  const sp = template.indexOf(" ");
  const m = template.slice(0, sp).toUpperCase();
  const tmplPath = template.slice(sp + 1);
  if (method.toUpperCase() !== m) return false;
  const a = splitPath(pathname);
  const b = splitPath(tmplPath);
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (b[i] === "*") continue;
    if (a[i] !== b[i]) return false;
  }
  return true;
}

/** True if this HTTP request would be x402-gated on the Go API (same rules as PipelineRoutes). */
export function pipelineRouteRequiresX402(method: string, pathname: string): boolean {
  return PIPELINE_X402_ROUTE_KEYS.some((k) => matchTemplate(method, pathname, k));
}
