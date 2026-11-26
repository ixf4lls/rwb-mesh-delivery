#!/usr/bin/env python3
"""A reproducible synthetic order through the public HTTP API (stdlib only)."""
import json
import os
import time
import uuid
import urllib.request

BASE = os.environ.get("MESH_API_URL", "http://127.0.0.1:8081/api/v1").rstrip("/")
TOKEN = os.environ["MESH_TOKEN"]
RUN = None


def call(method, path, body=None):
    headers = {"Authorization": "Bearer " + TOKEN, "Content-Type": "application/json"}
    if RUN:
        headers["X-Demo-Run-Id"] = RUN
    if method != "GET":
        headers["Idempotency-Key"] = uuid.uuid4().hex
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)


def command(path, body):
    op = call("POST", path, body)
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        result = call("GET", "/operations/" + op["id"])
        if result["status"] == "failed":
            raise RuntimeError(result["error"])
        if result["status"] == "succeeded":
            return result
        time.sleep(0.05)
    raise TimeoutError("worker did not finish " + op["id"])


def event(order, kind, shipment=None, segment=None):
    body = {"type": kind, "expectedVersion": order["version"]}
    if shipment:
        body["shipmentId"] = shipment
    if segment:
        body["segmentId"] = segment
    command("/orders/" + order["id"] + "/events", body)
    return call("GET", "/orders/" + order["id"])


if __name__ == "__main__":
    try:
        current = call("GET", "/demo/context")
        version = current["contextVersion"]
    except urllib.error.HTTPError as exc:
        if exc.code != 404:
            raise
        version = 0
    run = call("POST", "/demo/runs", {"datasetId": "backend_demo", "expectedContextVersion": version})
    RUN = run["id"]
    quote = call("POST", "/quotes", {"pickupPointId": "13", "items": [{"productId": "101", "quantity": 1}]})
    option = next(o for o in quote["options"] if o["policy"] == "mesh_only" and o["feasible"])
    created = command("/orders", {"quoteId": quote["id"], "optionId": option["id"]})
    order = call("GET", "/orders/" + created["resourceId"])
    shipment = order["shipments"][0]["id"]
    order = event(order, "packed", shipment)
    command("/demo/clock/advances", {"to": "2026-01-05T12:00:00Z"})
    order = event(order, "handed_over", shipment, order["shipments"][0]["nextSegmentId"])
    command("/demo/clock/advances", {"to": "2026-01-05T15:00:00Z"})
    order = event(order, "arrived", shipment, order["shipments"][0]["activeSegmentId"])
    order = event(order, "issued")
    batch = call("GET", "/batches/41")
    entries = call("GET", "/ledger-entries?from=2026-01-05T00:00:00Z&to=2026-01-06T00:00:00Z&orderId=" + order["id"])
    assert order["status"] == "issued"
    assert batch["availableQuantity"] == 4 and batch["packedQuantity"] == 0
    assert entries["items"], "missing settlement"
    print(json.dumps({"runId": RUN, "orderId": order["id"], "status": order["status"], "remainingQuantity": batch["availableQuantity"], "ledgerEntryCount": len(entries["items"])}, ensure_ascii=False, indent=2))
