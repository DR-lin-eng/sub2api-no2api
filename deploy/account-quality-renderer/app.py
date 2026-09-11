"""Private CPU classifier. Generated HTML is never served to visitors."""
import asyncio
import base64
import hashlib
import hmac
import io
import os
import re

import torch
import ultralytics
from fastapi import FastAPI, HTTPException, Request
from PIL import Image
from playwright.async_api import async_playwright
from ultralytics import YOLO

MAX_HTML_BYTES = 1 << 20
MAX_ASSET_BYTES = 4 << 20
MODEL_VERSION = "8.4.14"
MODEL_SHA256 = "96bc1abf360ffba879310a0c5d4b4d9d70027083358999ed9fd3daba84fae2b6"
MODEL_PATH = os.getenv("QUALITY_MODEL_PATH", "/models/best.pt")
TOKEN = os.environ.get("ACCOUNT_QUALITY_RENDERER_TOKEN", "")
if not TOKEN:
    raise RuntimeError("ACCOUNT_QUALITY_RENDERER_TOKEN must be set")
if ultralytics.__version__ != MODEL_VERSION:
    raise RuntimeError("classifier runtime version mismatch")
with open(MODEL_PATH, "rb") as source:
    if hashlib.sha256(source.read()).hexdigest() != MODEL_SHA256:
        raise RuntimeError("classifier checkpoint hash mismatch")
torch.set_num_threads(2)
classifier = YOLO(MODEL_PATH, task="classify")
if set(classifier.names.values()) != {"normal", "unnormal"}:
    raise RuntimeError("unexpected classifier labels")
app = FastAPI(title="Sub2API account quality renderer", docs_url=None, redoc_url=None, openapi_url=None)
# One render/inference at a time, rather than an unbounded queue of Chromiums.
processing = asyncio.Semaphore(1)

CSP = ("default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; "
       "img-src data:; font-src data:; connect-src 'none'; frame-src 'none'; "
       "object-src 'none'; base-uri 'none'; form-action 'none'")


def normalize_html(value):
    if not isinstance(value, str) or not value.strip():
        raise HTTPException(422, "missing_html")
    if len(value.encode("utf-8")) > MAX_HTML_BYTES:
        raise HTTPException(413, "html_too_large")
    value = re.sub(r"^\s*```(?:html)?\s*|\s*```\s*$", "", value, flags=re.I)
    lower = value.lower()
    start = lower.find("<!doctype")
    if start < 0:
        start = lower.find("<svg")
    if start > 0:
        value = value[start:]
    if not re.search(r"<svg(?:\s|>)", value, re.I):
        raise HTTPException(422, "missing_svg")
    # Generated animation is allowed to run locally, but all external routes
    # are blocked below and the page has no cookies, storage, or host access.
    return f'<meta http-equiv="Content-Security-Policy" content="{CSP}">' + value


async def render(html):
    """CSS/SMIL animation only: no page scripts, network, cookies or host files."""
    async with async_playwright() as pw:
        browser = await pw.chromium.launch(headless=True, args=["--disable-dev-shm-usage", "--disable-background-networking"])
        try:
            context = await browser.new_context(
                viewport={"width": 896, "height": 672}, device_scale_factor=1,
                java_script_enabled=True, offline=True, service_workers="block",
                accept_downloads=False,
            )
            async def block_external(route):
                await route.abort()
            await context.route("**/*", block_external)
            page = await context.new_page()
            page.set_default_timeout(5000)
            await page.set_content(normalize_html(html), wait_until="domcontentloaded", timeout=5000)
            svg = page.locator("svg").first
            if not await svg.is_visible():
                raise HTTPException(422, "svg_not_visible")
            await page.wait_for_timeout(500)
            frames = []
            for _ in range(16):
                # Fixed viewport, never full_page: hostile CSS cannot create a gigantic image.
                png = await page.screenshot(type="png", full_page=False, timeout=5000)
                frames.append(Image.open(io.BytesIO(png)).convert("RGB"))
                await page.wait_for_timeout(100)
            return frames
        finally:
            await browser.close()


def classify_frames(frames):
    predictions = classifier.predict(source=[frames[0], frames[7], frames[15]], imgsz=224, device="cpu", verbose=False)
    abnormal_index = next(key for key, name in classifier.names.items() if name == "unnormal")
    abnormal = sum(float(item.probs.data[abnormal_index]) for item in predictions) / len(predictions)
    return ("unnormal", abnormal) if abnormal >= 0.5 else ("normal", 1 - abnormal)


def encode_artifacts(frames):
    png_buffer, webp_buffer = io.BytesIO(), io.BytesIO()
    frames[0].save(png_buffer, format="PNG", optimize=True)
    reduced = [frame.resize((640, 480), Image.Resampling.LANCZOS) for frame in frames]
    reduced[0].save(webp_buffer, format="WEBP", save_all=True, append_images=reduced[1:],
                    duration=100, loop=0, quality=82, method=4)
    png, webp = png_buffer.getvalue(), webp_buffer.getvalue()
    if len(png) > MAX_ASSET_BYTES or len(webp) > MAX_ASSET_BYTES:
        raise HTTPException(422, "artifact_too_large")
    return png, webp


@app.get("/health")
async def health():
    return {"status": "ok", "model_version": MODEL_VERSION, "model_sha256": MODEL_SHA256,
            "labels": classifier.names}


@app.post("/v1/process")
async def process(request: Request):
    if not hmac.compare_digest(request.headers.get("Authorization", ""), "Bearer " + TOKEN):
        raise HTTPException(401, "unauthorized")
    raw = bytearray()
    async for chunk in request.stream():
        raw.extend(chunk)
        if len(raw) > MAX_HTML_BYTES * 2:
            raise HTTPException(413, "request_too_large")
    import json
    try:
        data = json.loads(raw)
        html = data["html"]
    except (ValueError, KeyError, TypeError):
        raise HTTPException(422, "invalid_request")
    normalize_html(html)
    try:
        await asyncio.wait_for(processing.acquire(), timeout=30)
    except asyncio.TimeoutError:
        raise HTTPException(429, "renderer_busy")
    try:
        try:
            frames = await asyncio.wait_for(render(html), timeout=25)
            label, confidence = await asyncio.to_thread(classify_frames, frames)
            png, webp = await asyncio.to_thread(encode_artifacts, frames)
        except HTTPException:
            raise
        except Exception:
            # No generated content, file paths or tracebacks in the API response.
            raise HTTPException(422, "render_failed")
    finally:
        processing.release()
    return {"label": label, "confidence": confidence, "model_version": MODEL_VERSION,
            "model_sha256": MODEL_SHA256, "frames": len(frames),
            "png_base64": base64.b64encode(png).decode("ascii"),
            "webp_base64": base64.b64encode(webp).decode("ascii")}
