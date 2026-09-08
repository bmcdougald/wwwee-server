import type { Browser, Page } from "playwright";
import { chromium } from "playwright";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { BASE_URL } from "../config";

// These tests load the running container's pages in a real headless
// Chromium browser via Playwright, driven by Vitest as the test runner.
describe("landing page (real browser)", () => {
  let browser: Browser;
  let page: Page;

  beforeAll(async () => {
    browser = await chromium.launch();
    page = await browser.newPage();
  });

  afterAll(async () => {
    await browser?.close();
  });

  it("loads and renders the intranet landing page", async () => {
    const response = await page.goto(BASE_URL);
    expect(response?.status()).toBe(200);
    expect(await page.title()).toContain("Department Intranet");
    expect(await page.locator("h1").textContent()).toBe("Department Intranet");
  });

  it("serves the linked stylesheet", async () => {
    const response = await page.goto(`${BASE_URL}/assets/style.css`);
    expect(response?.status()).toBe(200);
    expect(response?.headers()["content-type"]).toContain("text/css");
  });

  it("serves same-origin external scripts", async () => {
    const response = await page.goto(`${BASE_URL}/assets/app.js`);
    expect(response?.status()).toBe(200);
    expect(response?.headers()["content-type"]).toContain("text/javascript");
  });

  it("shows a 404 page for unknown routes", async () => {
    const response = await page.goto(`${BASE_URL}/does-not-exist`);
    expect(response?.status()).toBe(404);
  });
});
