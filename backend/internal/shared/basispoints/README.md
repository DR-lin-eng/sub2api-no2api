# Structured output compatibility

The adapter accepts Responses requests using text.format.type=json_object or
json_schema. BPS rejects native text/response_format request fields, so the
adapter sends the output requirements as developer instructions and validates
the final response locally. This does not provide upstream constrained decoding.

- Preserve the requested model and account. Model-access rejections remain
  upstream errors; structured output support does not grant model permissions.
- Return a successful final answer only when it is valid JSON and, for
  json_schema, satisfies the supplied schema. Do not retry, repair, strip fences,
  or replace invalid model output with fabricated data.
- Withhold structured message text until the terminal response is validated.
  Reconstruct message events from that validated response, rather than replaying
  unvalidated deltas. Ordinary text requests remain incremental.
- Preserve tool continuations and explicit refusals as protocol items, outside
  the final-answer JSON contract. Preserve upstream failure/incomplete status
  without exposing partial structured message text.
- Resolve schema references inside the submitted document only. Never fetch
  remote schemas or read local files. Limit schemas to 1 MiB and answer text to
  16 MiB, subject to the existing SSE event limit.

# Tool history and input boundaries

- OAuth models routed through BPS do not advertise native encrypted multi-agent
  capabilities. Apply this limit to generated, fetched and pinned group catalogs
  by the actual mapped route, and recalculate the client ETag. Plaintext client
  tools remain available; refresh the catalog and start a new conversation to
  leave an old client-side v2 override or encrypted history behind.
- Rebuild complete CUSTOM calls using their current catalog's exact
  `codex2api.custom/NAME` marker and raw input. Preserve cached native calls
  verbatim. Retain unavailable historical tools as recorded history; never
  infer a new tool target from executable text.
- Require a matching complete call or scoped replay-cache entry for each tool
  result. Report the input path when neither is available. Do not invent calls
  or discard their results.
- Reject encrypted message parts with an `encrypted_content` diagnostic and
  input path. Do not reinterpret ciphertext as plaintext. Top-level encrypted
  reasoning items retain their existing handling.
- The OAuth gateway retries once when HTTP 400 specifically rejects encrypted
  content and the prepared history can recover by removing opaque reasoning
  items only. It keeps the same account, model, effort, proxy, attachments and
  conversation metadata, and closes the rejected response before retrying.
  It never drops encrypted messages, tool results or compaction context. A
  second rejection returns `invalid_encrypted_content` with plaintext-history
  recovery guidance; ordinary 400s and transport failures do not use this retry.
- Preserve `detail: original` on HTTPS images and inline images rewritten by
  the relay. Let the upstream model validate its supported detail levels; do
  not silently downgrade the requested detail.
- Enforce the existing 20-inline-image and 32 MiB per-request relay limits.
  A relay capacity error and an upstream overload are separate from a tool
  protocol error; HTTP 200 alone does not establish a successful SSE terminal.

# Tool transport corrections

- Validate a complete native tool batch before committing replay entries or
  emitting any client tool events. Never evaluate transport code in the gateway.
- When a completed batch consists entirely of identifiable run_officejs calls
  with valid outer arguments, return local validation errors as native tool
  results with `executed: false`. Continue on the same BPS account, model, effort,
  proxy and scoped conversation, with at most two corrective model requests.
- Preserve intended operations and call order. Require the corrected batch to
  have the same number of calls and pass the current catalog's transport checks.
  Reject changes to previously valid operations. For an invalid raw call corrected
  to an explicit CUSTOM, FUNCTION_CODE or FUNCTION_CMD transport, bind its code field to the
  original bytes before checking the operation and size limits. Never dispatch
  source text rewritten by the correction model, including whitespace changes.
  Store the bound native call in replay history so later turns see the exact
  operation dispatched to the client. Do not bind named JSON envelopes or infer
  targets from source text; retain the explicit target and whole-batch checks.
  Keep ordinary text incremental and retain its original response identity and
  output indexes. Hide intermediate correction text and native tool events.
- Release the preceding response body before a correction request so accounts
  with concurrency one do not deadlock. Cancel corrections on client disconnect.
  Include all returned terminal usage in the final success or failure response.
- This formatting path does not retry transport I/O failures, upstream rejections, incomplete streams, undeclared tool targets, unsupported native tools, missing call identities, function argument schema failures or structured answer failures. Preserve any explicit declared target on correction.
  Do not infer a tool target from arbitrary raw code. Stop on a changed batch or
  exhausted correction limit without dispatching any client tools.
- Treat this as bounded model correction, not constrained upstream decoding or
  a guarantee that arbitrary malformed model output will always recover.

# First-turn unknown-target correction

The gateway separately permits one regeneration when the first tool interaction ends in exactly one undeclared run_officejs target at the end of a completed response. It must have no prior tool calls/results and no dispatched client tool. This path reuses the prepared request, current catalog, account, model, proxy and attachment IDs; it does not append a fabricated executed tool result. Its corrected response must pass the current catalog, argument schema, identity and parallel-call checks. A second unknown target, invalid arguments or incomplete response fails without dispatching tools. Both attempts' reported usage is retained, including progressive usage if the correction disconnects before its terminal event.

This path and the existing known-target formatting path are selected independently. A function argument schema error alone does not trigger either path.
