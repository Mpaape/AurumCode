"use strict";
// Generates the documentation screenshots (scripts/docs/capturas.sh is the
// entry point; this file runs inside the digest-pinned Playwright image).
//
//   node capturas.cjs secoes    rewrites the "Como fica" block of every
//                               docs/tutorials/<t>.md (between markers)
//   node capturas.cjs casos     per tutorial case, renders what the user sees
//                               (terminal, PR comment, status checks) from
//                               out/<case>.log
//   node capturas.cjs paginas   serves ./site, captures every capability page
//                               (desktop and mobile), writes the manifest
//                               docs/assets/capturas/capturas.json
//
// The manifest records the digest of each image's INPUT (the out/ log or the
// page's Markdown source), never the PNG digest: two runs over the same inputs
// produce the same manifest and the same file set, while PNG bytes may differ
// between machines (fonts, GPU). It carries no date for the same reason.
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const crypto = require("node:crypto");
const { otimiza } = require("./png-sem-perda.cjs");

const ROOT = path.resolve(__dirname, "../..");
const TUTORIAIS = path.join(ROOT, "demo/tutoriais");
const DESTINO = "docs/assets/capturas";
const MANIFESTO = path.join(ROOT, DESTINO, "capturas.json");
const COMANDO = "bash scripts/docs/capturas.sh";
const INICIO = "<!-- capturas:inicio (gerado por scripts/docs/capturas.sh; nao editar a mao) -->";
const FIM = "<!-- capturas:fim -->";
const VISTAS = {
  desktop: { width: 1280, height: 800 },
  mobile: { width: 390, height: 844 },
};
const LARGURA_RENDER = 900;
// Capability pages captured in desktop and mobile, besides every tutorial.
const PAGINAS_FIXAS = ["index", "configuration", "gate-corporativo", "architecture", "extensao"];

const sha256 = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const rel = file => path.relative(ROOT, file).split(path.sep).join("/");

function tutoriais() {
  return fs.readdirSync(TUTORIAIS)
    .filter(t => !t.startsWith("_") && fs.existsSync(path.join(TUTORIAIS, t, "out")))
    .filter(t => fs.existsSync(path.join(ROOT, "docs/tutorials", t + ".md")))
    .sort();
}

function casos(t) {
  return fs.readdirSync(path.join(TUTORIAIS, t, "out"))
    .filter(n => n.endsWith(".log"))
    .map(n => n.slice(0, -4))
    .sort();
}

const log = (t, c) => fs.readFileSync(path.join(TUTORIAIS, t, "out", c + ".log"), "utf8");

// ------------------------------------------------------------ what the user sees

// The PR comment is the last "## Code Review Summary" block of the log, up to
// the first line the CLI prints outside the comment.
const FORA_DO_COMENTARIO = /^(exit_code=|RESULTADO:|\$ |--- |== |aurumcode |\S+:\d+: \[)/;
function comentario(texto) {
  const linhas = texto.split("\n");
  const inicio = linhas.lastIndexOf("## Code Review Summary");
  if (inicio < 0) return null;
  const corpo = [];
  let cerca = false;
  for (const l of linhas.slice(inicio)) {
    if (!cerca && corpo.length && FORA_DO_COMENTARIO.test(l)) break;
    if (l.startsWith("```")) cerca = !cerca;
    corpo.push(l);
  }
  return corpo.join("\n").trim();
}

const STATUS = /^status publicado: context=(\S+) state=(\S+)/;
function status(texto) {
  const linhas = texto.split("\n").map(l => STATUS.exec(l)).filter(Boolean);
  return linhas.length ? linhas.map(m => ({ contexto: m[1], estado: m[2] })) : null;
}

const escapa = s => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

// ANSI SGR (colors, bold) to HTML spans; any other escape sequence is dropped.
const CORES = ["#3b3b3b", "#f14c4c", "#23d18b", "#f5f543", "#3b8eea", "#d670d6", "#29b8db", "#e5e5e5"];
function ansiParaHtml(texto) {
  let html = "", aberto = false;
  for (const parte of texto.split(/(\x1b\[[0-9;]*[A-Za-z])/)) {
    const m = /^\x1b\[([0-9;]*)m$/.exec(parte);
    if (m) {
      if (aberto) { html += "</span>"; aberto = false; }
      const estilos = [];
      for (const cod of m[1].split(";").map(Number)) {
        if (cod === 1) estilos.push("font-weight:bold");
        if (cod >= 30 && cod <= 37) estilos.push("color:" + CORES[cod - 30]);
        if (cod >= 90 && cod <= 97) estilos.push("color:" + CORES[cod - 90]);
      }
      if (estilos.length) { html += '<span style="' + estilos.join(";") + '">'; aberto = true; }
    } else if (!/^\x1b/.test(parte)) {
      html += escapa(parte);
    }
  }
  return html + (aberto ? "</span>" : "");
}

function inline(s) {
  return escapa(s)
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, '<a href="#">$1</a>');
}

// Minimal Markdown renderer for the review comment (headings, lists, fences,
// paragraphs): the subset the review summary uses, as GitHub shows it.
function markdownParaHtml(md) {
  const out = [];
  let cerca = null, lista = false;
  const fechaLista = () => { if (lista) { out.push("</ul>"); lista = false; } };
  for (const l of md.split("\n")) {
    if (cerca !== null) {
      if (l.startsWith("```")) { out.push("<pre><code>" + escapa(cerca.join("\n")) + "</code></pre>"); cerca = null; }
      else cerca.push(l);
      continue;
    }
    if (l.startsWith("```")) { fechaLista(); cerca = []; continue; }
    const h = /^(#{1,6})\s+(.*)$/.exec(l);
    const item = /^(\s*)[-*]\s+(.*)$/.exec(l);
    if (h) { fechaLista(); out.push(`<h${h[1].length}>${inline(h[2])}</h${h[1].length}>`); }
    else if (item) {
      if (!lista) { out.push("<ul>"); lista = true; }
      out.push(`<li style="margin-left:${item[1].length * 10}px">${inline(item[2])}</li>`);
    } else if (!l.trim()) fechaLista();
    else { fechaLista(); out.push("<p>" + inline(l) + "</p>"); }
  }
  fechaLista();
  if (cerca !== null) out.push("<pre><code>" + escapa(cerca.join("\n")) + "</code></pre>");
  return out.join("\n");
}

const BASE_CSS = `*{box-sizing:border-box}body{margin:0;padding:16px;background:#fff;font-family:"DejaVu Sans",Arial,sans-serif;width:${LARGURA_RENDER}px}`;

function htmlTerminal(t, c, texto) {
  return `<!doctype html><meta charset="utf-8"><style>${BASE_CSS}
#alvo{background:#1e1e1e;color:#e5e5e5;border-radius:8px;overflow:hidden;border:1px solid #333}
.barra{background:#2d2d2d;color:#aaa;font:12px "DejaVu Sans Mono",monospace;padding:6px 12px}
pre{margin:0;padding:12px;font:12px/1.45 "DejaVu Sans Mono",monospace;white-space:pre-wrap;word-break:break-word}</style>
<div id="alvo"><div class="barra">terminal: demo/tutoriais/${escapa(t)}/run.sh ${escapa(c)}</div><pre>${ansiParaHtml(texto.trimEnd())}</pre></div>`;
}

function htmlComentario(md) {
  return `<!doctype html><meta charset="utf-8"><style>${BASE_CSS}
#alvo{border:1px solid #d0d7de;border-radius:6px;color:#1f2328;font-size:14px;line-height:1.5}
.cab{background:#f6f8fa;border-bottom:1px solid #d0d7de;padding:8px 16px;font-size:13px;color:#59636e}
.corpo{padding:8px 16px}h2{font-size:20px;border-bottom:1px solid #d1d9e0;padding-bottom:4px}
pre{background:#f6f8fa;padding:12px;border-radius:6px;font:12px "DejaVu Sans Mono",monospace;white-space:pre-wrap}
code{background:#eff1f3;padding:1px 4px;border-radius:4px;font-family:"DejaVu Sans Mono",monospace;font-size:12px}
pre code{background:none;padding:0}ul{padding-left:24px}a{color:#0969da}</style>
<div id="alvo"><div class="cab"><strong>aurumcode</strong> commented (pull request, rendered Markdown)</div>
<div class="corpo">${markdownParaHtml(md)}</div></div>`;
}

function htmlStatus(itens) {
  const icone = e => e === "success" ? ['#1a7f37', "&#10004;"] : e === "pending" ? ['#9a6700', "&#9679;"] : ['#d1242f', "&#10006;"];
  const linhas = itens.map((s, i) => {
    const [cor, simbolo] = icone(s.estado);
    return `<div class="linha"><span style="color:${cor};font-weight:bold">${simbolo}</span> <strong>${escapa(s.contexto)}</strong> <span class="estado">${escapa(s.estado)}</span><span class="n">publicacao ${i + 1}</span></div>`;
  }).join("\n");
  return `<!doctype html><meta charset="utf-8"><style>${BASE_CSS}
#alvo{border:1px solid #d0d7de;border-radius:6px;color:#1f2328;font-size:14px}
.cab{background:#f6f8fa;border-bottom:1px solid #d0d7de;padding:10px 16px;font-weight:bold}
.linha{padding:8px 16px;border-bottom:1px solid #eaeef2}.estado{color:#59636e;margin-left:6px}.n{float:right;color:#8c959f;font-size:12px}</style>
<div id="alvo"><div class="cab">Status checks do commit (na ordem em que o aurumcode os publicou)</div>${linhas}</div>`;
}

// ------------------------------------------------------------ "Como fica"

const LEGENDA = { terminal: "Terminal", comentario: "Comentario do PR", status: "Status checks" };

function vistasDoCaso(t, c) {
  const texto = log(t, c);
  const vistas = ["terminal"];
  if (comentario(texto)) vistas.push("comentario");
  if (status(texto)) vistas.push("status");
  return vistas;
}

function secao(t) {
  const linhas = [INICIO, "## Como fica", "",
    `Capturas geradas por scripts/docs/capturas.sh a partir das saidas gravadas em demo/tutoriais/${t}/out/: ` +
    "o terminal de cada caso e, quando o caso publica, o comentario do PR e os status checks. " +
    "O manifesto docs/assets/capturas/capturas.json registra o digest de cada insumo.", ""];
  for (const c of casos(t)) {
    linhas.push(`### ${c}`, "");
    for (const v of vistasDoCaso(t, c)) {
      linhas.push(`![${LEGENDA[v]} do caso ${c}](../assets/capturas/${t}/${c}-${v}.png)`, "");
    }
  }
  linhas.push(FIM);
  return linhas.join("\n");
}

function secoes() {
  for (const t of tutoriais()) {
    const arquivo = path.join(ROOT, "docs/tutorials", t + ".md");
    let md = fs.readFileSync(arquivo, "utf8");
    const i = md.indexOf(INICIO), f = md.indexOf(FIM);
    if (i >= 0 && f > i) md = md.slice(0, i).trimEnd() + "\n";
    md = md.trimEnd() + "\n\n" + secao(t) + "\n";
    fs.writeFileSync(arquivo, md);
  }
}

// ------------------------------------------------------------ capture

function servidor(raiz) {
  const tipos = { ".html": "text/html", ".css": "text/css", ".js": "text/javascript", ".json": "application/json",
    ".svg": "image/svg+xml", ".png": "image/png", ".yml": "text/plain", ".woff2": "font/woff2" };
  return http.createServer((req, res) => {
    let nome = decodeURIComponent(new URL(req.url, "http://localhost").pathname);
    if (nome.endsWith("/")) nome += "index.html";
    const arquivo = path.resolve(raiz, "." + nome);
    if (!arquivo.startsWith(raiz + path.sep) || !fs.existsSync(arquivo) || !fs.statSync(arquivo).isFile()) {
      res.writeHead(404).end(); return;
    }
    res.setHeader("Content-Type", (tipos[path.extname(arquivo)] || "application/octet-stream"));
    res.end(fs.readFileSync(arquivo));
  });
}

// Every image the inputs call for, with the digest of its input. It depends
// only on docs/ and demo/tutoriais/*/out/, so it is the same on every run.
function paginas() {
  return PAGINAS_FIXAS.map(p => ({ slug: p, md: `docs/${p}.md`, url: p === "index" ? "" : p + "/" }))
    .concat(tutoriais().map(t => ({ slug: "tutorials-" + t, md: `docs/tutorials/${t}.md`, url: `tutorials/${t}/` })));
}

function esperadas() {
  const entradas = [];
  for (const p of paginas()) {
    for (const vista of Object.keys(VISTAS)) {
      entradas.push({ imagem: `${DESTINO}/paginas/${p.slug}-${vista}.png`, tipo: "pagina-" + vista, insumo: p.md, pagina: p });
    }
  }
  for (const t of tutoriais()) {
    for (const c of casos(t)) {
      for (const v of vistasDoCaso(t, c)) {
        entradas.push({ imagem: `${DESTINO}/${t}/${c}-${v}.png`, tipo: v, insumo: `demo/tutoriais/${t}/out/${c}.log`, tutorial: t, caso: c });
      }
    }
  }
  return entradas.sort((a, b) => a.imagem < b.imagem ? -1 : a.imagem > b.imagem ? 1 : 0);
}

function imagemPinada() {
  const imagem = process.env.AURUM_PLAYWRIGHT_IMAGE || "";
  if (!/@sha256:[0-9a-f]{64}$/.test(imagem)) throw new Error("AURUM_PLAYWRIGHT_IMAGE must be pinned by digest");
  return imagem;
}

function escreveManifesto(imagem) {
  // One entry per line, so the sealed acceptance can read it with awk/sed.
  const linhas = esperadas().map(e => "    " + JSON.stringify({
    imagem: e.imagem, tipo: e.tipo, insumo: e.insumo, insumo_sha256: sha256(path.join(ROOT, e.insumo)),
    playwright: imagem, comando: COMANDO }));
  const json = "{\n" +
    `  "gerador": ${JSON.stringify(COMANDO)},\n` +
    `  "playwright": ${JSON.stringify(imagem)},\n` +
    `  "nota": "insumo_sha256 e o digest do out/ ou da pagina-fonte; o PNG pode variar byte a byte entre maquinas e nao entra no manifesto",\n` +
    "  \"capturas\": [\n" + linhas.join(",\n") + "\n  ]\n}\n";
  fs.writeFileSync(MANIFESTO, json);
  console.log(`capturas: ${linhas.length} imagens no manifesto ${rel(MANIFESTO)}`);
}

const filtro = e => !process.env.AURUM_CAPTURAS_SO ||
  process.env.AURUM_CAPTURAS_SO.split(",").some(s => e.imagem.includes("/" + s));

async function comNavegador(fn) {
  const { chromium } = require("playwright");
  const browser = await chromium.launch({ headless: true, args: ["--disable-lcd-text", "--font-render-hinting=none"] });
  try { return await fn(browser); } finally { await browser.close(); }
}

const contexto = (browser, viewport) =>
  browser.newContext({ viewport, deviceScaleFactor: 1, reducedMotion: "reduce", colorScheme: "light" });

// Case renders: the terminal, the PR comment and the status checks, each from
// out/<case>.log, screenshotted at a fixed width.
async function capturarCasos() {
  imagemPinada();
  await comNavegador(async browser => {
    const page = await (await contexto(browser, VISTAS.desktop)).newPage();
    for (const e of esperadas().filter(e => e.caso && filtro(e))) {
      const texto = log(e.tutorial, e.caso);
      const html = e.tipo === "terminal" ? htmlTerminal(e.tutorial, e.caso, texto)
        : e.tipo === "comentario" ? htmlComentario(comentario(texto)) : htmlStatus(status(texto));
      await page.setContent(html, { waitUntil: "load" });
      fs.mkdirSync(path.dirname(path.join(ROOT, e.imagem)), { recursive: true });
      await page.locator("#alvo").screenshot({ path: path.join(ROOT, e.imagem), animations: "disabled" });
      otimiza(path.join(ROOT, e.imagem));
    }
  });
}

// Capability pages of the built site (./site), desktop and mobile, served
// locally; any request outside the local server is aborted.
async function capturarPaginas() {
  const imagem = imagemPinada();
  const srv = servidor(path.join(ROOT, "site"));
  await new Promise(r => srv.listen(0, "127.0.0.1", r));
  const base = "http://127.0.0.1:" + srv.address().port + "/";
  const semAnimacao = "*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}";
  try {
    await comNavegador(async browser => {
      const ctx = {};
      for (const [nome, viewport] of Object.entries(VISTAS)) {
        ctx[nome] = await contexto(browser, viewport);
        await ctx[nome].route(u => !u.href.startsWith(base), r => r.abort());
      }
      for (const e of esperadas().filter(e => e.pagina && filtro(e))) {
        const page = await ctx[e.tipo.replace("pagina-", "")].newPage();
        const resp = await page.goto(base + e.pagina.url, { waitUntil: "load" });
        if (resp.status() !== 200) throw new Error(`${e.pagina.url}: HTTP ${resp.status()}`);
        await page.addStyleTag({ content: semAnimacao });
        await page.evaluate(() => document.fonts.ready);
        fs.mkdirSync(path.dirname(path.join(ROOT, e.imagem)), { recursive: true });
        await page.screenshot({ path: path.join(ROOT, e.imagem), animations: "disabled" });
        otimiza(path.join(ROOT, e.imagem));
        await page.close();
      }
    });
  } finally {
    await new Promise(r => srv.close(r));
  }
  escreveManifesto(imagem);
}

const modo = process.argv[2];
const acoes = { secoes: async () => secoes(), casos: capturarCasos, paginas: capturarPaginas };
if (!acoes[modo]) { console.error("uso: capturas.cjs secoes|casos|paginas"); process.exit(64); }
acoes[modo]().catch(e => { console.error(e); process.exitCode = 1; });
