-- mirrors sqlite 84: add a real basename column to folders.
-- sqlite needs Go pre/post migrations to repair legacy hierarchies before the
-- unique constraint; postgres databases are young enough to backfill directly.
-- strip everything up to the last path separator (handles / and \)
ALTER TABLE "folders" ADD COLUMN "basename" varchar(255);

UPDATE "folders"
SET "basename" = COALESCE(NULLIF(regexp_replace("path", '^.*[/\\]', ''), ''), "path");

ALTER TABLE "folders" ALTER COLUMN "basename" SET NOT NULL;

CREATE UNIQUE INDEX "index_folders_on_parent_folder_id_basename_unique" ON "folders" ("parent_folder_id", "basename");
CREATE INDEX "index_folders_on_basename" ON "folders" ("basename");
