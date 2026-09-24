package steerlist

import "strings"

type Service struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Site    string   `json:"site"`
	Note    string   `json:"note,omitempty"`
	Entries []string `json:"entries"`
}

var Services = []Service{
	{ID: "openai", Name: "OpenAI", Site: "openai.com", Note: "ChatGPT, Sora, Codex, API", Entries: []string{
		"openai.com", "chatgpt.com", "chat.com", "sora.com", "oaistatic.com", "oaiusercontent.com", "openai.org",
		"openaiapi-site.azureedge.net", "openaicom-api-bdcpf8c6d2e9atf6.z01.azurefd.net",
		"openaicomproductionae4b.blob.core.windows.net", "production-openaicom-storage.azureedge.net",
		"o33249.ingest.sentry.io", "livekit.cloud", "featuregates.org", "featureassets.org", "prodregistryv2.org",
		"statsigapi.net", "intercom.io", "intercomcdn.com", "browser-intake-datadoghq.com", "revenuecat.com",
		"cdn.auth0.com",
	}},
	{ID: "anthropic", Name: "Claude", Site: "claude.ai", Note: "Claude, Claude Code, Anthropic API", Entries: []string{
		"anthropic.com", "claude.ai", "claude.com", "clau.de", "claudeusercontent.com", "servd-anthropic-website.b-cdn.net",
	}},
	{ID: "gemini", Name: "Gemini", Site: "gemini.google.com", Note: "Gemini, AI Studio, NotebookLM, Labs, Jules", Entries: []string{
		"gemini.google.com", "gemini.google", "bard.google.com", "aistudio.google.com", "makersuite.google.com",
		"ai.google.dev", "notebooklm.google.com", "notebooklm.google", "labs.google", "labs.google.com",
		"jules.google", "jules.google.com", "deepmind.google", "deepmind.com", "generativelanguage.googleapis.com",
		"alkalimakersuite-pa.clients6.google.com", "webchannel-alkalimakersuite-pa.clients6.google.com",
		"alkalicore-pa.clients6.google.com", "aisandbox-pa.googleapis.com", "robinfrontend-pa.googleapis.com",
		"proactivebackend-pa.googleapis.com", "assistant-s3-pa.googleapis.com", "geller-pa.googleapis.com",
		"cloudcode-pa.googleapis.com", "daily-cloudcode-pa.googleapis.com", "aida.googleapis.com",
	}},
	{ID: "copilot", Name: "Microsoft Copilot", Site: "copilot.microsoft.com", Entries: []string{
		"copilot.microsoft.com", "copilot.cloud.microsoft", "sydney.bing.com", "edgeservices.bing.com",
	}},
	{ID: "github-copilot", Name: "GitHub Copilot", Site: "github.com", Entries: []string{
		"githubcopilot.com", "copilot-proxy.githubusercontent.com", "copilot-telemetry.githubusercontent.com",
		"origin-tracker.githubusercontent.com", "default.exp-tas.com",
	}},
	{ID: "grok", Name: "Grok", Site: "grok.com", Entries: []string{"x.ai", "grok.com"}},
	{ID: "perplexity", Name: "Perplexity", Site: "perplexity.ai", Entries: []string{
		"perplexity.ai", "perplexity.com", "pplx.ai", "pplx-res.cloudinary.com",
	}},
	{ID: "mistral", Name: "Mistral", Site: "mistral.ai", Entries: []string{"mistral.ai"}},
	{ID: "poe", Name: "Poe", Site: "poe.com", Entries: []string{"poe.com", "poecdn.net"}},
	{ID: "character-ai", Name: "Character.AI", Site: "character.ai", Entries: []string{"character.ai"}},
	{ID: "midjourney", Name: "Midjourney", Site: "midjourney.com", Entries: []string{"midjourney.com"}},
	{ID: "suno", Name: "Suno", Site: "suno.com", Entries: []string{"suno.com", "suno.ai"}},
	{ID: "udio", Name: "Udio", Site: "udio.com", Entries: []string{"udio.com"}},
	{ID: "elevenlabs", Name: "ElevenLabs", Site: "elevenlabs.io", Entries: []string{"elevenlabs.io"}},
	{ID: "runway", Name: "Runway", Site: "runwayml.com", Entries: []string{"runwayml.com"}},
	{ID: "pika", Name: "Pika", Site: "pika.art", Entries: []string{"pika.art"}},
	{ID: "leonardo", Name: "Leonardo", Site: "leonardo.ai", Entries: []string{"leonardo.ai"}},
	{ID: "ideogram", Name: "Ideogram", Site: "ideogram.ai", Entries: []string{"ideogram.ai"}},
	{ID: "cursor", Name: "Cursor", Site: "cursor.com", Entries: []string{"cursor.com", "cursor.sh", "cursorapi.com"}},
	{ID: "windsurf", Name: "Windsurf", Site: "windsurf.com", Entries: []string{"codeium.com", "windsurf.com"}},
	{ID: "jetbrains", Name: "JetBrains", Site: "jetbrains.com", Entries: []string{"jetbrains.com", "jetbrains.ai", "jb.gg"}},
	{ID: "docker", Name: "Docker Hub", Site: "docker.com", Entries: []string{"docker.com", "docker.io"}},
	{ID: "hashicorp", Name: "HashiCorp", Site: "hashicorp.com", Entries: []string{
		"hashicorp.com", "terraform.io", "vagrantup.com", "vagrantcloud.com",
	}},
	{ID: "notion", Name: "Notion", Site: "notion.so", Entries: []string{
		"notion.so", "notion.com", "notion.site", "notion.new", "notion-static.com", "notionusercontent.com",
	}},
	{ID: "canva", Name: "Canva", Site: "canva.com", Entries: []string{"canva.com"}},
	{ID: "grammarly", Name: "Grammarly", Site: "grammarly.com", Entries: []string{"grammarly.com", "grammarly.io"}},
	{ID: "spotify", Name: "Spotify", Site: "spotify.com", Entries: []string{
		"spotify.com", "spotify.net", "scdn.co", "spotifycdn.com", "spotifycdn.net", "spotify.link", "spoti.fi",
		"spotify.design", "spotifycharts.com", "byspotify.com", "tospotify.com", "pscdn.co", "spotilocal.com",
		"audio-ak-spotify-com.akamaized.net", "audio4-ak-spotify-com.akamaized.net",
		"heads-ak-spotify-com.akamaized.net", "heads4-ak-spotify-com.akamaized.net",
	}},
	{ID: "netflix", Name: "Netflix", Site: "netflix.com", Entries: []string{
		"netflix.com", "netflix.net", "nflxext.com", "nflximg.com", "nflximg.net", "nflxso.net", "nflxvideo.net",
		"nflxsearch.net", "fast.com",
	}},
	{ID: "discord", Name: "Discord", Site: "discord.com", Entries: []string{
		"discord.com", "discord.gg", "discord.co", "discord.dev", "discord.new", "discord.gift", "discord.store",
		"discord.media", "discordapp.com", "discordapp.net", "discordcdn.com", "discordstatus.com",
		"discordactivities.com", "discordsays.com", "dis.gd", "discord-attachments-uploads-prd.storage.googleapis.com",
	}},
	{ID: "meta", Name: "Instagram, Facebook", Site: "instagram.com", Note: "Instagram, Facebook, Threads, Messenger, Meta AI", Entries: []string{
		"instagram.com", "cdninstagram.com", "instagr.am", "ig.me", "facebook.com", "facebook.net", "fb.com",
		"fb.me", "fb.gg", "fbcdn.net", "fbsbx.com", "messenger.com", "m.me", "threads.net", "threads.com",
		"meta.com", "meta.ai", "oculus.com", "oculuscdn.com",
	}},
	{ID: "whatsapp", Name: "WhatsApp", Site: "whatsapp.com", Entries: []string{"whatsapp.com", "whatsapp.net", "wa.me"}},
	{ID: "x", Name: "X", Site: "x.com", Entries: []string{
		"x.com", "twitter.com", "twimg.com", "t.co", "twttr.com", "tweetdeck.com", "twitterstat.us",
		"ads-twitter.com", "pscp.tv", "periscope.tv",
	}},
	{ID: "linkedin", Name: "LinkedIn", Site: "linkedin.com", Entries: []string{"linkedin.com", "licdn.com", "lnkd.in"}},
	{ID: "signal", Name: "Signal", Site: "signal.org", Entries: []string{
		"signal.org", "whispersystems.org", "signal.art", "signal.group", "signal.me",
	}},
	{ID: "viber", Name: "Viber", Site: "viber.com", Entries: []string{"viber.com"}},
	{ID: "medium", Name: "Medium", Site: "medium.com", Entries: []string{"medium.com"}},
	{ID: "patreon", Name: "Patreon", Site: "patreon.com", Entries: []string{"patreon.com", "patreonusercontent.com"}},
	{ID: "telegram-fi", Name: "Telegram · Helsinki", Site: "telegram.org", Note: "Telegram networks some Russian hosts cannot reach", Entries: []string{
		"91.105.192.0/23", "185.76.151.0/24", "2a0a:f280:203::/48",
	}},
	{ID: "youtube", Name: "YouTube", Site: "youtube.com", Note: "Heavy on the exit node", Entries: []string{
		"youtube.com", "youtu.be", "yt.be", "ytimg.com", "googlevideo.com", "youtube-nocookie.com", "youtubekids.com",
		"youtubegaming.com", "youtubei.googleapis.com", "youtube.googleapis.com",
		"youtubeembeddedplayer.googleapis.com", "jnn-pa.googleapis.com", "ggpht.com", "yt3.googleusercontent.com",
		"yt4.googleusercontent.com",
	}},
}

func Expand(ids string) string {
	want := map[string]bool{}
	for _, id := range strings.FieldsFunc(ids, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
		want[id] = true
	}
	var b strings.Builder
	for _, s := range Services {
		if !want[s.ID] {
			continue
		}
		for _, e := range s.Entries {
			b.WriteString(e)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
