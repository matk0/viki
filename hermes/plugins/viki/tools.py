import json
import os
from pathlib import Path
from threading import Lock
from urllib.error import HTTPError, URLError
from urllib.parse import urlsplit
from urllib.request import Request, urlopen


_PROFILE_NAMES = {"viki-qa": "qa", "viki-edit": "edit", "viki-developer": "developer"}
_ENDPOINTS = {
    "search": "search_viki",
    "get_page": "get_viki_page",
    "get_revision": "get_viki_revision",
    "apply_draft_changeset": "apply_viki_draft_changeset",
    "claim_scenario": "claim_next_scenario",
    "complete_development": "complete_scenario_development",
    "block_development": "block_scenario_development",
}
_MAX_RESPONSE_BYTES = 2 * 1024 * 1024
_development_leases: dict[tuple[str, str], str] = {}
_development_leases_lock = Lock()


def _error(code: str, message: str) -> str:
    return json.dumps(
        {"error": {"code": code, "message": message}},
        ensure_ascii=False,
        separators=(",", ":"),
    )


def _runtime_profile() -> str:
    from hermes_constants import get_hermes_home

    home = Path(get_hermes_home())
    profile = _PROFILE_NAMES.get(home.name)
    if profile is None:
        raise RuntimeError("Hermes turn is not scoped to a managed viki profile")
    return profile


def _internal_base_url() -> str:
    raw = os.environ.get("VIKI_INTERNAL_URL", "http://viki:8090").rstrip("/")
    parsed = urlsplit(raw)
    allowed_hosts = {"viki", "127.0.0.1", "localhost", "::1"}
    if parsed.scheme != "http" or parsed.hostname not in allowed_hosts:
        raise RuntimeError("VIKI_INTERNAL_URL must target the managed viki service")
    if parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise RuntimeError("VIKI_INTERNAL_URL contains unsupported URL components")
    return raw


def _decode_response(payload: bytes) -> str:
    try:
        decoded = json.loads(payload)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return _error("invalid_upstream_response", "viki vrátilo neplatnú odpoveď")

    if isinstance(decoded, dict) and "result" in decoded:
        return json.dumps(decoded["result"], ensure_ascii=False, separators=(",", ":"))
    if isinstance(decoded, dict) and "error" in decoded:
        return json.dumps(decoded, ensure_ascii=False, separators=(",", ":"))
    return _error("invalid_upstream_response", "viki vrátilo neočakávanú odpoveď")


def _has_result(payload: bytes) -> bool:
    try:
        decoded = json.loads(payload)
    except (UnicodeDecodeError, json.JSONDecodeError):
        return False
    return isinstance(decoded, dict) and "result" in decoded


def _tool_token(profile: str) -> str:
    variable = "VIKI_DEVELOPER_TOOL_TOKEN" if profile == "developer" else "VIKI_HERMES_TOOL_TOKEN"
    return os.environ.get(variable, "").strip()


def _development_lease(session_id: str, task_id: str) -> str:
    with _development_leases_lock:
        return _development_leases.get((session_id, task_id), "")


def _store_development_lease(session_id: str, task_id: str, lease: str) -> None:
    with _development_leases_lock:
        _development_leases[(session_id, task_id)] = lease


def _clear_development_lease(session_id: str, task_id: str) -> None:
    with _development_leases_lock:
        _development_leases.pop((session_id, task_id), None)


def _call(endpoint: str, args: dict, kwargs: dict, *, profiles: set[str] | None = None) -> str:
    session_id = str(kwargs.get("session_id") or "").strip()
    if not session_id:
        return _error(
            "missing_runtime_context",
            "Hermes neposkytol dôveryhodný identifikátor relácie; volanie bolo zamietnuté.",
        )

    try:
        profile = _runtime_profile()
    except Exception:
        return _error(
            "missing_runtime_context",
            "Hermes relácia nie je priradená k spravovanému profilu viki.",
        )
    if profiles is not None and profile not in profiles:
        return _error("profile_forbidden", "Tento nástroj nie je povolený pre aktuálny profil.")

    task_id = str(kwargs.get("task_id") or "").strip()
    if profile == "developer" and not task_id:
        return _error(
            "missing_runtime_context",
            "Hermes neposkytol dôveryhodný identifikátor úlohy; volanie bolo zamietnuté.",
        )

    token = _tool_token(profile)
    if not token:
        return _error("configuration_error", "Chýba interné overenie nástrojov viki.")

    lease = ""
    if profile == "developer" and endpoint in {_ENDPOINTS["complete_development"], _ENDPOINTS["block_development"]}:
        lease = _development_lease(session_id, task_id)
        if not lease:
            return _error("missing_development_claim", "Vývojový scenár nebol bezpečne vyzdvihnutý.")

    try:
        base_url = _internal_base_url()
    except RuntimeError:
        return _error("configuration_error", "Interná adresa viki nie je bezpečne nastavená.")

    request = Request(
        f"{base_url}/internal/v1/hermes/tools/{endpoint}",
        data=json.dumps(args, ensure_ascii=False, separators=(",", ":")).encode(),
        method="POST",
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
            "X-Hermes-Session-ID": session_id,
            "X-Hermes-Profile": profile,
        },
    )
    if task_id:
        request.add_header("X-Hermes-Task-ID", task_id)
    if lease:
        request.add_header("X-Viki-Development-Lease", lease)

    try:
        with urlopen(request, timeout=30) as response:
            payload = response.read(_MAX_RESPONSE_BYTES + 1)
            response_lease = response.headers.get("X-Viki-Development-Lease", "").strip()
    except HTTPError as exc:
        payload = exc.read(_MAX_RESPONSE_BYTES + 1)
        if len(payload) <= _MAX_RESPONSE_BYTES:
            return _decode_response(payload)
        return _error("upstream_response_too_large", "Chybová odpoveď viki je príliš veľká.")
    except (TimeoutError, URLError, OSError):
        return _error("upstream_unavailable", "viki momentálne nie je dostupné.")

    if len(payload) > _MAX_RESPONSE_BYTES:
        return _error("upstream_response_too_large", "Odpoveď viki je príliš veľká.")
    if profile == "developer" and endpoint == _ENDPOINTS["claim_scenario"]:
        if not response_lease:
            return _error("invalid_upstream_response", "viki nevrátilo bezpečný vývojový nárok.")
        _store_development_lease(session_id, task_id, response_lease)
    if profile == "developer" and endpoint in {_ENDPOINTS["complete_development"], _ENDPOINTS["block_development"]} and _has_result(payload):
        _clear_development_lease(session_id, task_id)
    return _decode_response(payload)


def search_viki(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["search"], args, kwargs)


def get_viki_page(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["get_page"], args, kwargs)


def get_viki_revision(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["get_revision"], args, kwargs)


def apply_viki_draft_changeset(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["apply_draft_changeset"], args, kwargs, profiles={"edit"})


def claim_next_scenario(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["claim_scenario"], args, kwargs, profiles={"developer"})


def complete_scenario_development(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["complete_development"], args, kwargs, profiles={"developer"})


def block_scenario_development(args: dict, **kwargs) -> str:
    return _call(_ENDPOINTS["block_development"], args, kwargs, profiles={"developer"})
