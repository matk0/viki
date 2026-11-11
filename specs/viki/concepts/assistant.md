<!-- business-id: VIKI-CON-ASSISTANT -->
<!-- relates-to: VIKI-CON-SPECIFICATION, VIKI-CON-REVISION, VIKI-CON-AUTHORITY -->
# Assistant

The optional assistant helps readers understand a specification and helps
authors prepare changes. Separate Hermes profiles constrain its capabilities.

**Q&A** retrieves workspace content and answers questions with citations to
specific revisions. Its Viki tools are read-only. Citations let a reader inspect
the evidence behind an answer; they do not guarantee that the answer's reasoning
is correct.

**Edit** can prepare linked draft revisions in a human-governed workspace.
It cannot approve those drafts. A Git-managed workspace disables Edit because
the repository remains the source of changes.

The core wiki is usable without an assistant provider. Q&A and editing require
configured credentials and a running Hermes gateway. Model usage and translation
usage are separate from the cost of hosting the wiki.

The experimental developer worker is a separate, opt-in capability. It is
disabled by default, unavailable for Git-managed workspaces, and the included
development target only returns a receipt. It is not a production delivery path.
