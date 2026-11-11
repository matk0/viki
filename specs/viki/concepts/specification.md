<!-- business-id: VIKI-CON-SPECIFICATION -->
# Specification

A specification explains a system's intended behavior in a form that people can
read and discuss. Viki organizes it into three kinds of page:

- A **concept** defines shared vocabulary, such as a revision or an objection.
- A **feature** explains a capability and its purpose.
- A **scenario** belongs to a feature and gives a concrete example of behavior
  using Given, When, and Then steps.

References connect related pages. A scenario is more useful when its terms mean
the same thing as the concepts it references. Viki stores structured steps and
references alongside each page's prose.

Viki supports two sources of specification authority: a human-governed workspace
and a Git-managed workspace. They share a reader but have different ways to
change the catalogue. A workspace does not mix independently edited wiki pages
with an imported repository baseline.
