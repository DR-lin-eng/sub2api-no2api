FROM python:3.11-slim-bookworm
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 QUALITY_MODEL_PATH=/models/best.pt \
    YOLO_CONFIG_DIR=/tmp/ultralytics MPLCONFIGDIR=/tmp/matplotlib PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    OMP_NUM_THREADS=2 OPENBLAS_NUM_THREADS=2
WORKDIR /app
RUN pip install --no-cache-dir torch==2.6.0 torchvision==0.21.0 --index-url https://download.pytorch.org/whl/cpu
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
 && playwright install --with-deps chromium
COPY app.py .
COPY models/best.pt /models/best.pt
RUN useradd --create-home --uid 10001 renderer \
 && mkdir -p /tmp/ultralytics /tmp/matplotlib \
 && chown -R renderer:renderer /tmp/ultralytics /tmp/matplotlib \
 && chmod -R a+rX /app /models /ms-playwright
USER renderer
EXPOSE 8090
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8090", "--workers", "1", "--limit-concurrency", "8", "--no-access-log"]
