ALTER TABLE pages
    DROP CONSTRAINT pages_check,
    DROP CONSTRAINT pages_kind_check;

UPDATE pages SET kind = 'feature' WHERE kind = 'scenario';
UPDATE pages SET kind = 'scenario' WHERE kind = 'subscenario';

ALTER TABLE pages
    ADD CONSTRAINT pages_kind_check CHECK (kind IN ('primitive', 'feature', 'scenario')),
    ADD CONSTRAINT pages_hierarchy_check CHECK (
        (kind = 'primitive' AND primitive_kind IS NOT NULL AND parent_id IS NULL)
        OR (kind = 'feature' AND primitive_kind IS NULL AND parent_id IS NULL)
        OR (kind = 'scenario' AND primitive_kind IS NULL AND parent_id IS NOT NULL)
    );

UPDATE assistant_draft_proposals AS proposal
SET changeset = jsonb_set(
    proposal.changeset,
    '{operations}',
    (
        SELECT jsonb_agg(
            jsonb_set(
                operation.value,
                '{kind}',
                to_jsonb(
                    CASE operation.value->>'kind'
                        WHEN 'scenario' THEN 'feature'
                        WHEN 'subscenario' THEN 'scenario'
                        ELSE operation.value->>'kind'
                    END
                )
            )
            ORDER BY operation.position
        )
        FROM jsonb_array_elements(proposal.changeset->'operations') WITH ORDINALITY AS operation(value, position)
    )
)
WHERE jsonb_path_exists(proposal.changeset, '$.operations[*] ? (@.kind == "scenario" || @.kind == "subscenario")');
