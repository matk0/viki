import importlib
import json
from functools import wraps


VIKI_TOOL_NAMES = {
    "search_viki",
    "get_viki_page",
    "get_viki_revision",
    "propose_viki_changeset",
}
HISTORY_PROJECTION_MARKER = "_viki_receipt_projection_v1"
_SAFE_CONTEXT = {
    "search_viki": "Hľadanie vo viki",
    "get_viki_page": "Čítanie podkladov vo viki",
    "get_viki_revision": "Čítanie podkladov vo viki",
    "propose_viki_changeset": "Návrh zmien vo viki",
}


def _decode_content(content):
    if isinstance(content, (dict, list)):
        return content
    if not isinstance(content, (str, bytes, bytearray)):
        return None
    try:
        decoded = json.loads(content)
        if isinstance(decoded, str):
            decoded = json.loads(decoded)
        return decoded
    except (json.JSONDecodeError, TypeError, UnicodeDecodeError, RecursionError):
        return None


def _tool_candidates(name, payload):
    if not isinstance(payload, dict):
        return []
    if isinstance(payload.get("result"), dict):
        payload = payload["result"]

    if name == "search_viki":
        candidates = payload.get("documents")
    elif name == "get_viki_page":
        candidates = [payload.get("acceptedRevision"), payload.get("draftRevision")]
    elif name == "get_viki_revision":
        candidates = [payload]
    else:
        return []
    return candidates if isinstance(candidates, list) else []


def _receipt(candidate):
    if not isinstance(candidate, dict):
        return None
    revision_id = candidate.get("revisionId") or candidate.get("id")
    page_id = candidate.get("pageId")
    page_title = candidate.get("pageTitle") or candidate.get("title")
    if not all(isinstance(value, str) and value.strip() for value in (revision_id, page_id, page_title)):
        return None
    draft = candidate.get("draft")
    if not isinstance(draft, bool):
        draft = candidate.get("status") == "draft"
    return {
        "revisionId": revision_id,
        "pageId": page_id,
        "pageTitle": page_title,
        "draft": draft,
    }


def _safe_result(name, content):
    payload = _decode_content(content)
    if name == "propose_viki_changeset":
        proposal = payload.get("proposal") if isinstance(payload, dict) else None
        if not isinstance(proposal, dict):
            return {"citations": [], "drafts": [], "proposal": None}
        return {
            "citations": [],
            "drafts": [],
            "proposal": {
                "id": proposal.get("id"),
                "turnId": proposal.get("turnId"),
                "summary": proposal.get("summary"),
                "status": proposal.get("status"),
            },
        }
    receipts = []
    seen = set()
    for candidate in _tool_candidates(name, payload):
        receipt = _receipt(candidate)
        if receipt is None or receipt["revisionId"] in seen:
            continue
        seen.add(receipt["revisionId"])
        receipts.append(receipt)
    return {"citations": receipts, "drafts": []}


def install(server_module=None):
    if server_module is None:
        try:
            server_module = importlib.import_module("tui_gateway.server")
        except ModuleNotFoundError as exc:
            if exc.name in {"tui_gateway", "tui_gateway.server"}:
                return False
            raise

    original = getattr(server_module, "_history_to_messages", None)
    if not callable(original):
        raise RuntimeError("Pinned Hermes history projection is unavailable")
    if getattr(original, HISTORY_PROJECTION_MARKER, False):
        return True

    @wraps(original)
    def project(history):
        messages = original(history)
        raw_tools = [
            message
            for message in history
            if isinstance(message, dict) and message.get("role") == "tool"
        ]
        raw_index = 0
        for message in messages:
            if not isinstance(message, dict) or message.get("role") != "tool":
                continue
            raw = raw_tools[raw_index] if raw_index < len(raw_tools) else {}
            raw_index += 1
            name = message.get("name")
            if name not in VIKI_TOOL_NAMES:
                continue
            message["context"] = _SAFE_CONTEXT[name]
            message["result"] = _safe_result(name, raw.get("content"))
        return messages

    setattr(project, HISTORY_PROJECTION_MARKER, True)
    setattr(project, "_viki_original", original)
    server_module._history_to_messages = project
    return True
