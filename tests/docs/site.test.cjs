"use strict";
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const { chromium } = require("playwright");

async function main() {
  const root = path.resolve(__dirname, "../../docs/site");
  let server;
  let base = process.env.DOCS_URL;
  if (!base) {
    server = http.createServer((req, res) => {
      const pathname = new URL(req.url, "http://localhost").pathname;
      if (!pathname.startsWith("/AurumCode/")) { res.writeHead(404).end(); return; }
      const name = pathname.slice("/AurumCode/".length) || "index.html";
      const file = path.resolve(root, name);
      if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
        res.writeHead(404).end(); return;
      }
      const mime = { ".html": "text/html", ".css": "text/css", ".js": "text/javascript", ".svg": "image/svg+xml", ".yml": "text/plain" };
      res.setHeader("Content-Type", (mime[path.extname(file)] || "text/plain") + "; charset=utf-8");
      res.end(fs.readFileSync(file));
    });
    await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
    base = "http://127.0.0.1:" + server.address().port + "/AurumCode/";
  }
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, reducedMotion: "reduce" });
    await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: new URL(base).origin });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    assert.equal((await page.goto(base)).status(), 200);
    assert.match(await page.title(), /AurumCode/);

    // The install snippet must mirror the shipped example verbatim.
    await page.locator("#workflow-code").waitFor();
    const workflow = await page.locator("#workflow-code").textContent();
    assert.equal(workflow.trim(), fs.readFileSync(path.resolve(__dirname, "../../.github/workflows/examples/code-review.yml"), "utf8").trim());

    // Every in-page anchor resolves to exactly one element.
    for (const href of await page.locator('a[href^="#"]').evaluateAll(nodes => nodes.map(n => n.getAttribute("href")))) {
      assert.equal(await page.locator(href).count(), 1, "Missing anchor " + href);
    }
    for (const file of ["style.css", "app.js", "mark.svg", "workflow.yml"]) {
      assert.equal((await page.request.get(new URL(file, base).href)).status(), 200, file);
    }

    // The tutorials section exposes a concrete example per capability.
    assert.equal(await page.locator("#tutoriais").count(), 1);
    const tutorials = await page.locator("#tutoriais").textContent();
    assert.match(tutorials, /memory: local/);
    assert.match(tutorials, /aurumcode:local fix/);
    assert.match(tutorials, /--limite/);
    assert.match(tutorials, /LLM_BASE_URL/);

    // The public page must not leak local model values, ports or personal endpoints.
    const site = fs.readFileSync(path.join(root, "index.html"), "utf8");
    for (const leak of ["claude-qwen38", "qwen-local", "11435"]) {
      assert.ok(!site.includes(leak), "leaked in docs/site/index.html: " + leak);
    }

    // Copy-to-clipboard still works for the workflow snippet.
    await page.locator('[data-copy="workflow-code"]').click();
    assert.equal((await page.evaluate(() => navigator.clipboard.readText())).trim(), workflow.trim());

    await page.screenshot({ path: "/tmp/aurum-docs-desktop.png", fullPage: true });
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "Horizontal overflow at " + width);
    }
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: "/tmp/aurum-docs-mobile.png", fullPage: true });
    assert.deepEqual(errors, []);
    console.log("PASS: hero, workflow snippet, anchors, assets, tutorials, clipboard, mobile: " + base);
  } finally {
    await browser.close();
    if (server) await new Promise(resolve => server.close(resolve));
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
