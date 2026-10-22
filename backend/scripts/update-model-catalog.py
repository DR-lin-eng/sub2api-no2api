#!/usr/bin/env python3
"""Generate discovery fallback metadata from the repository's reviewed price resource."""
import json
from pathlib import Path

BACKEND = Path(__file__).resolve().parents[1]
SOURCE = BACKEND / 'resources/model-pricing/model_prices_and_context_window.json'
TARGET = BACKEND / 'internal/modules/modelcatalog/fallbacks.json'
prices = json.loads(SOURCE.read_text())
catalog = {}
for model, price in prices.items():
    if not isinstance(price, dict) or model == 'sample_spec':
        continue
    context = price.get('context_window') or price.get('max_input_tokens')
    if not isinstance(context, int) or isinstance(context, bool) or context <= 0:
        continue
    descriptor = {'context_window': context, 'max_context_window': context}
    output = price.get('max_output_tokens')
    if isinstance(output, int) and output > 0:
        descriptor['max_output_tokens'] = output
    for source, target in [('supports_reasoning', 'supports_reasoning'),
                           ('supports_parallel_function_calling', 'supports_parallel_tool_calls'),
                           ('supports_computer_use', 'supports_computer_use'),
                           ('supports_web_search', 'supports_web_search')]:
        if isinstance(price.get(source), bool):
            descriptor[target] = price[source]
    if isinstance(price.get('supports_vision'), bool):
        descriptor['input_modalities'] = ['text', 'image'] if price['supports_vision'] else ['text']
    if descriptor.get('supports_reasoning'):
        levels = ['low', 'medium', 'high']
        if price.get('supports_none_reasoning_effort'):
            levels.insert(0, 'none')
        if price.get('supports_xhigh_reasoning_effort'):
            levels.append('xhigh')
        if price.get('supports_max_reasoning_effort'):
            levels.append('max')
        descriptor['supported_reasoning_levels'] = levels
        descriptor['default_reasoning_level'] = 'medium'
    if model == 'gpt-6.1-sol':
        # Codex's conservative working window is distinct from provider max.
        descriptor['context_window'] = 500000
    if model == 'claude-sonnet-5-5':
        descriptor['default_reasoning_level'] = 'high'
    catalog[model.lower()] = descriptor
TARGET.write_text(json.dumps(catalog, ensure_ascii=False, indent=2, sort_keys=True) + '\n')
print(f'{TARGET}: {len(catalog)} descriptors')
