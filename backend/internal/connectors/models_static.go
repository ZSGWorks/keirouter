package connectors

import (
	"github.com/mydisha/keirouter/backend/internal/core"
)

// k tags a model with a non-LLM service kind.
func k(id, name string, kind core.ServiceKind) ModelSpec {
	return ModelSpec{ID: id, Name: name, Kind: kind}
}

// emb tags an embedding model with its dimension count.
func emb(id, name string, dims int) ModelSpec {
	return ModelSpec{ID: id, Name: name, Kind: core.ServiceEmbedding, Dimensions: dims}
}

// providerStaticModels maps a provider id to its hardcoded non-LLM models
// (embedding, TTS, STT, image, video, image-to-text, search, fetch).
// LLM models are intentionally absent: LLM discovery is dynamic (live
// provider GET /models first, models.dev snapshot as fallback, then
// user-registered custom models). Providers marked passthrough upstream
// (openrouter, vercel, ...) accept any model id, so their listed set here is
// only a discovery hint, not an allow-list.
var providerStaticModels = map[string][]ModelSpec{
	"openai": {
		emb("text-embedding-3-large", "Text Embedding 3 Large", 3072),
		emb("text-embedding-3-small", "Text Embedding 3 Small", 1536),
		emb("text-embedding-ada-002", "Text Embedding Ada 002", 1536),
		k("tts-1", "TTS-1", core.ServiceTTS), k("tts-1-hd", "TTS-1 HD", core.ServiceTTS),
		k("gpt-4o-mini-tts", "GPT-4o Mini TTS", core.ServiceTTS),
		k("whisper-1", "Whisper 1", core.ServiceSTT), k("gpt-4o-transcribe", "GPT-4o Transcribe", core.ServiceSTT),
		k("gpt-4o-mini-transcribe", "GPT-4o Mini Transcribe", core.ServiceSTT),
		k("gpt-image-1", "GPT Image 1", core.ServiceImage), k("dall-e-3", "DALL-E 3", core.ServiceImage),
		k("dall-e-2", "DALL-E 2", core.ServiceImage),
	},
	"codex": {
		k("gpt-5.5-image", "GPT-5.5 Image", core.ServiceImage), k("gpt-5.4-image", "GPT-5.4 Image", core.ServiceImage),
		k("gpt-5.3-image", "GPT-5.3 Image", core.ServiceImage), k("gpt-5.2-image", "GPT-5.2 Image", core.ServiceImage),
	},
	"anthropic": {
		k("claude-opus-4-5-20251101", "Claude Opus 4.5 (Vision)", core.ServiceImageToText),
	},
	"minimax": {
		k("minimax-image-01", "MiniMax Image 01", core.ServiceImage),
		k("MiniMax-VL-01", "MiniMax VL 01 (Vision)", core.ServiceImageToText),
	},
	"groq": {
		k("whisper-large-v3", "Whisper Large v3", core.ServiceSTT),
		k("whisper-large-v3-turbo", "Whisper Large v3 Turbo", core.ServiceSTT),
		k("distil-whisper-large-v3-en", "Distil Whisper Large v3 EN", core.ServiceSTT),
		k("meta-llama/llama-4-maverick-17b-128e-instruct", "Llama 4 Maverick (Vision)", core.ServiceImageToText),
	},
	"xai": {
		k("grok-2-image-1212", "Grok 2 Image", core.ServiceImage),
		k("grok-2-vision-1212", "Grok 2 Vision", core.ServiceImageToText),
		k("grok-imagine-video", "Grok Imagine Video", core.ServiceVideo),
	},
	"mistral": {
		emb("mistral-embed", "Mistral Embed", 1024),
		k("pixtral-large-latest", "Pixtral Large (Vision)", core.ServiceImageToText),
	},
	"together": {
		emb("BAAI/bge-large-en-v1.5", "BGE Large EN v1.5", 1024),
		emb("togethercomputer/m2-bert-80M-8k-retrieval", "M2 BERT 80M 8K", 768),
	},
	"fireworks": {
		emb("nomic-ai/nomic-embed-text-v1.5", "Nomic Embed Text v1.5", 768),
	},
	"nvidia": {
		emb("nvidia/nv-embedqa-e5-v5", "NV EmbedQA E5 v5", 1024),
		k("nvidia/parakeet-ctc-1.1b-asr", "Parakeet CTC 1.1B", core.ServiceSTT),
		k("fastpitch", "FastPitch", core.ServiceTTS),
		k("tacotron2", "Tacotron2", core.ServiceTTS),
	},
	"xiaomi-tokenplan": {
		k("mimo-v2-tts", "MiMo V2 TTS", core.ServiceTTS), k("mimo-v2.5-tts", "MiMo V2.5 TTS", core.ServiceTTS),
		k("mimo-v2.5-tts-voiceclone", "MiMo V2.5 TTS Voice Clone", core.ServiceTTS), k("mimo-v2.5-tts-voicedesign", "MiMo V2.5 TTS Voice Design", core.ServiceTTS),
	},
	"cloudflare-ai": {
		// Image models
		k("@cf/black-forest-labs/flux-1-schnell", "FLUX.1 Schnell", core.ServiceImage),
		k("@cf/stabilityai/stable-diffusion-xl-base-1.0", "Stable Diffusion XL", core.ServiceImage),
	},
	"kimchi": {
		k("claude-opus-4-6", "Claude Opus 4.6 (Vision)", core.ServiceImageToText),
	},
	"venice": {
		emb("text-embedding-3-large", "Text Embedding 3 Large", 3072),
		emb("text-embedding-bge-m3", "BGE-M3 Embedding", 1024),
		emb("text-embedding-qwen3-8b", "Qwen3 8B Embedding", 4096),
		k("venice-sd35", "Venice SD3.5", core.ServiceImage), k("flux-2-pro", "FLUX.2 Pro", core.ServiceImage),
		k("gpt-image-2", "GPT Image 2 (via Venice)", core.ServiceImage),
	},
	"openrouter": {
		k("openai/gpt-4o", "GPT-4o (Vision)", core.ServiceImageToText),
		k("anthropic/claude-opus-4.5", "Claude Opus 4.5 (Vision)", core.ServiceImageToText),
	},
	"vertex": {
		k("gemini-2.5-pro", "Gemini 2.5 Pro (Vision)", core.ServiceImageToText),
	},
	"vercel-ai-gateway": {
		k("openai/gpt-4o", "GPT-4o (Vision)", core.ServiceImageToText),
	},
	"gemini": {
		emb("gemini-embedding-001", "Gemini Embedding 001", 768), emb("text-embedding-004", "Text Embedding 004", 768),
		k("gemini-2.5-flash-image", "Gemini 2.5 Flash Image (Nano Banana)", core.ServiceImage),
		k("gemini-2.5-pro-preview-tts", "Gemini 2.5 Pro TTS", core.ServiceTTS),
		k("gemini-2.5-pro", "Gemini 2.5 Pro (Vision)", core.ServiceImageToText),
	},
	"fal-ai": {
		k("fal-ai/flux/schnell", "FLUX Schnell", core.ServiceImage), k("fal-ai/flux/dev", "FLUX Dev", core.ServiceImage),
		k("fal-ai/flux-pro/v1.1", "FLUX Pro v1.1", core.ServiceImage),
	},
	"stability-ai": {
		k("stable-image-ultra", "Stable Image Ultra", core.ServiceImage), k("stable-image-core", "Stable Image Core", core.ServiceImage),
		k("sd3.5-large", "Stable Diffusion 3.5 Large", core.ServiceImage),
	},
	"black-forest-labs": {
		k("flux-pro-1.1", "FLUX Pro 1.1", core.ServiceImage), k("flux-pro", "FLUX Pro", core.ServiceImage),
		k("flux-dev", "FLUX Dev", core.ServiceImage),
	},
	"voyage-ai": {
		emb("voyage-3-large", "Voyage 3 Large", 1024), emb("voyage-3.5", "Voyage 3.5", 1024),
		emb("voyage-code-3", "Voyage Code 3", 1024),
	},
	"jina-ai": {
		emb("jina-embeddings-v3", "Jina Embeddings v3", 1024),
		emb("jina-embeddings-v2-base-code", "Jina Embeddings v2 Base Code", 768),
	},
	"nebius": {
		emb("Qwen/Qwen3-Embedding-8B", "Qwen3 Embedding 8B", 4096),
	},
	"deepgram": {
		k("nova-3", "Nova 3", core.ServiceSTT), k("nova-2", "Nova 2", core.ServiceSTT), k("whisper-large", "Whisper Large", core.ServiceSTT),
	},
	"assemblyai": {
		k("universal-3-pro", "Universal 3 Pro", core.ServiceSTT), k("universal-2", "Universal 2", core.ServiceSTT),
	},
	"elevenlabs": {
		k("eleven_multilingual_v2", "Eleven Multilingual v2", core.ServiceTTS), k("eleven_turbo_v2_5", "Eleven Turbo v2.5", core.ServiceTTS),
	},
	"inworld": {
		k("inworld-tts-1.5-mini", "Inworld TTS 1.5 Mini", core.ServiceTTS), k("inworld-tts-1.5-max", "Inworld TTS 1.5 Max", core.ServiceTTS),
	},
	"nanobanana": {
		k("nanobanana-flash", "NanoBanana Flash", core.ServiceImage), k("nanobanana-pro", "NanoBanana Pro", core.ServiceImage),
	},
	"tavily": {
		k("tavily-search", "Tavily Search", core.ServiceSearch), k("tavily-extract", "Tavily Extract", core.ServiceFetch),
	},
	"brave-search": {
		k("brave-search", "Brave Search", core.ServiceSearch),
	},
	"serper": {
		k("serper-search", "Serper Search", core.ServiceSearch),
	},
	"exa": {
		k("exa-search", "Exa Search", core.ServiceSearch), k("exa-contents", "Exa Contents", core.ServiceFetch),
	},
	"searxng": {
		k("searxng-search", "SearXNG Search", core.ServiceSearch),
	},
	"firecrawl": {
		k("firecrawl-scrape", "Firecrawl Scrape", core.ServiceFetch),
	},
	"jina-reader": {
		k("jina-reader", "Jina Reader", core.ServiceFetch),
	},
}
