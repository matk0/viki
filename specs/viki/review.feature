@id:VIKI-FEAT-REVIEW @concept:VIKI-CON-SPECIFICATION @concept:VIKI-CON-REVISION @concept:VIKI-CON-AUTHORITY
Feature: Review specification changes
  In a human-governed workspace, people decide which proposed behavior to accept.
  Drafts and objections make that decision explicit instead of silently replacing
  agreed requirements.

  @id:VIKI-SC-REVIEW-OBJECTION
  Scenario: An unresolved objection blocks approval
    Given a draft revision has an unresolved objection
    When a reviewer tries to approve the revision
    Then approval is rejected
    And the revision remains a draft

  @id:VIKI-SC-REVIEW-PARENT
  Scenario: A scenario requires an approved parent feature
    Given a scenario belongs to a feature without an approved revision
    When a reviewer tries to approve the scenario
    Then approval is rejected because the parent feature is not approved

  @id:VIKI-SC-REVIEW-RESOLVED
  Scenario: Resolved objections no longer block approval
    Given a draft revision has only resolved objections
    And any required parent feature is approved
    When a reviewer approves the revision
    Then the revision becomes the page's approved revision
