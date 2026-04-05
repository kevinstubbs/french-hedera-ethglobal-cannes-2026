import { NextRequest, NextResponse } from "next/server";

const BASE = process.env.API_BASE_URL ?? "http://127.0.0.1:8080";

export async function GET(req: NextRequest) {
  try {
    const pipelineId = req.nextUrl.searchParams.get("pipelineId");
    const u = new URL(
      `${BASE.replace(/\/$/, "")}/observability/v1/naryo/configuration`,
    );
    if (pipelineId?.trim()) {
      u.searchParams.set("pipelineId", pipelineId.trim());
    }
    const r = await fetch(u.toString(), {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    const text = await r.text();
    if (!r.ok) {
      return NextResponse.json(
        { error: text || r.statusText },
        { status: r.status },
      );
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
