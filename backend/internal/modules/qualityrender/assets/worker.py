"""Embedded quality worker. One bounded stdin request, one raster/classifier result."""
import asyncio
import base64
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import re
import sys

MAX_HTML_BYTES = 1 << 20
MAX_ASSET_BYTES = 4 << 20
MODEL_VERSION = "8.4.14"
MODEL_SHA256 = "96bc1abf360ffba879310a0c5d4b4d9d70027083358999ed9fd3daba84fae2b6"

class RenderError(Exception):
    def __init__(self, status, code):
        super().__init__(code)
        self.code = code

CSP = ("default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; "
       "img-src data:; font-src data:; connect-src 'none'; frame-src 'none'; "
       "object-src 'none'; base-uri 'none'; form-action 'none'")


def normalize_html(value):
    if not isinstance(value, str) or not value.strip():
        raise RenderError(422, "missing_html")
    if len(value.encode("utf-8")) > MAX_HTML_BYTES:
        raise RenderError(413, "html_too_large")
    value = re.sub(r"^\s*```(?:html)?\s*|\s*```\s*$", "", value, flags=re.I)
    lower = value.lower()
    start = lower.find("<!doctype")
    if start < 0:
        start = lower.find("<svg")
    if start > 0:
        value = value[start:]
    if not re.search(r"<svg(?:\s|>)", value, re.I):
        raise RenderError(422, "missing_svg")
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
                raise RenderError(422, "svg_not_visible")
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
        raise RenderError(422, "artifact_too_large")
    return png, webp



def main():
    raw = sys.stdin.buffer.read(MAX_HTML_BYTES * 6 + 1)
    if len(raw) > MAX_HTML_BYTES * 6:
        raise RenderError(413, "request_too_large")
    data = json.loads(raw)
    html = data.get("html", "")
    normalize_html(html)
    model = Path(__file__).with_name("best.pt")
    if hashlib.sha256(model.read_bytes()).hexdigest() != MODEL_SHA256:
        raise RenderError(500, "model_hash_mismatch")
    # Third-party imports/inference may log to stdout. Keep the protocol clean.
    with contextlib.redirect_stdout(sys.stderr):
        global Image, async_playwright, classifier
        from PIL import Image
        from playwright.async_api import async_playwright
        import torch
        import ultralytics
        from ultralytics import YOLO
        if ultralytics.__version__ != MODEL_VERSION:
            raise RenderError(500, "model_runtime_mismatch")
        torch.set_num_threads(2)
        classifier = YOLO(str(model), task="classify")
        if set(classifier.names.values()) != {"normal", "unnormal"}:
            raise RenderError(500, "invalid_model_labels")
        frames = asyncio.run(asyncio.wait_for(render(html), timeout=30))
        label, confidence = classify_frames(frames)
        png, webp = encode_artifacts(frames)
    print(json.dumps({"label": label, "confidence": confidence, "model_version": MODEL_VERSION,
                      "png_base64": base64.b64encode(png).decode("ascii"),
                      "webp_base64": base64.b64encode(webp).decode("ascii")}))

if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        # No generated HTML, environment, filesystem paths or stack traces.
        code = exc.code if isinstance(exc, RenderError) else "local_render_failed"
        print(json.dumps({"error": code}))
        sys.exit(1)
