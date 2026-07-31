ALTER TABLE assistant_draft_proposals
    ADD COLUMN rejection_reason TEXT;

UPDATE assistant_draft_proposals
SET rejection_reason = 'Dôvod nebol zaznamenaný pri staršom odmietnutí.'
WHERE status = 'discarded';

ALTER TABLE assistant_draft_proposals
    ADD CONSTRAINT assistant_draft_proposals_rejection_reason_check
    CHECK (status <> 'discarded' OR (rejection_reason IS NOT NULL AND length(btrim(rejection_reason)) > 0));
