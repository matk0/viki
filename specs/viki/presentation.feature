@id:VIKI-FEAT-PRESENTATION @concept:VIKI-CON-SPECIFICATION @concept:VIKI-CON-REVISION
Feature: Present canonical English specifications in Slovak
  Translations help readers understand the specification while English remains
  the canonical source. Translation availability does not control repository imports.

  @id:VIKI-SC-PRESENTATION-CANONICAL
  Scenario: A translation leaves the canonical revision unchanged
    Selecting a presentation language changes what the reader sees, while
    citations retain the identity of the canonical English revision.
    Given an imported English revision has a completed Slovak translation
    When a reader selects Slovak
    Then the reader sees the translation associated with that revision
    And the canonical English content and revision identity are unchanged

  @id:VIKI-SC-PRESENTATION-FALLBACK
  Scenario: A missing translation falls back to the current English revision
    Readers see the latest canonical content rather than a translation that
    belongs to a different business revision.
    Given the current imported revision has no completed Slovak translation
    When a reader selects Slovak
    Then the current English content is displayed with a translation notice
    And a translation of an older revision is not substituted
