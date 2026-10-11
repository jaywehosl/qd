const fs = require("fs");
const BASE = "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/";
const GEO = "https://raw.githubusercontent.com/ipverse/rir-ip/master/country/ru/";
const AS = (n) => "https://raw.githubusercontent.com/ipverse/asn-ip/master/as/" + n + "/ipv4-aggregated.txt";
const X = (...names) => ({ extra: names });

const S = (id, name, site, files, more = {}) => ({ id, name, site, files: files === undefined ? [id] : files, ...more });

const PLAN = [
  ["ai", "AI", [
    S("openai", "OpenAI", "openai.com", ["openai"], X("chatgpt.com", "sora.com", "chat.com", "oaistatic.com", "oaiusercontent.com")),
    S("anthropic", "Claude", "claude.ai", ["anthropic"], X("claude.com")),
    S("gemini", "Gemini", "gemini.google.com", ["google-deepmind", "google-gemini"], X("gemini.google", "notebooklm.google", "notebooklm.google.com", "aistudio.google.com", "generativelanguage.googleapis.com", "jules.google", "labs.google", "antigravity.google", "generativeai.google", "deepmind.google", "deepmind.com")),
    S("copilot", "Microsoft Copilot", "copilot.microsoft.com", []), S("github-copilot", "GitHub Copilot", "github.com"),
    S("grok", "Grok", "grok.com", ["xai"], X("grok.com", "x.ai")), S("perplexity", "Perplexity", "perplexity.ai"),
    S("deepseek", "DeepSeek", "deepseek.com"), S("mistral", "Mistral", "mistral.ai", []), S("poe", "Poe", "poe.com"),
    S("huggingface", "Hugging Face", "huggingface.co"), S("groq", "Groq", "groq.com"),
    S("manus", "Manus", "manus.im"), S("cerebras", "Cerebras", "cerebras.ai"),
    S("character-ai", "Character.AI", "character.ai", []), S("midjourney", "Midjourney", "midjourney.com", []),
    S("suno", "Suno", "suno.com", []), S("udio", "Udio", "udio.com", []), S("elevenlabs", "ElevenLabs", "elevenlabs.io"),
    S("runway", "Runway", "runwayml.com", []), S("pika", "Pika", "pika.art", []),
    S("leonardo", "Leonardo", "leonardo.ai", []), S("ideogram", "Ideogram", "ideogram.ai", []),
    S("deepl", "DeepL", "deepl.com", [], X("deepl.com")), S("openrouter", "OpenRouter", "openrouter.ai", [], X("openrouter.ai")),
    S("genspark", "Genspark", "genspark.ai", [], X("genspark.ai")), S("recraft", "Recraft", "recraft.ai", [], X("recraft.ai")),
  ]],
  ["streaming", "Streaming", [
    S("youtube", "YouTube", "youtube.com"), S("twitch", "Twitch", "twitch.tv"), S("netflix", "Netflix", "netflix.com"),
    S("spotify", "Spotify", "spotify.com"), S("kick", "Kick", "kick.com"), S("soundcloud", "SoundCloud", "soundcloud.com"),
    S("apple-music", "Apple Music", "music.apple.com"), S("apple-tvplus", "Apple TV+", "tv.apple.com"),
    S("disney", "Disney+", "disneyplus.com"), S("hbo", "HBO Max", "max.com", ["hbo"], X("hbomax.com")), S("primevideo", "Prime Video", "primevideo.com"),
    S("hulu", "Hulu", "hulu.com"), S("deezer", "Deezer", "deezer.com"), S("tidal", "Tidal", "tidal.com"),
    S("vimeo", "Vimeo", "vimeo.com"), S("dailymotion", "Dailymotion", "dailymotion.com"), S("plex", "Plex", "plex.tv"),
    S("bandcamp", "Bandcamp", "bandcamp.com", ["bandcamp"], X("bcbits.com")), S("rumble", "Rumble", "rumble.com"), S("dazn", "DAZN", "dazn.com"),
    S("mubi", "MUBI", "mubi.com"), S("crunchyroll", "Crunchyroll", "crunchyroll.com", [], X("crunchyroll.com", "crunchyrollcdn.com", "vrv.co")),
    S("lastfm", "Last.fm", "last.fm", ["lastfm"], X("last.fm")),
  ]],
  ["social", "Social", [
    S("meta", "Instagram, Facebook", "instagram.com", ["facebook", "instagram", "meta", "threads", "messenger", "oculus"], { skip: ["whatsapp"] }),
    S("x", "X", "x.com", ["twitter", "x"], { skip: ["xai"] }), S("tiktok", "TikTok", "tiktok.com"),
    S("reddit", "Reddit", "reddit.com"), S("linkedin", "LinkedIn", "linkedin.com"), S("pinterest", "Pinterest", "pinterest.com"),
    S("snap", "Snapchat", "snapchat.com"), S("tumblr", "Tumblr", "tumblr.com"), S("bluesky", "Bluesky", "bsky.app"),
    S("quora", "Quora", "quora.com"), S("medium", "Medium", "medium.com"), S("patreon", "Patreon", "patreon.com"),
    S("deviantart", "DeviantArt", "deviantart.com"), S("pixiv", "pixiv", "pixiv.net"), S("imgur", "Imgur", "imgur.com"),
    S("flickr", "Flickr", "flickr.com"),
  ]],
  ["messengers", "Messengers", [
    S("telegram", "Telegram", "telegram.org", ["telegram"], { nets: ["https://core.telegram.org/resources/cidr.txt"] }),
    S("discord", "Discord", "discord.com", ["discord"], { nets: ["66.22.192.0/18", "138.128.136.0/21", "5.200.14.128/25"] }),
    S("whatsapp", "WhatsApp", "whatsapp.com"), S("signal", "Signal", "signal.org"),
    S("viber", "Viber", "viber.com"), S("line", "LINE", "line.me"), S("teamspeak", "TeamSpeak", "teamspeak.com"),
  ]],
  ["games", "Games", [
    S("steam", "Steam", "store.steampowered.com", ["steam"], { nets: [AS(32590)] }), S("epicgames", "Epic Games", "epicgames.com"),
    S("activision-blizzard", "Battle.net", "battle.net", ["activision-blizzard", "blizzard"], { nets: [AS(57976)] }), S("ea", "EA", "ea.com"),
    S("riot", "Riot Games", "riotgames.com", ["riot"], { nets: [AS(6507)] }), S("roblox", "Roblox", "roblox.com", ["roblox"], { nets: [AS(22697)] }),
    S("mojang", "Minecraft", "minecraft.net"), S("xbox", "Xbox", "xbox.com", ["xbox"], { skip: ["bethesda"] }),
    S("playstation", "PlayStation", "playstation.com"), S("nintendo", "Nintendo", "nintendo.com"),
    S("ubisoft", "Ubisoft", "ubisoft.com"), S("take-two", "Rockstar Games", "rockstargames.com", ["take-two", "rockstar"]),
    S("gog", "GOG", "gog.com"),
    S("wargaming", "Wargaming", "wargaming.net"), S("gaijin", "Gaijin", "gaijin.net"),
    S("escapefromtarkov", "Escape from Tarkov", "escapefromtarkov.com"), S("faceit", "FACEIT", "faceit.com"),
    S("supercell", "Supercell", "supercell.com"), S("hoyoverse", "HoYoverse", "hoyoverse.com"), S("pubg", "PUBG", "pubg.com"),
    S("curseforge", "CurseForge", "curseforge.com"), S("modrinth", "Modrinth", "modrinth.com"),
    S("itchio", "itch.io", "itch.io"), S("humblebundle", "Humble Bundle", "humblebundle.com"),
    S("geforce-now", "GeForce NOW", "geforcenow.com", [], X("geforcenow.com", "nvidiagrid.net")),
    S("chess", "Chess.com", "chess.com", [], X("chess.com", "chesscomfiles.com")), S("bethesda", "Bethesda", "bethesda.net"),
  ]],
  ["work", "Work", [
    S("notion", "Notion", "notion.so"), S("slack", "Slack", "slack.com"), S("zoom", "Zoom", "zoom.us"),
    S("google-meet", "Google Meet", "meet.google.com", [], { extra: ["meet.google.com", "meet.google"], nets: ["74.125.247.128/32", "74.125.250.0/24", "142.250.82.0/24"] }),
    S("figma", "Figma", "figma.com"), S("canva", "Canva", "canva.com"), S("miro", "Miro", "miro.com", [], { extra: ["miro.com", "mirostatic.com", "realtimeboard.com"] }),
    S("atlassian", "Atlassian", "atlassian.com", ["atlassian", "trello"]), S("adobe", "Adobe", "adobe.com"),
    S("grammarly", "Grammarly", "grammarly.com", []), S("dropbox", "Dropbox", "dropbox.com"), S("mega", "MEGA", "mega.io"),
    S("evernote", "Evernote", "evernote.com"), S("wix", "Wix", "wix.com"), S("coursera", "Coursera", "coursera.org"),
    S("udemy", "Udemy", "udemy.com"), S("edx", "edX", "edx.org"), S("khanacademy", "Khan Academy", "khanacademy.org"),
    S("duolingo", "Duolingo", "duolingo.com"), S("autodesk", "Autodesk", "autodesk.com"), S("framer", "Framer", "framer.com"),
    S("clickup", "ClickUp", "clickup.com", [], X("clickup.com")), S("linear", "Linear", "linear.app", [], X("linear.app")),
    S("zapier", "Zapier", "zapier.com", [], X("zapier.com")), S("teamviewer", "TeamViewer", "teamviewer.com"),
    S("anydesk", "AnyDesk", "anydesk.com", [], X("anydesk.com")), S("wetransfer", "WeTransfer", "wetransfer.com", [], X("wetransfer.com")),
    S("upwork", "Upwork", "upwork.com", [], X("upwork.com")),
  ]],
  ["dev", "Development", [
    S("npmjs", "npm", "npmjs.com"),
    S("github", "GitHub", "github.com", ["github"], { skip: ["github-copilot"], keep: ["github.com", "githubusercontent.com"] }),
    S("gitlab", "GitLab", "gitlab.com"), S("cursor", "Cursor", "cursor.com"), S("windsurf", "Windsurf", "windsurf.com"),
    S("jetbrains", "JetBrains", "jetbrains.com"), S("docker", "Docker Hub", "docker.com"),
    S("hashicorp", "HashiCorp", "hashicorp.com"), S("python", "Python", "python.org"),
    S("golang", "Go", "go.dev"), S("rust", "Rust", "rust-lang.org"),
    S("stackexchange", "Stack Overflow", "stackoverflow.com"), S("unity", "Unity", "unity.com"),
    S("anaconda", "Anaconda", "anaconda.com"), S("intel", "Intel", "intel.com"), S("amd", "AMD", "amd.com"),
    S("nvidia", "NVIDIA", "nvidia.com"), S("vercel", "Vercel", "vercel.com", ["vercel"], { keep: ["vercel.app"] }),
    S("netlify", "Netlify", "netlify.com"), S("digitalocean", "DigitalOcean", "digitalocean.com"),
    S("heroku", "Heroku", "heroku.com", ["heroku"], { keep: ["herokuapp.com"] }), S("codeberg", "Codeberg", "codeberg.org"),
    S("oracle", "Oracle", "oracle.com"), S("mongodb", "MongoDB", "mongodb.com"), S("redis", "Redis", "redis.io"),
    S("sentry", "Sentry", "sentry.io", ["sentry"], { keep: ["sentry.io"], extra: ["sentry.dev"] }), S("ngrok", "ngrok", "ngrok.com"),
    S("hetzner", "Hetzner", "hetzner.com"), S("qt", "Qt", "qt.io"), S("vmware", "VMware", "vmware.com"),
    S("grafana", "Grafana", "grafana.com", [], X("grafana.com", "grafana.net")), S("arduino", "Arduino", "arduino.cc", [], X("arduino.cc")),
  ]],
  ["shopping", "Shopping", [
    S("amazon", "Amazon", "amazon.com"), S("ebay", "eBay", "ebay.com"), S("etsy", "Etsy", "etsy.com", [], X("etsy.com", "etsystatic.com")),
    S("ikea", "IKEA", "ikea.com"), S("nike", "Nike", "nike.com"),
    S("adidas", "Adidas", "adidas.com"), S("hm", "H&M", "hm.com"), S("walmart", "Walmart", "walmart.com"),
    S("shopify", "Shopify", "shopify.com"), S("iherb", "iHerb", "iherb.com", [], X("iherb.com")), S("bestbuy", "Best Buy", "bestbuy.com"),
  ]],
  ["travel", "Travel", [
    S("booking", "Booking", "booking.com"), S("airbnb", "Airbnb", "airbnb.com"),
    S("expedia", "Expedia", "expedia.com", [], X("expedia.com", "hotels.com", "vrbo.com")),
    S("tripadvisor", "Tripadvisor", "tripadvisor.com", [], X("tripadvisor.com", "tacdn.com")),
    S("skyscanner", "Skyscanner", "skyscanner.net", [], X("skyscanner.net", "skyscanner.com")), S("uber", "Uber", "uber.com"),
  ]],
  ["finance", "Finance", [
    S("paypal", "PayPal", "paypal.com"), S("wise", "Wise", "wise.com"), S("revolut", "Revolut", "revolut.com", [], X("revolut.com")),
    S("stripe", "Stripe", "stripe.com"), S("payoneer", "Payoneer", "payoneer.com", [], X("payoneer.com")),
  ]],
  ["ru", "Russia", [
    S("zone-ru", ".ru .рф", "", [], { extra: ["ru", "xn--p1ai"], bare: true }),
    S("zone-su", ".su", "", [], { extra: ["su"], bare: true }),
    S("geo-ru", "Russian networks", "", [], { nets: [GEO + "ipv4-aggregated.txt", GEO + "ipv6-aggregated.txt"] }),
    S("okko", "Okko", "okko.tv"), S("dzen", "Dzen", "dzen.ru"), S("2gis", "2GIS", "2gis.ru"),
    S("yandex", "Yandex", "ya.ru"), S("vk", "VK", "vk.com"), S("ok", "OK", "ok.ru"),
    S("mailru-group", "Mail.ru", "mail.ru", ["mailru-group", "mailru"]), S("sberbank", "Sber", "sberbank.ru", ["sberbank", "sber"]),
    S("tbank-ru", "T-Bank", "tbank.ru"), S("ozon", "Ozon", "ozon.ru"), S("wildberries", "Wildberries", "wildberries.ru"),
    S("avito", "Avito", "avito.ru"), S("kinopoisk", "Kinopoisk", "kinopoisk.ru"),
    S("wink", "Wink", "wink.ru"), S("rutube", "Rutube", "rutube.ru"),
    S("mts-ru", "MTS", "mts.ru"), S("megafon", "MegaFon", "megafon.ru"),
    S("rostelecom", "Rostelecom", "rt.ru"), S("t2-ru", "T2", "t2.ru"), S("kaspersky", "Kaspersky", "kaspersky.ru"),
    S("headhunter", "hh.ru", "hh.ru"), S("cdek", "CDEK", "cdek.ru"), S("gismeteo", "Gismeteo", "gismeteo.ru"),
    S("ru-more", "More Russian services", "", ["category-ru"], { skip: ["tld-ru"] }),
  ]],
];

const SHARED = new Set([
  "cloudfront.net", "akamaized.net", "akamai.net", "akamaihd.net", "amazonaws.com", "azureedge.net", "azurefd.net",
  "windows.net", "googleusercontent.com", "gstatic.com", "googleapis.com", "google.com", "cloudflare.com",
  "cloudflare.net", "fastly.net", "github.com", "githubusercontent.com", "microsoft.com", "apple.com", "sentry.io",
  "cloudinary.com", "b-cdn.net", "edgekey.net", "edgesuite.net", "appspot.com", "herokuapp.com", "vercel.app",
  "live.com", "office.com", "icloud.com", "mzstatic.com", "akadns.net", "msecnd.net", "aka.ms", "goo.gl",
]);

const cache = new Map();
async function file(name) {
  if (cache.has(name)) return cache.get(name);
  const res = await fetch(BASE + encodeURIComponent(name));
  const text = res.ok ? await res.text() : "";
  cache.set(name, text);
  return text;
}

async function names(name, skip, seen = new Set()) {
  if (seen.has(name) || skip.has(name)) return [];
  seen.add(name);
  const out = [];
  for (let line of (await file(name)).split("\n")) {
    line = line.split("#")[0].trim();
    if (!line) continue;
    const [head, ...attrs] = line.split(/\s+/);
    if (attrs.includes("@ads")) continue;
    if (head.startsWith("include:")) { out.push(...await names(head.slice(8), skip, seen)); continue; }
    if (head.startsWith("regexp:") || head.startsWith("keyword:")) continue;
    const one = head.replace(/^(domain|full):/, "").toLowerCase();
    if (/^[a-z0-9.-]+$/.test(one) && one.includes(".")) out.push(one);
  }
  return out;
}

const covered = (name, set) => {
  for (let rest = name; ;) {
    const dot = rest.indexOf(".");
    if (dot < 0) return false;
    rest = rest.slice(dot + 1);
    if (set.has(rest)) return true;
  }
};

(async () => {
  const held = {};
  for (const line of fs.readFileSync("internal/steerlist/names.txt", "utf8").split("\n")) {
    const [id, ...rest] = line.trim().split(/\s+/);
    if (id) held[id] = rest;
  }

  const owner = new Map();
  const nameLines = [], netLines = [], report = [], clashes = [], empty = [];
  for (const [bid, , services] of PLAN) {
    for (const s of services) {
      const skip = new Set(s.skip || []);
      const keep = new Set(s.keep || []);
      const merged = new Set([...(s.extra || []), ...(s.bare ? [] : held[s.id] || [])]);
      for (const f of s.files) for (const n of await names(f, skip)) merged.add(n);
      for (const n of [...merged]) if (SHARED.has(n) && !keep.has(n) && !s.bare) merged.delete(n);
      let list = [...merged].filter((n) => !covered(n, merged));
      list = list.filter((n) => {
        const was = owner.get(n);
        if (was && was.id !== s.id) { if (was.bundle !== bid) clashes.push(`${n}: ${was.id} keeps, ${s.id} loses`); return false; }
        owner.set(n, { id: s.id, bundle: bid });
        return true;
      }).sort();
      nameLines.push(s.id + " " + list.join(" "));

      const nets = [];
      for (const url of s.nets || []) {
        if (!url.startsWith("http")) { nets.push(url); continue; }
        const text = await (await fetch(url)).text();
        for (const line of text.split("\n")) {
          const one = line.split("#")[0].trim();
          if (/^[0-9a-f:.]+\/\d+$/i.test(one)) nets.push(one);
        }
      }
      if (nets.length) netLines.push(s.id + " " + nets.join(" "));
      if (!list.length && !nets.length) empty.push(s.id);
      report.push(`${bid}/${s.id}: ${list.length}${nets.length ? " +" + nets.length + " nets" : ""}`);
    }
  }
  fs.writeFileSync("internal/steerlist/names.txt", nameLines.join("\n") + "\n");
  fs.writeFileSync("internal/steerlist/nets.txt", netLines.join("\n") + "\n");

  const q = (v) => JSON.stringify(v);
  let go = "package steerlist\n\nvar Services = []Service{\n";
  for (const [, , services] of PLAN) for (const s of services) go += `\t{ID: ${q(s.id)}, Name: ${q(s.name)}},\n`;
  go += "}\n\nvar Bundles = []Bundle{\n";
  for (const [bid, bname, services] of PLAN) {
    go += `\t{ID: ${q(bid)}, Name: ${q(bname)}, Members: []string{\n`;
    const ids = services.map((s) => q(s.id));
    for (let i = 0; i < ids.length; i += 6) go += "\t\t" + ids.slice(i, i + 6).join(", ") + ",\n";
    go += "\t}},\n";
  }
  go += "}\n";
  fs.writeFileSync("internal/steerlist/catalog.go", go);

  const dir = "panel/src/assets/services/";
  const noIcon = [];
  for (const [, , services] of PLAN) {
    for (const s of services) {
      if (!s.site || [".png", ".svg", ".mono.svg"].some((ext) => fs.existsSync(dir + s.id + ext))) continue;
      try {
        const res = await fetch("https://www.google.com/s2/favicons?sz=64&domain=" + s.site);
        const body = Buffer.from(await res.arrayBuffer());
        if (!res.ok || body.length < 200) { noIcon.push(s.id); continue; }
        fs.writeFileSync(dir + s.id + ".png", body);
      } catch { noIcon.push(s.id); }
    }
  }

  console.log(report.join(" | "));
  console.log("services:", report.length, "| empty:", empty.join(" ") || "none", "| no icon:", noIcon.join(" ") || "none");
  console.log("cross-bundle clashes:", clashes.length, clashes.slice(0, 25).join("; "));
})();
