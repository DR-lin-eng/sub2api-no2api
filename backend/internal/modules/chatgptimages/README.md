# ChatGPT App images

Pure request and stream decoding for the 20261107 ChatGPT App Images workflow.
The application adapter owns credentials, account egress, uploads, downloads,
SSE/topic recovery, API projection and reliable billing. This module performs no I/O.

The App conversation model and its internal image model are separate identities.
Input attachments and unfinished image placeholders are never returned as generated images.
