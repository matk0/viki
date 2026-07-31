DELETE FROM audit_events
WHERE action IN ('auth.login', 'auth.logout');
