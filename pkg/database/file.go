package database

import (
	"github.com/stashapp/stash/pkg/models"
)

// FileStore tracks upstream's repository contract so interface drift is
// caught in one place.
type FileStore interface {
	models.FileReaderWriter
}
