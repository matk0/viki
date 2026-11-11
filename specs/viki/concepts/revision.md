<!-- business-id: VIKI-CON-REVISION -->
<!-- relates-to: VIKI-CON-SPECIFICATION, VIKI-CON-AUTHORITY -->
# Revision

A revision is a version of one specification page's content: its title, prose,
scenario steps, and references. Page identity and revision identity are separate,
so a reference can identify a page or a particular version of its meaning.

In a human-governed workspace, edits create drafts. A person reviews a draft and
approves it when its blockers are resolved. Further edits do not silently replace
the agreed content; they create another draft.

In a Git-managed workspace, an imported business revision is a repository
baseline, not a human approval. Stable source IDs preserve page identity when
files move or titles change. Re-publishing unchanged specification content reuses
the business revision, even if the CI receipt or source location changes.

Slovak translations are derived versions associated with a particular canonical
English revision. Correcting a translation does not create a new English
business revision.
