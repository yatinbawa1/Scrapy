# Scrapy

> **An agentic, AI-powered wallpaper manager for macOS, Windows, and Linux.**
> Built and iterated by an AI coding agent. All AI runs **on your hardware** — no cloud, no telemetry, fully offline and private.

Scrapy is a high-performance desktop app that scrapes wallpapers from
multiple sources, downloads and caches them locally, analyzes every image with
on-device AI, and lets you search, filter, favorite, and set wallpapers across
**all of your monitors** with a single click.

---

## Why "agentic"?

This project was designed, scaffolded, and evolved **agentically** — an AI agent
drove the architecture, implemented the backend and frontend, wired up the
cross-platform wallpaper engine, and set up the continuous-build pipeline. The
result is a clean, extensible Go codebase (`internal/` packages) paired with a
Svelte frontend, packaged natively for every desktop OS.

## On-hardware AI (no cloud)

Every AI feature runs **locally on your machine**:

- **Embeddings / semantic search** — a dependency-free heuristic embedder runs
  entirely on-CPU to power natural-language search and "find similar".
- **Optional CLIP model** — build with `-tags clip` to use a real
  [CLIP](https://openai.com/research/clip) vision-language model via
  [onnxruntime](https://onnxruntime.ai/) (downloaded once on first run). The
  model runs **on your hardware** (CPU) and never contacts a server.
- **Image analysis** — dominant color extraction, brightness/contrast/sharpness,
  aesthetic scoring, and perceptual hashing (duplicate detection) all happen
  locally.

Your wallpapers and the data derived from them **never leave your computer.**

---

## Features

### Sourcing & download
- **Multi-source scraping** — Wallhaven, Unsplash, and Pexels, each behind a
  pluggable provider interface.
- **Concurrent downloader** — worker pool, retry logic, resume support,
  SHA-256 hash verification, and live progress reporting.
- **Local cache & thumbnails** — originals and 256px / 512px thumbnails stored
  on disk, with configurable cache-size limits and cleanup.
- **Onboarding flow** — pick a storage folder, enable sources, set concurrency.

### AI analysis (all on-device)
- **Metadata extraction** (dimensions, aspect ratio, format, file size).
- **Dominant color** extraction and human-readable color names.
- **Quality metrics** — brightness, contrast, sharpness, and an aesthetic score.
- **Perceptual hashing** for duplicate detection across your library.
- **Auto tags & categories** generated from visual features and source metadata.
- **Vector embeddings** for semantic search and similarity (heuristic by
  default; CLIP when built with `-tags clip`).

### Search & discovery
- **Semantic search** — type natural language and rank by meaning, with a
  hybrid boost so user tags surface exactly.
- **Find similar** — discover visually related wallpapers by embedding distance.
- **Collections** — auto-generated groups (Dark, Light, by category, by color).
- **Search by color** — pick a color and rank by nearest dominant color.
- **Filters** — resolution, aspect ratio, source, favorites, and custom labels.
- **Infinite-scroll grid** with lazy-loaded thumbnails for 20k+ wallpapers.

### Organization
- **Favorites** with one-click toggle.
- **Custom labels** per wallpaper that re-embed and become searchable.
- **Duplicate detection & removal** — clusters near-duplicates and keeps the best.
- **Storage management** — view sizes/counts and clear cache, thumbnails,
  downloads, or the full database.

### Wallpaper setting (cross-platform, all monitors)
- Sets the wallpaper on **every desktop/display**:
  - **macOS** — applies to all desktops via System Events.
  - **Linux** — probes the desktop environment (GNOME/Unity/Cinnamon, KDE,
    XFCE, LXDE, feh) and sets across all monitors.
  - **Windows** — spans a single image across all monitors via the registry +
    `SystemParametersInfo`.
- Per-display targeting is supported where the environment allows.

### Settings & stats
- Download location, concurrent downloads, cache-size limit, enabled sources,
  custom search terms, and theme.
- Live stats: total wallpapers, downloaded count, cache size, source/category
  breakdowns, and background-analysis progress (pause/resume).

---

## Download

Prebuilt binaries for **macOS (Apple Silicon)**, **Windows**, and **Linux** are
attached to the
[GitHub Releases](https://github.com/yatinbawa1/Scrapy/releases).

| Platform | Asset |
| -------- | ----- |
| macOS    | `scrapy-macos.zip` |
| Windows  | `scrapy-windows.zip` |
| Linux    | `scrapy-linux.zip` |

Unzip and run the app. On first launch you'll be walked through onboarding.

---

## Build from source

Requirements: Go 1.25+, Node 18+, and the platform native web dependencies
(see the [Wails docs](https://wails.io/docs/getting-started/installation)).

```bash
# Default build (heuristic on-device embedder, no model download)
wails build

# Optional: real CLIP embeddings (downloads the ONNX model on first run)
wails build -tags clip
```

Cross-platform release artifacts are produced automatically by the GitHub
Actions workflow in `.github/workflows/release.yml` whenever a `v*` tag is pushed.

---

## Architecture

```
Scrapers ─▶ Metadata ─▶ Downloader ─▶ SQLite + Cache
                                      ─▶ Thumbnail Generator
                                      ─▶ AI Analysis (on-device)
                                      ─▶ Search / Semantic / Similar
                                      ─▶ Svelte UI ─▶ Set Wallpaper (all displays)
```

```
internal/
  scraper/      provider engine (wallhaven, unsplash, pexels)
  downloader/   worker pool + retries + hashing
  cache/        local store + size limits
  thumbnail/    thumbnail generation
  image/        colors, quality, duplicate (perceptual hash)
  ai/           heuristic + optional CLIP embeddings, tags, aesthetic
  search/       search, semantic, similarity
  database/     SQLite schema & queries
  wallpaper/    cross-platform setter (darwin/linux/windows)
  config/       settings
  workers/      background analysis pool
```

## License

MIT — see [LICENSE](LICENSE).
