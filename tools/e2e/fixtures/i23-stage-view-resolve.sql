-- i23-stage-view-resolve.sql — уборка живой фикстуры витрины поселения И2.3.
\set ON_ERROR_STOP on
BEGIN;

DELETE FROM settlement_branch_buffers
 WHERE branch_id = 'e0000000-0000-4000-8000-0000000000a2';
DELETE FROM settlement_branches
 WHERE id = 'e0000000-0000-4000-8000-0000000000a2';
DELETE FROM settlements
 WHERE id = 'e0000000-0000-4000-8000-0000000000a1';

COMMIT;
