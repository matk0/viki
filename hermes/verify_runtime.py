import json
import os
import sys


EXPECTED_TOOLS = {
    "search_viki": "viki_read",
    "get_viki_page": "viki_read",
    "get_viki_revision": "viki_read",
    "apply_viki_draft_changeset": "viki_edit",
    "claim_next_scenario": "viki_develop",
    "complete_scenario_development": "viki_develop",
    "block_scenario_development": "viki_develop",
}
EXPECTED_TOOLSETS = {
    "qa": ["memory", "clarify", "viki_read"],
    "edit": ["memory", "clarify", "viki_read", "viki_edit"],
    "developer": ["viki_develop"],
}


def main() -> None:
    if len(sys.argv) != 2 or sys.argv[1] not in EXPECTED_TOOLSETS:
        raise SystemExit("usage: verify_runtime.py qa|edit")

    profile = sys.argv[1]
    expected_toolsets = EXPECTED_TOOLSETS[profile]
    configured = [item for item in os.environ.get("HERMES_TUI_TOOLSETS", "").split(",") if item]
    if configured != expected_toolsets:
        raise RuntimeError(f"unexpected configured toolsets: {configured!r}")
    if os.environ.get("HERMES_TUI_PASS_SESSION_ID") != "0":
        raise RuntimeError("Hermes session identifiers must stay outside model context")

    from tui_gateway import server
    from hermes_cli.profile_distribution import describe_distribution
    from tools.registry import registry
    from toolsets import resolve_multiple_toolsets

    profile_name = f"viki-{profile}"
    distribution = describe_distribution(profile_name)
    if distribution.get("name") != profile_name:
        raise RuntimeError(f"profile is not a native distribution: {distribution!r}")
    if not str(distribution.get("source") or "").endswith(
        f"/.viki-distributions/{profile_name}"
    ):
        raise RuntimeError(f"unexpected distribution source: {distribution.get('source')!r}")
    if "plugins/viki" not in set(distribution.get("distribution_owned") or []):
        raise RuntimeError("Viki plugin is not distribution-owned")

    enabled = server._load_enabled_toolsets()
    if enabled != expected_toolsets:
        raise RuntimeError(f"unexpected effective toolsets: {enabled!r}")
    if "project" in enabled:
        raise RuntimeError("explicit Viki posture unexpectedly enabled project tools")

    projection = server._history_to_messages
    if not getattr(projection, "_viki_receipt_projection_v2", False):
        raise RuntimeError("Viki receipt-only history projection is not installed")
    history = [
        {
            "role": "assistant",
            "content": "",
            "tool_calls": [
                {
                    "id": "viki-contract-call",
                    "function": {
                        "name": "search_viki",
                        "arguments": json.dumps({"query": "never expose this argument"}),
                    },
                }
            ],
        },
        {
            "role": "tool",
            "tool_call_id": "viki-contract-call",
            "content": json.dumps(
                {
                    "documents": [
                        {
                            "revisionId": "revision-contract",
                            "pageId": "page-contract",
                            "pageTitle": "Zmluva",
                            "draft": False,
                            "content": "never expose this body",
                        }
                    ]
                }
            ),
        },
    ]
    projected = projection(history)
    expected_receipt = {
        "role": "tool",
        "name": "search_viki",
        "context": "Hľadanie vo viki",
        "result": {
            "citations": [
                {
                    "revisionId": "revision-contract",
                    "pageId": "page-contract",
                    "pageTitle": "Zmluva",
                    "draft": False,
                }
            ],
            "drafts": [],
        },
    }
    if projected != [expected_receipt]:
        raise RuntimeError(f"unexpected Viki history projection: {projected!r}")
    projected_json = json.dumps(projected, ensure_ascii=False)
    if "never expose" in projected_json:
        raise RuntimeError("Viki history projection exposed raw arguments or prose")

    actual_tools = {
        name: registry.get_toolset_for_tool(name)
        for name in EXPECTED_TOOLS
        if name in registry.get_all_tool_names()
    }
    if actual_tools != EXPECTED_TOOLS:
        raise RuntimeError(f"unexpected Viki registry tools: {actual_tools!r}")

    contract_tool = "claim_next_scenario" if profile == "developer" else "search_viki"
    contract_arguments = {} if profile == "developer" else {"query": "zmluva", "limit": 1}
    contract_result = {"revisionId": "revision-contract", "status": "running"} if profile == "developer" else {"documents": []}
    schema_json = json.dumps(registry.get_schema(contract_tool), sort_keys=True).lower()
    if any(field in schema_json for field in ("sessionid", "profile", "userid", "organizationid")):
        raise RuntimeError("trusted identity leaked into the Viki model schema")

    handler = registry.get_entry("search_viki").handler
    handler_globals = handler.__globals__
    original_urlopen = handler_globals["urlopen"]
    captured = {}

    class ContractResponse:
        def __enter__(self):
            return self

        def __exit__(self, *_):
            return False

        def read(self, _limit):
            return json.dumps({"result": contract_result}).encode()

    def contract_urlopen(request, timeout):
        captured["request"] = request
        captured["timeout"] = timeout
        return ContractResponse()

    class ContractAgent:
        session_id = "viki-contract-stored-session"
        _current_turn_id = "viki-contract-turn"
        _current_api_request_id = "viki-contract-request"
        _memory_manager = None
        valid_tool_names = list(EXPECTED_TOOLS)
        enabled_toolsets = expected_toolsets
        disabled_toolsets = []

    try:
        handler_globals["urlopen"] = contract_urlopen
        from agent.agent_runtime_helpers import invoke_tool

        result = invoke_tool(
            ContractAgent(),
            contract_tool,
            contract_arguments,
            "viki-contract-task",
        )
    finally:
        handler_globals["urlopen"] = original_urlopen

    if json.loads(result) != contract_result:
        raise RuntimeError(f"unexpected pinned Viki dispatch result: {result!r}")
    request = captured.get("request")
    if request is None:
        raise RuntimeError("pinned Hermes dispatch did not reach the Viki handler")
    if request.get_header("X-hermes-session-id") != ContractAgent.session_id:
        raise RuntimeError("pinned Hermes did not inject the durable session ID")
    if request.get_header("X-hermes-profile") != profile:
        raise RuntimeError("Viki handler derived the wrong runtime profile")
    if json.loads(request.data) != contract_arguments:
        raise RuntimeError("trusted runtime identity leaked into model tool arguments")

    exposed = set(resolve_multiple_toolsets(enabled))
    expected_viki = {name for name, toolset in EXPECTED_TOOLS.items() if toolset in enabled}
    exposed_viki = set(EXPECTED_TOOLS) & exposed
    if exposed_viki != expected_viki:
        raise RuntimeError(f"unexpected exposed Viki tools: {sorted(exposed_viki)!r}")
    if profile != "developer" and not {"memory", "clarify"}.issubset(exposed):
        raise RuntimeError("memory or clarify is absent from the effective tool surface")


if __name__ == "__main__":
    main()
