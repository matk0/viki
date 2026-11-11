@id:VIKI-FEAT-ASSISTANT @concept:VIKI-CON-ASSISTANT @concept:VIKI-CON-REVISION @concept:VIKI-CON-AUTHORITY
Feature: Assist readers and authors within the workspace's authority
  Q&A can read specification evidence. Edit can prepare drafts only where people
  govern the specification. Neither profile can approve a change.

  @id:VIKI-SC-ASSISTANT-QA-BOUNDARY
  Scenario: The Q&A profile cannot invoke editing tools
    Answering a question does not grant authority to change the specification.
    The server enforces the profile's tool boundary.
    Given a Hermes turn is authenticated for the Q&A profile
    When it attempts to invoke a Viki editing tool
    Then the tool request is rejected
    And no draft is created

  @id:VIKI-SC-ASSISTANT-GIT-EDIT
  Scenario: Git-managed workspaces allow Q&A but reject Edit
    Readers can ask questions about repository content. Proposed changes still
    go through that repository rather than a separate assistant editing path.
    Given a workspace is managed by a Git repository
    And its Q&A gateway is configured and connected
    When the assistant's available modes are requested
    Then Q&A is available
    And Edit is unavailable
