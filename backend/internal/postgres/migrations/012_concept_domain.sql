ALTER TABLE pages
    DROP CONSTRAINT pages_kind_check,
    DROP CONSTRAINT pages_hierarchy_check;

ALTER TABLE pages RENAME COLUMN primitive_kind TO concept_kind;
UPDATE pages SET kind = 'concept' WHERE kind = 'primitive';

ALTER TABLE pages
    ADD CONSTRAINT pages_kind_check CHECK (kind IN ('concept', 'feature', 'scenario')),
    ADD CONSTRAINT pages_hierarchy_check CHECK (
        (kind = 'concept' AND concept_kind IS NOT NULL AND parent_id IS NULL)
        OR (kind = 'feature' AND concept_kind IS NULL AND parent_id IS NULL)
        OR (kind = 'scenario' AND concept_kind IS NULL AND parent_id IS NOT NULL)
    );

UPDATE assistant_draft_proposals AS proposal
SET changeset = jsonb_set(
    proposal.changeset,
    '{operations}',
    (
        SELECT jsonb_agg(
            jsonb_set(
                CASE
                    WHEN operation.value ? 'primitiveKind'
                        THEN jsonb_set(operation.value - 'primitiveKind', '{conceptKind}', operation.value->'primitiveKind')
                    ELSE operation.value
                END,
                '{kind}',
                to_jsonb(
                    CASE operation.value->>'kind'
                        WHEN 'primitive' THEN 'concept'
                        ELSE operation.value->>'kind'
                    END
                )
            )
            ORDER BY operation.position
        )
        FROM jsonb_array_elements(proposal.changeset->'operations') WITH ORDINALITY AS operation(value, position)
    )
)
WHERE EXISTS (
    SELECT 1
    FROM jsonb_array_elements(proposal.changeset->'operations') AS operation(value)
    WHERE operation.value->>'kind' = 'primitive'
       OR operation.value ? 'primitiveKind'
);

UPDATE audit_events
SET metadata = jsonb_set(metadata, '{kind}', '"concept"')
WHERE metadata->>'kind' = 'primitive';
