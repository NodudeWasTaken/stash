-- mirrors sqlite 78 + 85 in one step: career dates stored as date + precision.
-- career_length is converted and dropped by the Go post-migration (post15).
ALTER TABLE "performers" ADD COLUMN "career_start" date;
ALTER TABLE "performers" ADD COLUMN "career_start_precision" SMALLINT;
ALTER TABLE "performers" ADD COLUMN "career_end" date;
ALTER TABLE "performers" ADD COLUMN "career_end_precision" SMALLINT;
