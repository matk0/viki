UPDATE assistant_draft_proposals AS proposal
SET changeset = jsonb_set(
    proposal.changeset,
    '{operations}',
    (
        SELECT jsonb_agg(
            jsonb_set(operation.value, '{content}', (operation.value->'content') - 'aliases')
            ORDER BY operation.position
        )
        FROM jsonb_array_elements(proposal.changeset->'operations') WITH ORDINALITY AS operation(value, position)
    )
)
WHERE jsonb_path_exists(proposal.changeset, '$.operations[*].content.aliases');

UPDATE revisions
SET search_document = to_tsvector('simple', title || ' ' || body_md);

ALTER TABLE revisions DROP COLUMN aliases;
