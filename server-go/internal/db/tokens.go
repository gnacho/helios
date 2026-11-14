package db

import (
	"database/sql"
	"time"
)

// APITokenRow is one stored API token. Hash holds the SHA-256 of the secret,
// the secret itself is never persisted.
type APITokenRow struct {
	ID         string  `json:"id"`
	UserID     string  `json:"-"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	Hash       string  `json:"-"`
	Scopes     string  `json:"scopes"`
	CreatedAt  int64   `json:"created_at"`
	LastUsedAt int64   `json:"last_used_at"`
	ExpiresAt  int64   `json:"expires_at"`
	RevokedAt  int64   `json:"revoked_at"`
	Username   *string `json:"username,omitempty"`
}

func (d *DB) CreateAPIToken(userID, name, prefix, hash, scopes string, expiresAt int64) (*APITokenRow, error) {
	id := cryptoRandomID()
	now := time.Now().UnixMilli()
	_, err := d.db.Exec(
		`INSERT INTO api_tokens (id, user_id, name, prefix, hash, scopes, created_at, last_used_at, expires_at, revoked_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, 0)`,
		id, userID, name, prefix, hash, scopes, now, expiresAt,
	)
	if err != nil {
		return nil, err
	}
	return d.GetAPIToken(id)
}

func (d *DB) GetAPIToken(id string) (*APITokenRow, error) {
	var t APITokenRow
	err := d.db.QueryRow(
		`SELECT id, user_id, name, prefix, hash, scopes, created_at, last_used_at, expires_at, revoked_at
		 FROM api_tokens WHERE id = ?`, id,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Hash, &t.Scopes, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *DB) GetAPITokenByPrefix(prefix string) (*APITokenRow, error) {
	var t APITokenRow
	err := d.db.QueryRow(
		`SELECT id, user_id, name, prefix, hash, scopes, created_at, last_used_at, expires_at, revoked_at
		 FROM api_tokens WHERE prefix = ?`, prefix,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Hash, &t.Scopes, &t.CreatedAt, &t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListAPITokens returns every token, with the owner username, newest first.
// Tokens are an account-wide feature, so the list is not filtered by user.
func (d *DB) ListAPITokens() ([]APITokenRow, error) {
	rows, err := d.db.Query(
		`SELECT t.id, t.user_id, t.name, t.prefix, t.hash, t.scopes, t.created_at, t.last_used_at,
		        t.expires_at, t.revoked_at, u.username
		 FROM api_tokens t LEFT JOIN users u ON u.id = t.user_id
		 ORDER BY t.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []APITokenRow{}
	for rows.Next() {
		var t APITokenRow
		var username sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Prefix, &t.Hash, &t.Scopes, &t.CreatedAt,
			&t.LastUsedAt, &t.ExpiresAt, &t.RevokedAt, &username); err != nil {
			return nil, err
		}
		if username.Valid {
			u := username.String
			t.Username = &u
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken marks a token as revoked. A revoked row is kept so the audit
// trail keeps the name and the creation date.
func (d *DB) RevokeAPIToken(id string) (bool, error) {
	res, err := d.db.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at = 0`,
		time.Now().UnixMilli(), id,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// TouchAPIToken records the last use. Only called when the previous value is
// old enough, so a polling client does not turn into a write per request.
func (d *DB) TouchAPIToken(id string, ts int64) error {
	_, err := d.db.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, ts, id)
	return err
}
