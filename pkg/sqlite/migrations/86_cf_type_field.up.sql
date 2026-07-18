-- 86_cf_type_field.up.sql
-- add the fork's custom-field `type` column to all custom field tables,
-- including those introduced upstream in schemas 76-83
ALTER TABLE `performer_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `studio_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `tag_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `scene_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `gallery_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `group_custom_fields` ADD COLUMN `type` text;
ALTER TABLE `image_custom_fields` ADD COLUMN `type` text;
