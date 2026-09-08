import { describe, expect, it } from "vitest";
import { BASE_URL } from "../config";

describe("health and readiness endpoints", () => {
  it("GET /healthz returns 200 ok", async () => {
    const res = await fetch(`${BASE_URL}/healthz`);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ok");
  });

  it("GET /readyz returns 200 once content is mounted", async () => {
    const res = await fetch(`${BASE_URL}/readyz`);
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ready");
  });

  it("rejects non-GET/HEAD methods", async () => {
    const res = await fetch(`${BASE_URL}/healthz`, { method: "POST" });
    expect(res.status).toBe(405);
    expect(res.headers.get("allow")).toContain("GET");
  });
});

describe("static content hardening", () => {
  it("sets hardened security headers on every response", async () => {
    const res = await fetch(`${BASE_URL}/`);
    expect(res.status).toBe(200);
    expect(res.headers.get("x-content-type-options")).toBe("nosniff");
    expect(res.headers.get("x-frame-options")).toBe("DENY");
    expect(res.headers.get("content-security-policy")).toContain(
      "default-src 'self'",
    );
    expect(res.headers.get("referrer-policy")).toBe("no-referrer");
  });

  it("returns 404 for unknown paths", async () => {
    const res = await fetch(`${BASE_URL}/does-not-exist`);
    expect(res.status).toBe(404);
  });

  it("blocks path traversal attempts", async () => {
    const res = await fetch(`${BASE_URL}/%2e%2e/%2e%2e/etc/passwd`);
    expect(res.status).toBe(404);
  });

  it("blocks access to dotfiles", async () => {
    const res = await fetch(`${BASE_URL}/.env`);
    expect(res.status).toBe(404);
  });
});
