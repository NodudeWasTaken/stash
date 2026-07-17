package migrations

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/sqlite"
)

type schema86Migrator struct {
	migrator
}

func post86(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 86")

	m := schema86Migrator{
		migrator: migrator{
			db: db,
		},
	}

	return m.migrate(ctx)
}

var schema86CustomFieldTables = map[string]string{
	"performer_custom_fields": "performer_id",
	"studio_custom_fields":    "studio_id",
	"tag_custom_fields":       "tag_id",
	"scene_custom_fields":     "scene_id",
	"gallery_custom_fields":   "gallery_id",
	"group_custom_fields":     "group_id",
	"image_custom_fields":     "image_id",
}

func (m *schema86Migrator) migrate(ctx context.Context) error {
	for table, fk := range schema86CustomFieldTables {
		if err := m.migrateTable(ctx, table, fk); err != nil {
			return fmt.Errorf("backfilling type in %s: %w", table, err)
		}
	}

	return nil
}

func (m *schema86Migrator) migrateTable(ctx context.Context, table string, fk string) error {
	return m.withTxn(ctx, func(tx *sqlx.Tx) error {
		query := fmt.Sprintf("SELECT `%s`, `field`, `value` FROM `%s`", fk, table)

		rows, err := tx.Queryx(query)
		if err != nil {
			return err
		}
		defer rows.Close()

		type rowUpdate struct {
			id     int
			field  string
			gotype string
		}
		var updates []rowUpdate

		for rows.Next() {
			var (
				id    int
				field string
				value interface{}
			)

			if err := rows.Scan(&id, &field, &value); err != nil {
				return err
			}

			gotype := reflect.TypeOf(value).String()
			logger.Debugf("setting type for %v to %v for %s %v", value, gotype, table, id)
			updates = append(updates, rowUpdate{id: id, field: field, gotype: gotype})
		}

		if err := rows.Err(); err != nil {
			return err
		}

		updateSQL := fmt.Sprintf("UPDATE `%s` SET `type` = ? WHERE `%s` = ? AND `field` = ?", table, fk)
		for _, u := range updates {
			r, err := tx.Exec(updateSQL, u.gotype, u.id, u.field)
			if err != nil {
				return fmt.Errorf("error setting type to %v for %s %v field %s: %w", u.gotype, table, u.id, u.field, err)
			}

			rowsAffected, err := r.RowsAffected()
			if err != nil {
				return err
			}

			if rowsAffected == 0 {
				return fmt.Errorf("no rows affected when updating type to %v for %s %v field %s", u.gotype, table, u.id, u.field)
			}
		}

		return nil
	})
}

func init() {
	sqlite.RegisterPostMigration(86, post86)
}
