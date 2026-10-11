const fs = require("fs");
const HERE = require("os").tmpdir() + "/qd-icons";
const OUT = "panel/src/assets/services/";
const INDEX = {
  "dash.json": "https://raw.githubusercontent.com/homarr-labs/dashboard-icons/main/tree.json",
  "dashmeta.json": "https://raw.githubusercontent.com/homarr-labs/dashboard-icons/main/metadata.json",
  "gil.json": "https://data.jsdelivr.com/v1/packages/gh/gilbarbara/logos@main?structure=flat",
  "sidata.json": "https://cdn.jsdelivr.net/npm/simple-icons@16.34.0/data/simple-icons.json",
};
const fetchIt = process.argv.includes("--fetch");
const only = process.argv.filter((a) => a.startsWith("--only=")).map((a) => a.slice(7).split(","))[0];
const UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36";

(async () => {
  fs.mkdirSync(HERE, { recursive: true });
  for (const [name, url] of Object.entries(INDEX)) {
    if (!fs.existsSync(HERE + "/" + name)) fs.writeFileSync(HERE + "/" + name, Buffer.from(await (await fetch(url)).arrayBuffer()));
  }
  await main();
})();

async function main() {
const dash = JSON.parse(fs.readFileSync(HERE + "/dash.json", "utf8"));
const dashMeta = JSON.parse(fs.readFileSync(HERE + "/dashmeta.json", "utf8"));
const dashSvg = new Set(dash.svg.map((n) => n.replace(/\.svg$/, "")));
const dashPng = new Set(dash.png.map((n) => n.replace(/\.png$/, "")));
const gil = new Set(JSON.parse(fs.readFileSync(HERE + "/gil.json", "utf8")).files.map((f) => f.name).filter((n) => n.startsWith("/logos/") && n.endsWith(".svg")).map((n) => n.slice(7, -4)));
const si = new Map(JSON.parse(fs.readFileSync(HERE + "/sidata.json", "utf8")).map((i) => [i.slug, i.hex]));
const ALIAS = JSON.parse(fs.readFileSync(__dirname + "/steerlist-icons.json", "utf8"));

const src = fs.readFileSync(__dirname + "/steerlist-catalog.js", "utf8");
const services = [...src.matchAll(/S\("([^"]+)", "([^"]+)", "([^"]*)"/g)].map((m) => ({ id: m[1], name: m[2], site: m[3] }));

const slugs = (s) => {
  const out = [s.id];
  const name = s.name.toLowerCase();
  out.push(name.replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, ""), name.replace(/[^a-z0-9]+/g, ""));
  if (s.site) {
    const parts = s.site.split(".");
    out.push(parts[parts.length - 2]);
  }
  return [...new Set(out.filter(Boolean))];
};

const pick = (s) => {
  const forced = ALIAS[s.id];
  if (forced) {
    const at = forced.indexOf(":");
    return { kind: forced.slice(0, at), slug: forced.slice(at + 1) };
  }
  const cand = slugs(s);
  for (const c of cand) if (dashSvg.has(c)) return { kind: "dash", slug: c };
  for (const c of cand) if (gil.has(c + "-icon")) return { kind: "gil", slug: c + "-icon" };
  for (const c of cand) if (si.has(c)) return { kind: "si", slug: c };
  for (const c of cand) if (dashPng.has(c)) return { kind: "dashpng", slug: c };
  return { kind: "site", slug: s.site };
};

const DASH = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/";
const lum = (hex) => {
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(hex.slice(i, i + 2), 16));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};
const monoSvg = (text) => {
  const found = new Set();
  for (const m of text.matchAll(/#([0-9a-f]{6}|[0-9a-f]{3})\b/gi)) {
    let h = m[1].toLowerCase();
    if (h.length === 3) h = h.split("").map((c) => c + c).join("");
    found.add(h);
  }
  if (/\b(rgb|hsl)a?\(|url\(#|<image\b/i.test(text)) return false;
  for (const h of found) if (lum(h) >= 40) return false;
  return true;
};
const pngWidth = (b) => (b.length > 24 && b[0] === 0x89 && b[1] === 0x50 ? b.readUInt32BE(16) : 0);
const get = async (url) => {
  const res = await fetch(url, { headers: { "User-Agent": UA, Accept: "*/*" }, redirect: "follow", signal: AbortSignal.timeout(15000) });
  if (!res.ok) throw new Error("http " + res.status);
  return { body: Buffer.from(await res.arrayBuffer()), url: res.url, type: res.headers.get("content-type") || "" };
};
const attr = (tag, name) => (tag.match(new RegExp("\\b" + name + "\\s*=\\s*[\"']([^\"']*)[\"']", "i")) || [])[1] || "";

const fromSite = async (site) => {
  const tries = [];
  try {
    const page = await get("https://" + site + "/");
    const html = page.body.toString("utf8");
    const links = [...html.matchAll(/<link\b[^>]*>/gi)].map((m) => ({ rel: attr(m[0], "rel").toLowerCase(), href: attr(m[0], "href"), type: attr(m[0], "type").toLowerCase(), sizes: parseInt(attr(m[0], "sizes"), 10) || 0 })).filter((l) => l.href && /icon/.test(l.rel) && !/mask/.test(l.rel));
    const abs = (h) => new URL(h.replace(/&amp;/g, "&"), page.url).href;
    for (const l of links.filter((l) => /svg/.test(l.type) || /\.svg(\?|$)/.test(l.href))) tries.push({ url: abs(l.href), svg: true });
    for (const l of links.filter((l) => /apple-touch/.test(l.rel)).sort((a, b) => b.sizes - a.sizes)) tries.push({ url: abs(l.href) });
    for (const l of links.filter((l) => l.sizes >= 96).sort((a, b) => b.sizes - a.sizes)) tries.push({ url: abs(l.href) });
  } catch {}
  tries.push({ url: "https://" + site + "/apple-touch-icon.png" });
  tries.push({ url: "https://www.google.com/s2/favicons?sz=256&domain=" + site });
  for (const t of tries) {
    try {
      const got = await get(t.url);
      const text = got.body.subarray(0, 400).toString("utf8");
      if (t.svg || /<svg/i.test(text)) {
        if (/<svg/i.test(got.body.toString("utf8"))) return { body: got.body, ext: ".svg", from: t.url };
        continue;
      }
      const w = pngWidth(got.body);
      if (w >= 96) return { body: got.body, ext: ".png", from: t.url, w };
    } catch {}
  }
  return null;
};

  const tally = {};
  const rows = [];
  for (const s of services) {
    if (["zone-ru", "zone-su", "geo-ru", "ru-more"].includes(s.id)) continue;
    if (only && !only.includes(s.id)) continue;
    const p = pick(s);
    if (!fetchIt) { tally[p.kind] = (tally[p.kind] || 0) + 1; rows.push(`${s.id}=${p.kind}:${p.slug}`); continue; }
    let got = null;
    let night = null;
    let mono = false;
    let note = "";
    try {
      if (p.kind === "dash" || p.kind === "dashpng") {
        const ext = p.kind === "dash" ? ".svg" : ".png";
        const dir = p.kind === "dash" ? "svg/" : "png/";
        got = { body: (await get(DASH + dir + p.slug + ext)).body, ext };
        const light = dashMeta[p.slug] && dashMeta[p.slug].colors && dashMeta[p.slug].colors.light;
        if (light && light !== p.slug && (ext === ".svg" ? dashSvg : dashPng).has(light)) night = { body: (await get(DASH + dir + light + ext)).body, ext };
      } else if (p.kind === "gil") {
        got = { body: (await get("https://cdn.jsdelivr.net/gh/gilbarbara/logos/logos/" + p.slug + ".svg")).body, ext: ".svg" };
      } else if (p.kind === "si") {
        const hex = si.get(p.slug);
        mono = lum(hex) < 40 || lum(hex) > 235;
        const text = (await get("https://cdn.jsdelivr.net/npm/simple-icons@16.34.0/icons/" + p.slug + ".svg")).body.toString("utf8");
        got = { body: Buffer.from(text.replace("<svg ", `<svg fill="#${mono ? "000000" : hex}" `)), ext: ".svg" };
      } else {
        const found = await fromSite(p.slug);
        if (found) { got = found; note = " " + found.from.replace(/^https:\/\//, "").slice(0, 60) + (found.w ? " " + found.w + "px" : ""); }
      }
    } catch (e) { note = " !! " + String(e.message || e).slice(0, 60); }
    if (!got) { tally.kept = (tally.kept || 0) + 1; rows.push(`${s.id}: KEPT OLD (${p.kind}:${p.slug})${note}`); continue; }
    if (got.ext === ".svg" && !mono && !night) mono = monoSvg(got.body.toString("utf8"));
    for (const ext of [".png", ".svg", ".mono.svg", ".night.svg", ".night.png"]) if (fs.existsSync(OUT + s.id + ext)) fs.unlinkSync(OUT + s.id + ext);
    fs.writeFileSync(OUT + s.id + (mono ? ".mono.svg" : got.ext), got.body);
    if (night) fs.writeFileSync(OUT + s.id + ".night" + night.ext, night.body);
    const kind = p.kind + (mono ? "+mono" : "") + (night ? "+night" : "");
    tally[kind] = (tally[kind] || 0) + 1;
    rows.push(`${s.id}: ${p.kind}:${String(p.slug).slice(0, 30)} ${got.ext} ${Math.round(got.body.length / 1024)}k${mono ? " MONO" : ""}${night ? " +night" : ""}${note}`);
  }
  console.log(rows.join("\n"));
  console.log(JSON.stringify(tally));
}
