// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: clear_users.sql

package database

import (
	"context"
)

const clearUsers = `-- name: ClearUsers :exec
DELETE FROM users
`

func (q *Queries) ClearUsers(ctx context.Context) error {
	_, err := q.db.ExecContext(ctx, clearUsers)
	return err
}
