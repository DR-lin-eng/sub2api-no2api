# Model Pricing Data

This directory contains a local copy of the mirrored model pricing data as a fallback mechanism.

## Source
The original file is maintained by the LiteLLM project and mirrored into the `price-mirror` branch of this repository via GitHub Actions:
- Mirror branch (configurable via `PRICE_MIRROR_REPO`): https://raw.githubusercontent.com/<your-repo>/price-mirror/model_prices_and_context_window.json
- Upstream source: https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json

## Purpose
This local copy serves as a fallback when the remote file cannot be downloaded due to:
- Network restrictions
- Firewall rules
- DNS resolution issues
- GitHub being blocked in certain regions
- Docker container network limitations

## Update Process
The pricingService will:
1. First attempt to download the latest version from GitHub
2. If download fails, use this local copy as fallback
3. Log a warning when using the fallback file

## Manual Update
To manually update this file with the latest pricing data (if automation is unavailable):
```bash
curl -s https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json -o model_prices_and_context_window.json
```

## File Format
The file contains JSON data with model pricing information including:
- Model names and identifiers
- Input/output token costs
- Context window sizes
- Model capabilities

Last reviewed: 2026-11-23 for Gemini 4 Neon against Google's model and pricing pages.
OpenAI pricing cards were last reviewed on 2026-11-18.
The bundled cards include GPT-6.2 Astra/Sol with Standard, 30-minute cache
write, cache read, long-context, Batch and Fast rates. Their Ultrafast promotion
is 3x Standard through February 28, 2027 Pacific time, then 6x from March 1
(08:00 UTC); billing uses the pricing instant and applies the switch automatically.
The raw snapshot records current Ultrafast prices, list prices and the promotion
end. GPT-6 Astra and GPT-6.1 Sol retain their 6x Ultrafast card. Older supported
models retain Flex rates. Custom catalog base rates retain precedence. See [GPT-6 models and pricing](../../../docs/GPT6_MODELS.md)
for supported billing paths and the 12-hour cache-write limitation.

The Grok 4.7 Fast and Grok Build Fast entries follow the official xAI Fast card.
Explicit `long_context_input_token_threshold` and multiplier fields are included
for this project parser; the inherited xAI threshold is inclusive at 200000 tokens.
See [PR #8170 review](../../../docs/UPSTREAM_SYNC_20261022_PR8170.md).

Gemini 4 Argon and Carbon use standard non-promotional rates in this gateway.
Both are $4/$20/$0.20 per million input/output/cache-read tokens, with Priority
at $7.20/$36/$0.36. The default feed correction upgrades recognized old promo
cards before publishing them. Custom and pinned feeds retain their configured
rates.

Gemini 4 Neon was checked against the official [model card](https://ai.google.dev/gemini-api/docs/models/gemini-4-neon)
and [price card](https://ai.google.dev/gemini-api/docs/pricing#gemini-4-neon) on 2026-11-23.
The bundled synchronous rates per million tokens are $3/$15/$0.15 for standard,
$5.40/$27/$0.27 for priority, and $1.50/$7.50/$0.075 for flex, ordered as
input/output/cache-read. This gateway card uses the synchronous 65,536-token
output limit; batch and long-decode pricing are separate API paths.
Search and Maps grounding are provider-billed per-query add-ons. The gateway's
local settlement records token usage and does not include those add-on charges.

Gemini 3.7 and 3.8 Flash use $1.50/$7.50/$0.15 per million input/output/cache-read
tokens. Recognized promo cards in the default feed are corrected before
publication, and static fallback cards carry the same standard rates.
