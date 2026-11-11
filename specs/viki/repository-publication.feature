@id:VIKI-FEAT-REPOSITORY @concept:VIKI-CON-AUTHORITY @concept:VIKI-CON-REVISION
Feature: Publish a read-only specification from Git
  The application repository owns the canonical specification and its executable
  checks. Viki validates complete publications and presents the resulting catalogue.

  @id:VIKI-SC-REPOSITORY-READ-ONLY
  Scenario: Imported specifications cannot be edited independently
    Given a workspace is managed by a configured Git repository
    When an authenticated reader tries to create a wiki draft
    Then the request is rejected as read-only
    And the imported catalogue is unchanged

  @id:VIKI-SC-REPOSITORY-LAST-GOOD
  Scenario: A rejected publication preserves the last valid snapshot
    Given a workspace has a successfully imported specification snapshot
    When the importer receives a publication that fails validation
    Then the publication is rejected
    And the previous snapshot remains the current catalogue

  @id:VIKI-SC-REPOSITORY-SAME-CONTENT
  Scenario: A new CI receipt reuses unchanged specification revisions
    Given the current snapshot contains an imported scenario revision
    When an eligible publication repeats the same scenario content with a new CI receipt
    Then the scenario keeps its existing business revision
    And the snapshot records the new publication metadata
