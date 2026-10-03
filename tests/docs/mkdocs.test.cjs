"use strict";
// Browser check of the built MkDocs site (./site, from scripts/docs/build.sh):
// every navigation entry answers 200, search finds the capability terms,
// every referenced screenshot loads and is in docs/assets/capturas/capturas.json,
// no page logs a console error or requests anything outside the local server,
// and no page scrolls horizontally on a 390px phone.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const { chromium } = require("playwright");

const ROOT = path.resolve(__dirname, "../..");
const SITE = path.join(ROOT, "site");
const TERMOS = ["engine", "gitleaks", "deliberacao", "skill", "ContextProvider", "excecao proposta"];
const TIPOS = { ".html": "text/html", ".css": "text/css", ".js": "text/javascript", ".json": "application/json",
  ".svg": "image/svg+xml", ".png": "image/png", ".yml": "text/plain", ".xml": "application/xml" };

function servidor() {
  return http.createServer((req, res) => {
    let nome = decodeURIComponent(new URL(req.url, "http://localhost").pathname);
    if (nome.endsWith("/")) nome += "index.html";
    const arquivo = path.resolve(SITE, "." + nome);
    if (!arquivo.startsWith(SITE + path.sep) || !fs.existsSync(arquivo) || !fs.statSync(arquivo).isFile()) {
      res.writeHead(404).end(); return;
    }
    res.setHeader("Content-Type", TIPOS[path.extname(arquivo)] || "application/octet-stream");
    res.end(fs.readFileSync(arquivo));
  });
}

function manifesto() {
  const json = JSON.parse(fs.readFileSync(path.join(ROOT, "docs/assets/capturas/capturas.json"), "utf8"));
  // Site path of each image: docs/assets/... is served at /assets/...
  return new Set(json.capturas.map(c => "/" + c.imagem.replace(/^docs\//, "")));
}

function vigia(page, base, problemas) {
  page.on("pageerror", e => problemas.push(page.url() + ": " + e.message));
  page.on("console", m => { if (m.type() === "error") problemas.push(page.url() + ": console: " + m.text()); });
  page.on("request", r => { if (!r.url().startsWith(base) && !r.url().startsWith("data:") && !r.url().startsWith("blob:")) problemas.push(page.url() + ": requisicao externa " + r.url()); });
}

async function main() {
  assert.ok(fs.existsSync(path.join(SITE, "index.html")), "site/ not built: run scripts/docs/build.sh");
  const capturas = manifesto();
  const srv = servidor();
  await new Promise(r => srv.listen(0, "127.0.0.1", r));
  const base = "http://127.0.0.1:" + srv.address().port + "/";
  const browser = await chromium.launch({ headless: true });
  const problemas = [];
  try {
    const desktop = await browser.newContext({ viewport: { width: 1280, height: 800 }, reducedMotion: "reduce" });
    const page = await desktop.newPage();
    vigia(page, base, problemas);
    assert.equal((await page.goto(base)).status(), 200);

    // Navigation: every entry of the primary nav (rendered on every page).
    const nav = [...new Set((await page.locator("nav.md-nav--primary a.md-nav__link[href]")
      .evaluateAll(as => as.map(a => a.href))).map(h => h.split("#")[0]))].filter(h => h.startsWith(base));
    assert.ok(nav.length > 50, "nav too small: " + nav.length);
    for (const url of nav) assert.equal((await page.request.get(url)).status(), 200, "nav " + url);

    // Search: each term returns at least one result.
    for (const termo of TERMOS) {
      await page.goto(base);
      await page.locator("label.md-search__icon, .md-search__input").first().click();
      await page.locator("input.md-search__input").fill(termo);
      await page.locator(".md-search-result__item").first().waitFor({ timeout: 15000 })
        .catch(() => { throw new Error("search without results: " + termo); });
      console.log(`search "${termo}": ${await page.locator(".md-search-result__item").count()} results`);
    }

    // Every page on a phone: no horizontal scroll, captures load and match the manifest.
    const mobile = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: "reduce" });
    const m = await mobile.newPage();
    vigia(m, base, problemas);
    const vistas = new Set();
    for (const url of nav) {
      assert.equal((await m.goto(url, { waitUntil: "load" })).status(), 200, url);
      const larga = await m.evaluate(() => document.documentElement.scrollWidth - innerWidth);
      assert.ok(larga <= 0, `horizontal scroll of ${larga}px on mobile: ${url}`);
      const imgs = await m.locator("img").evaluateAll(xs => xs.map(i => ({ src: i.currentSrc || i.src, ok: i.complete && i.naturalWidth > 0 })));
      for (const img of imgs.filter(i => i.src.includes("/assets/capturas/"))) {
        const caminho = new URL(img.src).pathname;
        assert.ok(img.ok, `capture does not load: ${caminho} on ${url}`);
        assert.ok(capturas.has(caminho), `capture outside the manifest: ${caminho} on ${url}`);
        assert.equal((await m.request.get(img.src)).status(), 200, caminho);
        vistas.add(caminho);
      }
    }
    assert.ok(vistas.size > 100, "too few captures referenced by the site: " + vistas.size);
    assert.deepEqual(problemas, []);
    console.log(`PASS: ${nav.length} nav pages 200, ${TERMOS.length} search terms, ${vistas.size} captures in the manifest, no console error, no horizontal scroll at 390px: ${base}`);
  } finally {
    await browser.close();
    await new Promise(r => srv.close(r));
  }
}
main().catch(e => { console.error(e); process.exitCode = 1; });
