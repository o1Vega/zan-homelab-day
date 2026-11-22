-- The documents source was renamed from "paperless" to "documents".
-- Days frozen under the old name keep their rows under the new name,
-- so they show as before and are not pulled again; the documents kept
-- on a day (pins) follow. Nothing is deleted: a day that somehow has
-- both keeps its old rows as they were.
UPDATE facts SET source = 'documents'
  WHERE source = 'paperless'
    AND NOT EXISTS (SELECT 1 FROM facts d WHERE d.user_id = facts.user_id AND d.date = facts.date AND d.source = 'documents');
UPDATE OR IGNORE pins SET source = 'documents',
    ref = CASE WHEN ref LIKE 'paperless:%' THEN 'documents:' || substr(ref, 11) ELSE ref END
  WHERE source = 'paperless';
