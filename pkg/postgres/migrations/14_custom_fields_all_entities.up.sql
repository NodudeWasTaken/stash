-- mirrors sqlite 76, 77, 79, 81, 82, 83 (custom field tables for all entities)
-- includes the fork's "type" column directly (sqlite adds it in 86)

CREATE TABLE studio_custom_fields (
  studio_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("studio_id", "field"),
  foreign key("studio_id") references "studios"("id") on delete CASCADE
);

CREATE INDEX "index_studio_custom_fields_field_value" ON "studio_custom_fields" ("field", "value");

CREATE TABLE tag_custom_fields (
  tag_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("tag_id", "field"),
  foreign key("tag_id") references "tags"("id") on delete CASCADE
);

CREATE INDEX "index_tag_custom_fields_field_value" ON "tag_custom_fields" ("field", "value");

CREATE TABLE scene_custom_fields (
  scene_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("scene_id", "field"),
  foreign key("scene_id") references "scenes"("id") on delete CASCADE
);

CREATE INDEX "index_scene_custom_fields_field_value" ON "scene_custom_fields" ("field", "value");

CREATE TABLE gallery_custom_fields (
  gallery_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("gallery_id", "field"),
  foreign key("gallery_id") references "galleries"("id") on delete CASCADE
);

CREATE INDEX "index_gallery_custom_fields_field_value" ON "gallery_custom_fields" ("field", "value");

CREATE TABLE group_custom_fields (
  group_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("group_id", "field"),
  foreign key("group_id") references "groups"("id") on delete CASCADE
);

CREATE INDEX "index_group_custom_fields_field_value" ON "group_custom_fields" ("field", "value");

CREATE TABLE image_custom_fields (
  image_id integer NOT NULL,
  field text NOT NULL,
  "value" jsonb NOT NULL,
  "type" text,
  PRIMARY KEY ("image_id", "field"),
  foreign key("image_id") references "images"("id") on delete CASCADE
);

CREATE INDEX "index_image_custom_fields_field_value" ON "image_custom_fields" ("field", "value");
