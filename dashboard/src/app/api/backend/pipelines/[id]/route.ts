import { NextResponse } from "next/server";

const BASE = process.env.API_BASE_URL ?? "http://127.0.0.1:8080";

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_req: Request, context: RouteContext) {
  const { id } = await context.params;
  if (!id) {
    return NextResponse.json({ error: "missing id" }, { status: 400 });
  }
  const safe = encodeURIComponent(id);
  try {
    const r = await fetch(
      `${BASE.replace(/\/$/, "")}/observability/v1/pipelines/${safe}`,
      {
        cache: "no-store",
        headers: { Accept: "application/json" },
      },
    );
    const text = await r.text();
    if (!r.ok) {
      let err = text || r.statusText;
      try {
        const j = JSON.parse(text) as { error?: string };
        if (j.error) err = j.error;
      } catch {
        /* use text */
      }
      return NextResponse.json({ error: err }, { status: r.status });
    }
    let data: unknown;
    try {
      data = JSON.parse(text);
    } catch {
      return NextResponse.json(
        { error: "invalid json from API" },
        { status: 502 },
      );
    }
    return NextResponse.json(data);
  } catch (e) {
    return NextResponse.json(
      { error: e instanceof Error ? e.message : String(e) },
      { status: 502 },
    );
  }
}
