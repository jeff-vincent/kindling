#!/usr/bin/env python3
"""
analyze.py — Parse a kindling-generated workflow YAML and cross-validate
networking between services.

Reports:
  - Port mismatches: env var URL port ≠ target service's declared port
  - Dangling refs:   env var URL references a service not in the workflow
  - Dockerfile port mismatches: EXPOSE port ≠ declared port
  - Missing health check paths

Output: JSON to stdout
  {
    "services_count": N,
    "services": [...],
    "issues": [{"severity": "...", "service": "...", "detail": "..."}, ...]
  }
"""

import json
import os
import re
import sys
from pathlib import Path

import yaml


def parse_workflow(path: str) -> dict:
    with open(path) as f:
        return yaml.safe_load(f)


def extract_services(data: dict) -> list[dict]:
    """Pull out every kindling-deploy step as a service record."""
    services = []
    for job_name, job in (data.get("jobs") or {}).items():
        for step in job.get("steps") or []:
            uses = step.get("uses", "")
            if "kindling-deploy" not in uses:
                continue
            w = step.get("with") or {}
            name_raw = w.get("name", "")
            # Strip ${{ github.actor }}- prefix pattern
            name_clean = re.sub(
                r'\$\{\{\s*github\.actor\s*\}\}-', '', name_raw
            )
            # Parse env list (YAML string of list)
            env_vars = {}
            env_raw = w.get("env", "")
            if env_raw:
                try:
                    env_list = yaml.safe_load(env_raw)
                    if isinstance(env_list, list):
                        for e in env_list:
                            if isinstance(e, dict):
                                env_vars[e.get("name", "")] = e.get("value", "")
                except Exception:
                    pass

            # Parse dependencies
            deps = []
            dep_raw = w.get("dependencies", "")
            if dep_raw:
                try:
                    dep_list = yaml.safe_load(dep_raw)
                    if isinstance(dep_list, list):
                        deps = dep_list
                except Exception:
                    pass

            services.append({
                "name": name_clean,
                "name_raw": name_raw,
                "port": w.get("port", ""),
                "health_check_path": w.get("health-check-path", ""),
                "context": w.get("context", "").replace(
                    "${{ github.workspace }}/", ""
                ).replace("${{ github.workspace }}", "."),
                "image": w.get("image", ""),
                "env": env_vars,
                "dependencies": deps,
                "ingress_host": w.get("ingress-host", ""),
            })

    return services


def extract_builds(data: dict) -> list[dict]:
    """Pull out every kindling-build step."""
    builds = []
    for job_name, job in (data.get("jobs") or {}).items():
        for step in job.get("steps") or []:
            uses = step.get("uses", "")
            if "kindling-build" not in uses:
                continue
            w = step.get("with") or {}
            builds.append({
                "name": w.get("name", ""),
                "context": w.get("context", "").replace(
                    "${{ github.workspace }}/", ""
                ).replace("${{ github.workspace }}", "."),
                "image": w.get("image", ""),
            })
    return builds


def get_dockerfile_expose(clone_dir: str, context: str) -> list[str]:
    """Read EXPOSE directives from a Dockerfile."""
    dockerfile = Path(clone_dir) / context / "Dockerfile"
    if not dockerfile.exists():
        # Try lowercase
        dockerfile = Path(clone_dir) / context / "dockerfile"
    if not dockerfile.exists():
        return []
    ports = []
    try:
        for line in dockerfile.read_text().splitlines():
            if re.match(r'^\s*EXPOSE\s', line, re.IGNORECASE):
                for token in line.split()[1:]:
                    port = re.match(r'(\d+)', token)
                    if port:
                        ports.append(port.group(1))
    except Exception:
        pass
    return ports


def validate_networking(services: list[dict], builds: list[dict],
                        clone_dir: str) -> list[dict]:
    """Cross-validate service networking."""
    issues = []

    # Build a lookup: service_name -> service
    svc_by_name = {}
    for svc in services:
        svc_by_name[svc["name"]] = svc

    # Also build a list of known service name fragments
    svc_names = set(svc_by_name.keys())

    for svc in services:
        # ── Check env var URLs reference real services with correct ports ──
        for env_name, env_value in svc.get("env", {}).items():
            # Match patterns like http://xxx-orders:5000 or redis://xxx-redis:6379
            url_match = re.search(
                r'(?:https?|redis|mongodb|amqp|grpc)://([^:/\s]+):(\d+)',
                env_value
            )
            if not url_match:
                # Also check for host:port without scheme
                url_match = re.search(r'([a-zA-Z][\w.-]*):(\d+)', env_value)
            if not url_match:
                continue

            target_host = url_match.group(1)
            target_port = url_match.group(2)

            # Strip ${{ github.actor }}- prefix
            target_host_clean = re.sub(
                r'\$\{\{\s*github\.actor\s*\}\}-', '', target_host
            )

            # Find which service this references
            target_svc = None
            for sn in svc_names:
                # Match if the target host ends with the service name
                # e.g. "user-orders" matches service "orders"
                # or exact match like "orders-redis" for dependency
                if target_host_clean == sn or target_host_clean.endswith(f"-{sn}"):
                    target_svc = svc_by_name[sn]
                    break

            if target_svc:
                # Check port matches
                declared_port = target_svc.get("port", "")
                if declared_port and target_port != declared_port:
                    issues.append({
                        "severity": "error",
                        "service": svc["name"],
                        "type": "port_mismatch",
                        "detail": (
                            f"Env {env_name} references {target_host_clean}:{target_port} "
                            f"but service '{target_svc['name']}' declares port {declared_port}"
                        ),
                    })
            else:
                # Check if it might be a dependency (redis, postgres, etc.)
                dep_suffixes = ["redis", "postgres", "postgresql", "mongodb",
                                "mongo", "mysql", "rabbitmq", "nats", "kafka"]
                is_dep = any(target_host_clean.endswith(s) for s in dep_suffixes)

                if not is_dep:
                    issues.append({
                        "severity": "warning",
                        "service": svc["name"],
                        "type": "dangling_ref",
                        "detail": (
                            f"Env {env_name} references '{target_host_clean}' "
                            f"which is not a declared service in the workflow"
                        ),
                    })

        # ── Check Dockerfile EXPOSE matches declared port ──────────
        if svc.get("context") and svc.get("port"):
            expose_ports = get_dockerfile_expose(clone_dir, svc["context"])
            if expose_ports and svc["port"] not in expose_ports:
                issues.append({
                    "severity": "warning",
                    "service": svc["name"],
                    "type": "expose_mismatch",
                    "detail": (
                        f"Service declares port {svc['port']} but "
                        f"Dockerfile EXPOSEs {', '.join(expose_ports)}"
                    ),
                })

        # ── Check health check path is set ─────────────────────────
        if not svc.get("health_check_path"):
            issues.append({
                "severity": "info",
                "service": svc["name"],
                "type": "missing_health_check",
                "detail": "No health-check-path specified",
            })

    # ── Check build contexts have Dockerfiles ──────────────────────
    for build in builds:
        ctx = build.get("context", "")
        if not ctx or ctx == ".":
            continue
        dockerfile = Path(clone_dir) / ctx / "Dockerfile"
        if not dockerfile.exists():
            dockerfile_lower = Path(clone_dir) / ctx / "dockerfile"
            if not dockerfile_lower.exists():
                issues.append({
                    "severity": "error",
                    "service": build["name"],
                    "type": "missing_dockerfile",
                    "detail": f"No Dockerfile found at {ctx}/Dockerfile",
                })

    return issues


def main():
    if len(sys.argv) < 3:
        print("Usage: analyze.py <workflow.yml> <clone-dir>", file=sys.stderr)
        sys.exit(1)

    workflow_path = sys.argv[1]
    clone_dir = sys.argv[2]

    data = parse_workflow(workflow_path)
    services = extract_services(data)
    builds = extract_builds(data)
    issues = validate_networking(services, builds, clone_dir)

    result = {
        "services_count": len(services),
        "services": services,
        "builds": [{"name": b["name"], "context": b["context"]} for b in builds],
        "issues": issues,
    }

    json.dump(result, sys.stdout, indent=2)


if __name__ == "__main__":
    main()
