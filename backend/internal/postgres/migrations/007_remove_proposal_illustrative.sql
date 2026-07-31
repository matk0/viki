UPDATE assistant_draft_proposals AS proposal
SET changeset = jsonb_set(
    proposal.changeset,
    '{operations}',
    (
        SELECT jsonb_agg(
            jsonb_set(operation.value, '{content}', (operation.value->'content') - 'illustrative')
            ORDER BY operation.position
        )
        FROM jsonb_array_elements(proposal.changeset->'operations') WITH ORDINALITY AS operation(value, position)
    )
)
WHERE jsonb_path_exists(proposal.changeset, '$.operations[*].content.illustrative');
