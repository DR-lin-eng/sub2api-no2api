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

Last reviewed: 2026-10-27 against the official OpenAI model and pricing pages.
The bundled cards include GPT-6.1 Sol and Terra, with standard, 30-minute cache
write, cache read, long-context, Batch, Flex and Fast rates. Astra and GPT-6.1 Sol
also include the official Ultrafast card (6x Standard). Custom catalog base rates
retain precedence. See [GPT-6 models and pricing](../../../docs/GPT6_MODELS.md)
for supported billing paths and the 12-hour cache-write limitation.

The Grok 4.7 Fast and Grok Build Fast entries follow the official xAI Fast card.
Explicit `long_context_input_token_threshold` and multiplier fields are included
for this project parser; the inherited xAI threshold is inclusive at 200000 tokens.
See [PR #8170 review](../../../docs/UPSTREAM_SYNC_20261022_PR8170.md).
