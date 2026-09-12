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
    await page.locator("#workflow-code").filter({ hasText: "workflow" }).waitFor();
    const workflow = await page.locator("#workflow-code").textContent();
    assert.equal(workflow.trim(), fs.readFileSync(path.resolve(__dirname, "../../.github/workflows/examples/code-review.yml"), "utf8").trim());
    assert.match(await page.title(), /AurumCode/);
    for (const href of await page.locator('a[href^="#"]').evaluateAll(nodes => nodes.map(n => n.getAttribute("href")))) {
      assert.equal(await page.locator(href).count(), 1, "Missing anchor " + href);
    }
    for (const file of ["style.css", "app.js", "mark.svg", "workflow.yml"]) {
      assert.equal((await page.request.get(new URL(file, base).href)).status(), 200, file);
    }
    const snippet = () => page.locator("#config-code").textContent();
    assert.match(await snippet(), /language: pt-BR/);
    assert.match(await snippet(), /publication: review/);
    await page.locator("#language").selectOption("en-US");
    await page.locator("#publication").selectOption("comments");
    await page.locator("#inline").uncheck();
    assert.match(await snippet(), /Não é necessário criar/);
    await page.locator("#language").selectOption("pt-BR");
    assert.equal(await snippet(), "review:\n  language: pt-BR");
    await page.locator('[data-copy="config-code"]').click();
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), "review:\n  language: pt-BR");
    await page.locator("#contexto summary").click();
    assert.equal(await page.locator("#context-code").isVisible(), true);
    await page.locator("#publication").selectOption("review");
    await page.locator("#inline").check();
    await page.locator('.brand[href="#inicio"]').first().click();
    await page.screenshot({ path: "/tmp/aurum-docs-desktop.png", fullPage: true });
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "Horizontal overflow at " + width);
      await page.locator("#publication").selectOption("comments");
      assert.doesNotMatch(await snippet(), /publication:/);
    }
    await page.setViewportSize({ width: 390, height: 844 });
    await page.locator('.brand[href="#inicio"]').first().click();
    await page.screenshot({ path: "/tmp/aurum-docs-mobile.png", fullPage: true });
    assert.deepEqual(errors, []);
    console.log("PASS: desktop/mobile, anchor and asset links, workflow download, config generation, clipboard and context disclosure: " + base);
  } finally {
    await browser.close();
    if (server) await new Promise(resolve => server.close(resolve));
  }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
