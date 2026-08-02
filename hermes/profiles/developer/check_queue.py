import json
import os
from urllib.request import Request, urlopen


base_url = os.environ.get("VIKI_INTERNAL_URL", "http://viki:8090").rstrip("/")
token = os.environ["VIKI_HERMES_TOOL_TOKEN"]
request = Request(
    f"{base_url}/internal/v1/development/pending",
    headers={"Authorization": f"Bearer {token}"},
)
with urlopen(request, timeout=10) as response:
    queued = bool(json.load(response).get("wakeAgent"))
print(json.dumps({"wakeAgent": queued}))
