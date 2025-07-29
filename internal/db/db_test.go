package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateTablesIfNotExist(t *testing.T) {
	if DB == nil {
		t.Skip("Skipping because DB is not initialized")
	}
	// This test just checks that the function is called and returns an error if DB is not initialized
	DB = nil
	err := CreateTablesIfNotExist()
	assert.Error(t, err)
}

func TestCloseDB_NoPanic(t *testing.T) {
	DB = nil
	assert.NotPanics(t, func() { CloseDB() })
}
