import json
import os
import sys
import tempfile
from pathlib import Path
from urllib.parse import urlsplit


def validated_base_url() -> str:
    raw = os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
    parsed = urlsplit(raw)
    loopback = parsed.hostname in {"127.0.0.1", "localhost", "::1"}
    if parsed.scheme != "https" and not (parsed.scheme == "http" and loopback):
        raise ValueError("OPENAI_BASE_URL must use HTTPS or HTTP loopback")
    if not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("OPENAI_BASE_URL contains unsupported URL components")
    return raw


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit("usage: render_config.py TEMPLATE TARGET")

    template_path = Path(sys.argv[1])
    target_path = Path(sys.argv[2])
    model = os.environ.get("HERMES_MODEL", "gpt-5.6-terra").strip()
    if not model or any(character in model for character in "\r\n"):
        raise ValueError("HERMES_MODEL must be a non-empty single-line value")

    rendered = template_path.read_text(encoding="utf-8")
    rendered = rendered.replace("@@HERMES_MODEL@@", json.dumps(model, ensure_ascii=False))
    rendered = rendered.replace(
        "@@OPENAI_BASE_URL@@",
        json.dumps(validated_base_url(), ensure_ascii=False),
    )
    if "@@" in rendered:
        raise ValueError("unresolved managed config placeholder")

    target_path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        prefix=f".{target_path.name}.",
        dir=target_path.parent,
        text=True,
    )
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as output:
            output.write(rendered)
            output.flush()
            os.fsync(output.fileno())
        os.chmod(temporary_name, 0o640)
        os.replace(temporary_name, target_path)
    except Exception:
        try:
            os.unlink(temporary_name)
        except FileNotFoundError:
            pass
        raise


if __name__ == "__main__":
    main()
