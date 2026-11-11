<!-- business-id: VIKI-CON-AUTHORITY -->
<!-- relates-to: VIKI-CON-SPECIFICATION -->
# Authority and evidence

Authority answers where an accepted change must come from. Evidence answers
what was checked. Viki keeps these meanings separate.

In a **human-governed workspace**, reviewers approve draft revisions. Comments
support discussion, while unresolved objections explicitly block approval.
An approval records a review decision; it does not claim that executable tests
passed.

In a **Git-managed workspace**, the configured application repository owns the
specification. Its successful main-branch CI publishes exact sources and actual
Cucumber results. Viki validates the source hashes, repository and run identity,
supported Gherkin, and complete passing execution before importing a snapshot.
Local exports and pull-request artifacts cannot replace the live baseline.

That evidence does not mean a person approved the specification or that the
application was deployed. Viki does not execute the imported code. The application
owns its executable step definitions, fixtures, and business assertions.

An import failure leaves the last committed snapshot readable. Freshness and
polling errors are available through the authenticated workspace-sync API.
