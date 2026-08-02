CREATE EXTENSION IF NOT EXISTS unaccent;

CREATE TABLE step_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    expression TEXT NOT NULL CHECK (btrim(expression) <> ''),
    role TEXT NOT NULL CHECK (role IN ('context', 'action', 'outcome')),
    approved_at TIMESTAMPTZ,
    UNIQUE (organization_id, role, expression)
);

ALTER TABLE bdd_steps
    ADD COLUMN definition_id UUID REFERENCES step_definitions(id),
    ADD COLUMN arguments JSONB NOT NULL DEFAULT '[]'::jsonb;

WITH classified AS (
    SELECT s.id,
           p.organization_id,
           btrim(s.text) AS expression,
           r.status,
           r.approved_at,
           CASE COALESCE(
               CASE WHEN s.keyword IN ('given', 'when', 'then') THEN s.keyword END,
               (
                   SELECT prior.keyword
                   FROM bdd_steps prior
                   WHERE prior.revision_id = s.revision_id
                     AND prior.position < s.position
                     AND prior.keyword IN ('given', 'when', 'then')
                   ORDER BY prior.position DESC
                   LIMIT 1
               )
           )
               WHEN 'given' THEN 'context'
               WHEN 'when' THEN 'action'
               ELSE 'outcome'
           END AS role
    FROM bdd_steps s
    JOIN revisions r ON r.id = s.revision_id
    JOIN pages p ON p.id = r.page_id
), definitions AS (
    INSERT INTO step_definitions(organization_id, expression, role, approved_at)
    SELECT organization_id,
           expression,
           role,
           max(approved_at) FILTER (WHERE status = 'approved')
    FROM classified
    GROUP BY organization_id, expression, role
    RETURNING id, organization_id, expression, role
)
UPDATE bdd_steps s
SET definition_id = d.id
FROM classified c
JOIN definitions d
  ON d.organization_id = c.organization_id
 AND d.expression = c.expression
 AND d.role = c.role
WHERE c.id = s.id;

ALTER TABLE bdd_steps ALTER COLUMN definition_id SET NOT NULL;
