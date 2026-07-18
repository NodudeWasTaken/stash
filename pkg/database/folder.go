package database

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

// FolderStore tracks upstream's repository contract so interface drift is
// caught in one place.
type FolderStore interface {
	models.FolderReaderWriter

	FindByIDs(ctx context.Context, ids []models.FolderID) ([]*models.Folder, error)
}
