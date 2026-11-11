<!-- business-id: VIKI-CON-ARCHITECTURE -->
<!-- relates-to: VIKI-CON-SPECIFICATION, VIKI-CON-AUTHORITY, VIKI-CON-ASSISTANT -->
# Architecture and the public demo

The full Viki application has a React/Vite reader, a Go HTTP API, and PostgreSQL
persistence with pgvector and text search. Separate Hermes profiles provide Q&A,
editing, and an experimental developer worker. Docker Compose runs those services
for local development; the existing private deployment runs on Umbrel.
Viki does not automatically deploy applications. The bundled experimental
development target returns a receipt rather than delivering production code.

This public demo reuses the React reader with a smaller, separate Cloudflare
Workers API. Its catalogue is built from Viki's own checked-in Markdown and
Gherkin. These scenarios explain behavior but have no executable Cucumber
bindings. The demo does not claim they passed acceptance tests or use the full
application's verified-artifact importer.

Visitors can browse concepts, features, scenarios, and source revisions, search
the catalogue, and ask questions grounded in selected source excerpts. Writing,
human review, assistant editing, and the developer worker are unavailable.

Public Q&A uses OpenAI gpt-oss-20b through Cloudflare Workers AI. The same model
is listed in NVIDIA's catalogue, but this demo does not use NVIDIA's hosted trial
API. Each question makes at most two bounded model calls, with one retry for
known temporary inference or structured-output failures. Each call has a
15-second timeout. Answers cite supplied source
revisions; citations are checked against the supplied sources before display.
This does not prove that every answer's interpretation is correct.

Chat history stays in the individual browser tab and clears on reload. The server
does not store conversations. Recent messages are sent to Cloudflare AI for
follow-up questions. Cloudflare D1 stores daily usage counters, with hashed
visitor and IP buckets. The limits are 20 model attempts globally per UTC day,
including retries, five questions per visitor, and ten per IP. Failed attempts
also count. Content-free diagnostics record failure categories and a request
reference; application logs contain no conversation text. When a limit or provider allowance is reached,
Q&A pauses while the reader and search remain available.
