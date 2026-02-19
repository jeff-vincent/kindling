#!/usr/bin/env python3
"""
fix-dockerfile.py — LLM-powered Dockerfile analysis and repair.

Before building a Docker image in the fuzz harness, this script sends the
Dockerfile (plus build-context metadata) to an LLM and asks it to produce
a corrected version that will actually build.

It can also be called in "retry" mode: after a build failure, pass the
error log and it will attempt a targeted fix.

Usage:
  # Pre-build analysis & fix
  fix-dockerfile.py --dockerfile path/to/Dockerfile --context-dir path/to/dir

  # Retry after build failure
  fix-dockerfile.py --dockerfile path/to/Dockerfile --context-dir path/to/dir \
                    --build-error "COPY failed: file not found ..."

Output: The fixed Dockerfile content is written to stdout.
        Exit 0 = produced a fix, Exit 1 = could not fix / API error.

Env vars:
  FUZZ_PROVIDER   openai (default) or anthropic
  FUZZ_API_KEY    API key (falls back to OPENAI_API_KEY)
  FUZZ_MODEL      Model override (optional)
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path

import urllib.request
import urllib.error


# ── LLM API calls ───────────────────────────────────────────────

def call_openai(api_key: str, model: str, system: str, user: str) -> str:
    body = json.dumps({
        "model": model,
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ],
        "temperature": 0.1,
        "max_tokens": 4096,
    }).encode()

    req = urllib.request.Request(
        "https://api.openai.com/v1/chat/completions",
        data=body,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
    )
    with urllib.request.urlopen(req, timeout=90) as resp:
        data = json.loads(resp.read())

    return data["choices"][0]["message"]["content"]


def call_anthropic(api_key: str, model: str, system: str, user: str) -> str:
    body = json.dumps({
        "model": model,
        "max_tokens": 4096,
        "system": system,
        "messages": [{"role": "user", "content": user}],
        "temperature": 0.1,
    }).encode()

    req = urllib.request.Request(
        "https://api.anthropic.com/v1/messages",
        data=body,
        headers={
            "Content-Type": "application/json",
            "x-api-key": api_key,
            "anthropic-version": "2023-06-01",
        },
    )
    with urllib.request.urlopen(req, timeout=90) as resp:
        data = json.loads(resp.read())

    return data["content"][0]["text"]


def call_llm(provider: str, api_key: str, model: str,
             system: str, user: str) -> str:
    if provider == "anthropic":
        return call_anthropic(api_key, model, system, user)
    return call_openai(api_key, model, system, user)


# ── Context gathering ───────────────────────────────────────────

def gather_context(context_dir: str, dockerfile_path: str) -> dict:
    """Gather build context metadata for the LLM."""
    ctx = {
        "dockerfile": Path(dockerfile_path).read_text(),
        "files": [],
        "dependency_files": {},
    }

    context_path = Path(context_dir)

    # List files in build context (limit depth to avoid massive trees)
    all_files = []
    for p in sorted(context_path.rglob("*")):
        if p.is_file():
            rel = str(p.relative_to(context_path))
            # Skip .git, node_modules, vendor, __pycache__
            if any(skip in rel for skip in [".git/", "node_modules/", "vendor/",
                                            "__pycache__/", ".DS_Store"]):
                continue
            all_files.append(rel)
    ctx["files"] = all_files[:200]  # cap at 200 files

    # Read key dependency files
    dep_files = [
        "package.json", "package-lock.json", "yarn.lock",
        "go.mod", "go.sum",
        "requirements.txt", "Pipfile", "pyproject.toml", "setup.py",
        "Gemfile", "Cargo.toml", "pom.xml", "build.gradle",
        "composer.json", "mix.exs",
        ".nvmrc", ".node-version", ".python-version", ".tool-versions",
        "Makefile",
    ]

    for name in dep_files:
        fp = context_path / name
        if fp.exists():
            try:
                content = fp.read_text()
                # Truncate large files (lock files, etc.)
                if len(content) > 2000:
                    content = content[:2000] + "\n... (truncated)"
                ctx["dependency_files"][name] = content
            except Exception:
                pass

    return ctx


# ── Prompts ──────────────────────────────────────────────────────

SYSTEM_PROMPT = """\
You are a Docker build expert. Your job is to fix Dockerfiles so they build
successfully. You will be given:
1. A Dockerfile
2. A listing of files in the build context
3. Key dependency/manifest files from the project

Common issues you should fix:
- Outdated or EOL base images (e.g. node:14 → node:20-alpine, python:3.8 → python:3.12)
- Missing or incorrect COPY sources (files referenced don't exist in the context)
- Wrong working directory or build context assumptions
- Missing build dependencies (e.g. gcc, make, git for native modules)
- Multi-stage builds referencing stages that don't produce the right artifacts
- Platform-specific issues (assume linux/amd64)
- Missing or wrong EXPOSE directives
- Package manager issues (lock file mismatches, wrong package manager)

Rules:
- Output ONLY the fixed Dockerfile content — no explanation, no markdown fences,
  no commentary before or after.
- If the Dockerfile looks correct and should build fine, output it unchanged.
- Preserve the intent and structure of the original as much as possible.
- Keep images small — prefer alpine variants when feasible.
- Do NOT change the application logic, only fix build issues."""

RETRY_SYSTEM_PROMPT = """\
You are a Docker build expert. A Docker build just failed. You will be given:
1. The Dockerfile that failed
2. The build error output
3. Files in the build context
4. Key dependency/manifest files

Fix the Dockerfile so it builds successfully. Common fixes:
- COPY/ADD source files don't exist → fix paths or remove
- Base image doesn't exist or was removed → update to current version
- Build commands fail → add missing dependencies or fix commands
- Multi-stage COPY --from references wrong stage or path

Rules:
- Output ONLY the fixed Dockerfile content — no explanation, no markdown fences.
- Make minimal changes to fix the specific error.
- If you cannot determine a fix, output the original unchanged."""


def build_user_prompt(ctx: dict, build_error: str | None = None) -> str:
    parts = []

    parts.append("## Dockerfile\n```dockerfile\n" + ctx["dockerfile"] + "\n```\n")

    if ctx["files"]:
        parts.append("## Files in build context\n```\n" +
                      "\n".join(ctx["files"]) + "\n```\n")

    if ctx["dependency_files"]:
        parts.append("## Dependency / manifest files\n")
        for name, content in ctx["dependency_files"].items():
            parts.append(f"### {name}\n```\n{content}\n```\n")

    if build_error:
        parts.append("## Build error output\n```\n" + build_error + "\n```\n")
        parts.append("Fix the Dockerfile so this error is resolved.")
    else:
        parts.append("Analyze this Dockerfile and fix any issues that would "
                      "prevent it from building. If it looks correct, return "
                      "it unchanged.")

    return "\n".join(parts)


# ── Response cleaning ────────────────────────────────────────────

def clean_response(text: str) -> str:
    """Strip markdown fences and leading/trailing whitespace."""
    text = text.strip()
    # Remove ```dockerfile ... ``` wrapping
    text = re.sub(r'^```(?:dockerfile|docker|Dockerfile)?\s*\n', '', text)
    text = re.sub(r'\n```\s*$', '', text)
    text = text.strip()
    return text


# ── Main ─────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="LLM-powered Dockerfile fixer")
    parser.add_argument("--dockerfile", required=True, help="Path to Dockerfile")
    parser.add_argument("--context-dir", required=True, help="Docker build context directory")
    parser.add_argument("--build-error", default=None, help="Build error output (for retry mode)")
    args = parser.parse_args()

    # Resolve API config
    provider = os.environ.get("FUZZ_PROVIDER", "openai")
    api_key = os.environ.get("FUZZ_API_KEY") or os.environ.get("OPENAI_API_KEY", "")
    if not api_key:
        print("No API key found (FUZZ_API_KEY / OPENAI_API_KEY)", file=sys.stderr)
        sys.exit(1)

    model = os.environ.get("FUZZ_MODEL", "")
    if not model:
        model = "claude-sonnet-4-20250514" if provider == "anthropic" else "gpt-4o"

    # Gather context
    ctx = gather_context(args.context_dir, args.dockerfile)

    # Build prompt
    if args.build_error:
        system = RETRY_SYSTEM_PROMPT
        user = build_user_prompt(ctx, args.build_error)
        print(f"[fix-dockerfile] retry mode — feeding build error to {provider}/{model}",
              file=sys.stderr)
    else:
        system = SYSTEM_PROMPT
        user = build_user_prompt(ctx)
        print(f"[fix-dockerfile] pre-build analysis via {provider}/{model}",
              file=sys.stderr)

    # Call LLM
    try:
        result = call_llm(provider, api_key, model, system, user)
    except urllib.error.HTTPError as e:
        body = e.read().decode()[:500] if hasattr(e, 'read') else str(e)
        print(f"[fix-dockerfile] API error: HTTP {e.code}: {body}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"[fix-dockerfile] API call failed: {e}", file=sys.stderr)
        sys.exit(1)

    fixed = clean_response(result)

    if not fixed:
        print("[fix-dockerfile] LLM returned empty response", file=sys.stderr)
        sys.exit(1)

    # Sanity check: must contain FROM
    if not re.search(r'^FROM\s', fixed, re.MULTILINE):
        print("[fix-dockerfile] LLM response doesn't look like a Dockerfile "
              "(no FROM instruction)", file=sys.stderr)
        sys.exit(1)

    print(fixed)


if __name__ == "__main__":
    main()
