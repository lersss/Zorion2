-- i23-stage-view-resolve.sql — уборка живой фикстуры витрины поселения И2.3.
\set ON_ERROR_STOP on
BEGIN;

DELETE FROM settlement_storage_cells
 WHERE owner_type = 'settlement'
   AND owner_id = 'e0000000-0000-4000-8000-0000000000a1';
DELETE FROM settlement_branches
 WHERE id = 'e0000000-0000-4000-8000-0000000000a2';
DELETE FROM settlements
 WHERE id = 'e0000000-0000-4000-8000-0000000000a1';

COMMIT;
