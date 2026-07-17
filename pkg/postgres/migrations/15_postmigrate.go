package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/postgres"
)

type schema15Migrator struct {
	migrator
}

// post15 mirrors the sqlite post78 migration: convert the free-text
// career_length column into career_start/career_end dates (year precision),
// preserving unparseable values as a custom field, then drop the column.
func post15(ctx context.Context, db *sqlx.DB) error {
	logger.Info("Running post-migration for schema version 15")

	m := schema15Migrator{
		migrator: migrator{
			db: db,
		},
	}

	if err := m.migrateCareerLength(ctx); err != nil {
		return fmt.Errorf("migrating career_length: %w", err)
	}

	if err := m.dropCareerLength(); err != nil {
		return fmt.Errorf("dropping career_length column: %w", err)
	}

	return nil
}

func (m *schema15Migrator) migrateCareerLength(ctx context.Context) error {
	logger.Info("Migrating career_length to career_start/career_end")

	const limit = 1000

	lastID := 0
	parsed := 0
	unparseable := 0

	for {
		gotSome := false

		if err := m.withTxn(ctx, func(tx *sqlx.Tx) error {
			query := `SELECT id, career_length FROM performers
				WHERE career_length IS NOT NULL AND career_length != ''`

			if lastID != 0 {
				query += fmt.Sprintf(" AND id > %d", lastID)
			}

			query += fmt.Sprintf(" ORDER BY id LIMIT %d", limit)

			rows, err := tx.Query(query)
			if err != nil {
				return err
			}
			defer rows.Close()

			type careerUpdate struct {
				id     int
				value  string
				start  *models.Date
				end    *models.Date
				failed bool
			}
			var updates []careerUpdate

			for rows.Next() {
				var (
					id           int
					careerLength string
				)

				if err := rows.Scan(&id, &careerLength); err != nil {
					return err
				}

				lastID = id
				gotSome = true

				start, end, err := models.ParseYearRangeString(careerLength)
				if err != nil {
					logger.Warnf("Could not parse career_length %q for performer %d: %v — preserving as custom field", careerLength, id, err)
					updates = append(updates, careerUpdate{id: id, value: careerLength, failed: true})
					continue
				}

				updates = append(updates, careerUpdate{id: id, start: start, end: end})
			}

			if err := rows.Err(); err != nil {
				return err
			}

			for _, u := range updates {
				if u.failed {
					if err := m.preserveAsCustomField(tx, u.id, u.value); err != nil {
						return fmt.Errorf("preserving career_length for performer %d: %w", u.id, err)
					}
					unparseable++
					continue
				}

				if err := m.updateCareerFields(tx, u.id, u.start, u.end); err != nil {
					return fmt.Errorf("updating career fields for performer %d: %w", u.id, err)
				}
				parsed++
			}

			return nil
		}); err != nil {
			return err
		}

		if !gotSome {
			break
		}
	}

	logger.Infof("Career length migration complete: %d parsed, %d unparseable (preserved as custom fields)", parsed, unparseable)
	return nil
}

func (m *schema15Migrator) updateCareerFields(tx *sqlx.Tx, id int, start *models.Date, end *models.Date) error {
	// year precision, matching the sqlite 85 conversion
	const yearPrecision = 2

	var (
		startDate, endDate           *string
		startPrecision, endPrecision *int
	)

	if start != nil {
		d := fmt.Sprintf("%04d-01-01", start.Year())
		p := yearPrecision
		startDate = &d
		startPrecision = &p
	}
	if end != nil {
		d := fmt.Sprintf("%04d-01-01", end.Year())
		p := yearPrecision
		endDate = &d
		endPrecision = &p
	}

	_, err := tx.Exec(
		`UPDATE performers SET
			career_start = $1::date, career_start_precision = $2,
			career_end = $3::date, career_end_precision = $4
		WHERE id = $5`,
		startDate, startPrecision, endDate, endPrecision, id,
	)
	return err
}

func (m *schema15Migrator) preserveAsCustomField(tx *sqlx.Tx, id int, value string) error {
	// check if a career_length custom field already exists
	var existing sql.NullString
	err := tx.Get(&existing, "SELECT value FROM performer_custom_fields WHERE performer_id = $1 AND field = 'career_length'", id)
	if err == nil {
		logger.Debugf("career_length custom field already exists for performer %d, skipping", id)
		return nil
	}

	_, err = tx.Exec(
		"INSERT INTO performer_custom_fields (performer_id, field, value, type) VALUES ($1, 'career_length', to_jsonb($2::text), 'string')",
		id, value,
	)
	return err
}

func (m *schema15Migrator) dropCareerLength() error {
	logger.Info("Dropping career_length column from performers table")
	return m.execAll([]string{
		"ALTER TABLE performers DROP COLUMN career_length",
	})
}

func init() {
	postgres.RegisterPostMigration(15, post15)
}
