"use strict";
// Lossless PNG optimizer for the screenshots: decodes an 8-bit RGB/RGBA PNG
// and, when it has at most 256 distinct colors (terminal text, flat pages),
// rewrites it as an indexed PNG with the exact palette and maximum deflate.
// Every pixel keeps its exact value; an image with more colors is rewritten
// only with stronger compression. No dependency besides node:zlib.
const fs = require("node:fs");
const zlib = require("node:zlib");

const ASSINATURA = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);

function crc32(buf) {
  let c, crc = 0xffffffff;
  for (let n = 0; n < buf.length; n++) {
    c = (crc ^ buf[n]) & 0xff;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    crc = (crc >>> 8) ^ c;
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(tipo, dados) {
  const len = Buffer.alloc(4); len.writeUInt32BE(dados.length);
  const td = Buffer.concat([Buffer.from(tipo, "ascii"), dados]);
  const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(td));
  return Buffer.concat([len, td, crc]);
}

function decodifica(buf) {
  if (!buf.subarray(0, 8).equals(ASSINATURA)) throw new Error("not a PNG");
  let pos = 8, ihdr, idat = [];
  while (pos < buf.length) {
    const len = buf.readUInt32BE(pos), tipo = buf.toString("ascii", pos + 4, pos + 8);
    const dados = buf.subarray(pos + 8, pos + 8 + len);
    if (tipo === "IHDR") ihdr = dados; else if (tipo === "IDAT") idat.push(dados);
    pos += 12 + len;
  }
  const w = ihdr.readUInt32BE(0), h = ihdr.readUInt32BE(4), prof = ihdr[8], cor = ihdr[9], entrel = ihdr[12];
  if (prof !== 8 || (cor !== 2 && cor !== 6) || entrel !== 0) return null;
  const bpp = cor === 6 ? 4 : 3, linha = w * bpp;
  const cru = zlib.inflateSync(Buffer.concat(idat));
  const px = Buffer.alloc(linha * h);
  for (let y = 0; y < h; y++) {
    const f = cru[y * (linha + 1)], src = y * (linha + 1) + 1, dst = y * linha;
    for (let x = 0; x < linha; x++) {
      const a = x >= bpp ? px[dst + x - bpp] : 0, b = y ? px[dst - linha + x] : 0, c = x >= bpp && y ? px[dst - linha + x - bpp] : 0;
      let v = cru[src + x];
      if (f === 1) v += a; else if (f === 2) v += b; else if (f === 3) v += (a + b) >> 1;
      else if (f === 4) { const p = a + b - c, pa = Math.abs(p - a), pb = Math.abs(p - b), pc = Math.abs(p - c); v += pa <= pb && pa <= pc ? a : pb <= pc ? b : c; }
      px[dst + x] = v & 0xff;
    }
  }
  return { w, h, bpp, px };
}

function codifica(img) {
  const { w, h, bpp, px } = img;
  const indice = new Map(), paleta = [];
  const idx = Buffer.alloc((w + 1) * h);
  let cabe = true;
  for (let y = 0; y < h && cabe; y++) {
    idx[y * (w + 1)] = 0;
    for (let x = 0; x < w; x++) {
      const o = (y * w + x) * bpp;
      const chave = bpp === 4 ? px.readUInt32BE(o) : (px.readUIntBE(o, 3) * 256 + 255) >>> 0;
      let i = indice.get(chave);
      if (i === undefined) {
        if (paleta.length === 256) { cabe = false; break; }
        i = paleta.length; indice.set(chave, i); paleta.push(chave);
      }
      idx[y * (w + 1) + 1 + x] = i;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[10] = 0; ihdr[11] = 0; ihdr[12] = 0;
  const partes = [ASSINATURA];
  if (cabe) {
    ihdr[9] = 3;
    const plte = Buffer.alloc(paleta.length * 3), trns = Buffer.alloc(paleta.length);
    paleta.forEach((k, i) => { plte[i * 3] = k >>> 24; plte[i * 3 + 1] = (k >>> 16) & 255; plte[i * 3 + 2] = (k >>> 8) & 255; trns[i] = k & 255; });
    partes.push(chunk("IHDR", ihdr), chunk("PLTE", plte));
    if (trns.some(a => a !== 255)) partes.push(chunk("tRNS", trns));
    partes.push(chunk("IDAT", zlib.deflateSync(idx, { level: 9 })));
  } else {
    ihdr[9] = bpp === 4 ? 6 : 2;
    const linha = w * bpp, cru = Buffer.alloc((linha + 1) * h);
    for (let y = 0; y < h; y++) { cru[y * (linha + 1)] = 2; for (let x = 0; x < linha; x++) cru[y * (linha + 1) + 1 + x] = (px[y * linha + x] - (y ? px[(y - 1) * linha + x] : 0)) & 255; }
    partes.push(chunk("IHDR", ihdr), chunk("IDAT", zlib.deflateSync(cru, { level: 9 })));
  }
  partes.push(chunk("IEND", Buffer.alloc(0)));
  return Buffer.concat(partes);
}

// otimiza(file): rewrites the file only when the result is smaller.
function otimiza(arquivo) {
  const orig = fs.readFileSync(arquivo);
  const img = decodifica(orig);
  if (!img) return;
  const novo = codifica(img);
  if (novo.length < orig.length) fs.writeFileSync(arquivo, novo);
}

module.exports = { otimiza, decodifica };
