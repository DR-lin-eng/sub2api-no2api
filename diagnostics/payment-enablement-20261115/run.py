#!/usr/bin/env python3
"""Run payment tests in Docker against disposable PostgreSQL and Redis."""
import argparse
from pathlib import Path
import subprocess
import time
import uuid

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--run", default="^TestPayment.*Postgres$")
args = parser.parse_args()
repo = Path(__file__).resolve().parents[2]
name = "sub2api-payment-enablement-" + uuid.uuid4().hex[:8]
created = []


def docker(*command):
    return subprocess.check_output(["docker", *command], text=True).strip()


docker("network", "create", name)
try:
    postgres = name + "-postgres"
    redis = name + "-redis"
    for container, options, image in [
        (postgres, ["-e", "POSTGRES_PASSWORD=fixture-only", "-e", "POSTGRES_DB=sub2api_test"], "postgres:18.1-alpine3.23"),
        (redis, [], "redis:8.4-alpine"),
    ]:
        docker("run", "-d", "--name", container, "--network", name, *options, image)
        created.append(container)
    for _ in range(60):
        if subprocess.run(["docker", "exec", postgres, "pg_isready", "-U", "postgres"],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            break
        time.sleep(0.5)
    else:
        raise RuntimeError("disposable PostgreSQL did not become ready")
    result = subprocess.run([
        "docker", "run", "--rm", "--network", name, "--cpus=2", "--memory=4g",
        "-v", str(repo) + ":/src:ro", "-w", "/src/backend",
        "-v", "sub2api-followup-gomod:/go/pkg/mod",
        "-v", "sub2api-followup-gocache:/root/.cache/go-build",
        "-e", "SUB2API_TEST_EXTERNAL_POSTGRES_DSN=postgres://postgres:fixture-only@" + postgres + ":5432/sub2api_test?sslmode=disable",
        "-e", "SUB2API_TEST_EXTERNAL_REDIS_ADDR=" + redis + ":6379",
        "-e", "CI=true", "-e", "CGO_ENABLED=0", "-e", "GOMAXPROCS=2",
        "-e", "GOMEMLIMIT=1600MiB", "-e", "GOFLAGS=-p=1", "-e", "GOTOOLCHAIN=local",
        "golang:1.26.11-alpine", "go", "test", "-tags=integration",
        "./internal/infrastructure/repository", "-run", args.run, "-count=1", "-timeout=10m", "-v",
    ])
    raise SystemExit(result.returncode)
finally:
    for container in reversed(created):
        subprocess.run(["docker", "rm", "-f", container], stdout=subprocess.DEVNULL, check=False)
    subprocess.run(["docker", "network", "rm", name], stdout=subprocess.DEVNULL, check=False)
