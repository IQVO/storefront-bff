#!/usr/bin/env python3
"""Assert this chart's Service selectors isolate each component.

Why this exists (fleet-wide): `app.kubernetes.io/name` + `instance` are identical
on every pod a release creates. A Service that selects on those two alone selects
ALL of them, which turns into a wrong answer the day a second component is added.
Every Deployment here carries `app.kubernetes.io/component` in selector.matchLabels
and its pod labels, and every Service selects on it, so each Service selects
EXACTLY ONE Deployment. This test fails if that ever stops being true.

storefront-bff has exactly one component: the api (the storefront edge
/api/storefront, a stateless backend-for-frontend composing six upstreams over
REST). There is no mcp and no frontend: the chart renders nothing for binaries
that do not exist. The checks run with every optional template (HPA, Ingress,
HTTPRoute) enabled together.

Run: python3 charts/storefront-bff/tests/test_service_selectors.py
Needs: helm, PyYAML.
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

CHART_DIR = Path(__file__).resolve().parents[1]
RELEASE = "storefront-bff"

# Dummy upstreams, never dialled: the chart refuses to render without all six
# (cmd/api exits at boot naming every missing one).
UPSTREAMS = {
    "productDiscoveryUrl": "http://product-discovery.example.invalid",
    "deliveryPromiseUrl": "http://delivery-promise.example.invalid",
    "shoppingCartUrl": "http://shopping-cart.example.invalid",
    "checkoutUrl": "http://checkout.example.invalid",
    "customerOrderingUrl": "http://customer-ordering.example.invalid",
    "pricingUrl": "http://pricing.example.invalid",
}


def runtime(skip: str | None = None) -> list[str]:
    """The mandatory values, minus the one named by skip (an upstream key)."""
    args: list[str] = []
    for key, value in UPSTREAMS.items():
        if skip != key:
            args += ["--set", f"config.{key}={value}"]
    return args


BASE = runtime()
ENABLE_EVERYTHING = BASE + [
    "--set", "autoscaling.api.enabled=true",
    "--set", "gatewayApi.enabled=true",
    "--set", "ingress.enabled=true",
    "--set", "config.cookieSecure=true",
    "--set", "config.budgetQueryMs=3000",
    "--set", "config.budgetCommandMs=6000",
    "--set", "config.budgetConfirmMs=20000",
    "--set", "config.shutdownDrainDelay=5s",
]

# Every variable cmd/api reads (internal/config/config.go), with a dedicated
# chart value. No DATABASE_URL, no KAFKA_BROKERS: state rides in the shopper's
# cookie and the upstreams are plain REST.
API_ENV = (
    "PORT", "COOKIE_SECURE",
    "PRODUCT_DISCOVERY_URL", "DELIVERY_PROMISE_URL", "SHOPPING_CART_URL",
    "CHECKOUT_URL", "CUSTOMER_ORDERING_URL", "PRICING_URL",
    "BUDGET_QUERY_MS", "BUDGET_COMMAND_MS", "BUDGET_CONFIRM_MS",
)


def helm_template(extra_args: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["helm", "template", RELEASE, str(CHART_DIR), *extra_args],
        capture_output=True, text=True,
    )


def render(extra_args: list[str]) -> list[dict]:
    result = helm_template(extra_args)
    if result.returncode != 0:
        raise SystemExit(f"FAIL: helm template {' '.join(extra_args)}:\n{result.stderr}")
    try:
        import yaml  # type: ignore
    except ModuleNotFoundError:  # pragma: no cover - environment guard
        print("SKIP: PyYAML not available; cannot assert selectors", file=sys.stderr)
        raise SystemExit(0)
    return [d for d in yaml.safe_load_all(result.stdout) if d]


def selector_of(doc: dict) -> dict:
    return doc.get("spec", {}).get("selector") or {}


def pod_labels_of(doc: dict) -> dict:
    return doc.get("spec", {}).get("template", {}).get("metadata", {}).get("labels") or {}


def matches(selector: dict, labels: dict) -> bool:
    return bool(selector) and all(labels.get(k) == v for k, v in selector.items())


def env_of(dep: dict) -> list[dict]:
    return dep["spec"]["template"]["spec"]["containers"][0].get("env", [])


def env_names(dep: dict) -> list[str]:
    return [e["name"] for e in env_of(dep)]


def env_value(dep: dict, name: str) -> str | None:
    return next((e.get("value") for e in env_of(dep) if e["name"] == name), None)


def check_components(docs: list[dict], failures: list[str]) -> None:
    services = {d["metadata"]["name"]: d for d in docs if d.get("kind") == "Service"}
    deployments = {d["metadata"]["name"]: d for d in docs if d.get("kind") == "Deployment"}

    if set(deployments) != {RELEASE}:
        failures.append(f"the chart has exactly one Deployment (the api), rendered: {sorted(deployments)}")
    if set(services) != {RELEASE}:
        failures.append(f"the chart has exactly one Service (the api), rendered: {sorted(services)}")
    if RELEASE not in services:
        failures.append("the api Service was not rendered")
    elif selector_of(services[RELEASE]).get("app.kubernetes.io/component") != "api":
        failures.append("the api Service selector must pin component=api")

    # Every Deployment's own selector must pin a component too, and be
    # satisfied by its pod labels.
    for name, dep in deployments.items():
        match_labels = dep["spec"]["selector"].get("matchLabels") or {}
        if "app.kubernetes.io/component" not in match_labels:
            failures.append(f"Deployment {name} selector.matchLabels lacks app.kubernetes.io/component")
        if not matches(match_labels, pod_labels_of(dep)):
            failures.append(f"Deployment {name} pod labels do not satisfy its own selector")

    # The real invariant: each Service selects exactly one Deployment.
    for svc_name, svc in services.items():
        sel = selector_of(svc)
        hit = [d for d, dep in deployments.items() if matches(sel, pod_labels_of(dep))]
        if len(hit) != 1:
            failures.append(f"Service {svc_name} selects {len(hit)} Deployments {sorted(hit)}; expected exactly 1")

    # Ingress and HTTPRoute route to the api Service only; the HPA owns replicas.
    for d in docs:
        if d.get("kind") == "Ingress":
            backends = {p["backend"]["service"]["name"] for r in d["spec"]["rules"] for p in r["http"]["paths"]}
            if backends != {RELEASE}:
                failures.append(f"the Ingress must route to the api Service only, got {sorted(backends)}")
        if d.get("kind") == "HTTPRoute":
            backends = {b["name"] for r in d["spec"]["rules"] for b in r["backendRefs"]}
            if backends != {RELEASE}:
                failures.append(f"the HTTPRoute must route to the api Service only, got {sorted(backends)}")
    hpa = next((d for d in docs if d.get("kind") == "HorizontalPodAutoscaler"), None)
    if hpa is None or hpa["spec"]["scaleTargetRef"]["name"] != RELEASE:
        failures.append("the api HPA was not rendered (or does not scale the api Deployment)")
    elif "replicas" in deployments.get(RELEASE, {}).get("spec", {}):
        failures.append("the api Deployment must omit replicas when its HPA owns them")

    api = deployments.get(RELEASE)
    if api is None:
        failures.append("the api Deployment was not rendered")
        return
    api_env = env_names(api)
    for want in API_ENV:
        if want not in api_env:
            failures.append(f"the api Deployment does not render {want} from its dedicated value")
    duplicates = sorted({n for n in api_env if api_env.count(n) > 1})
    if duplicates:
        failures.append(f"the api Deployment renders env vars twice: {duplicates}")
    for name, want in (("COOKIE_SECURE", "true"), ("BUDGET_QUERY_MS", "3000"),
                       ("BUDGET_COMMAND_MS", "6000"), ("BUDGET_CONFIRM_MS", "20000")):
        if env_value(api, name) != want:
            failures.append(f"the api Deployment renders {name}={env_value(api, name)!r}, want {want!r}")


def check_refusals(failures: list[str]) -> None:
    cases: list[tuple[str, list[str], str]] = [
        ("a *_MODE through extraEnv",
         BASE + ["--set", "extraEnv[0].name=CACHE_MODE", "--set", "extraEnv[0].value=permissive"], "extraEnv sets CACHE_MODE"),
        ("a chart-owned variable through extraEnv",
         BASE + ["--set", "extraEnv[0].name=PORT", "--set", "extraEnv[0].value=9090"], "extraEnv sets PORT"),
        ("a chart-owned upstream through extraEnv",
         BASE + ["--set", "extraEnv[0].name=CHECKOUT_URL", "--set", "extraEnv[0].value=x"], "extraEnv sets CHECKOUT_URL"),
        ("a grace period under the shutdown bound", BASE + ["--set", "terminationGracePeriodSeconds=19"], "terminationGracePeriodSeconds"),
        ("a grace period equal to the shutdown bound", BASE + ["--set", "terminationGracePeriodSeconds=20"], "must exceed 20"),
        ("a port on another port than the container's", BASE + ["--set", "config.port=9090"], "config.port"),
        ("a drain delay the chart cannot size", BASE + ["--set", "config.shutdownDrainDelay=soon"], "config.shutdownDrainDelay"),
        ("a non-positive budget", BASE + ["--set", "config.budgetQueryMs=0"], "config.budgetQueryMs"),
    ]
    for key in UPSTREAMS:
        cases.append((f"without config.{key}", runtime(key), f"config.{key}"))
    for label, args, needle in cases:
        result = helm_template(args)
        if result.returncode == 0 or needle not in result.stderr:
            failures.append(f"chart rendered (or failed for another reason) {label}")


def main() -> int:
    failures: list[str] = []

    check_components(render(ENABLE_EVERYTHING), failures)

    # Default values must not deploy an HPA or a route, nor any optional env.
    defaults = render(BASE)
    stray = [d["kind"] for d in defaults if d.get("kind") in {"HorizontalPodAutoscaler", "Ingress", "HTTPRoute", "Secret"}]
    if stray:
        failures.append(f"optional templates rendered with default values: {stray}")
    dep = next(d for d in defaults if d.get("kind") == "Deployment")
    names = env_names(dep)
    off = {"BUDGET_QUERY_MS", "BUDGET_COMMAND_MS", "BUDGET_CONFIRM_MS"}
    if off & set(names):
        failures.append(f"these must be off with default values: {sorted(off & set(names))}")
    for want in ("PORT", "COOKIE_SECURE", *[e for e in API_ENV if e.endswith("_URL")]):
        if want not in names:
            failures.append(f"{want} is mandatory and must always be rendered")
    # The chart defaults COOKIE_SECURE to false (kind serves plain HTTP); the
    # binary defaults to true.
    if env_value(dep, "COOKIE_SECURE") != "false":
        failures.append(f"COOKIE_SECURE must default to false in the chart, got {env_value(dep, 'COOKIE_SECURE')!r}")

    check_refusals(failures)

    if failures:
        for f in failures:
            print(f"FAIL: {f}")
        return 1

    print("PASS: the Service selects exactly one Deployment (the api); the HPA, Ingress and HTTPRoute are off by default "
          "and route to the api only; every cmd/api env var has a dedicated value; the chart refuses to render without "
          "any of the six upstream base URLs, with a *_MODE or a chart-owned variable in extraEnv, with a grace period "
          "that does not exceed the shutdown bound, with a port off the container port, or with a non-positive budget")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
