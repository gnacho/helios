package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
)

type ctxKey string

const userIDKey ctxKey = "userID"

func withUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func UserIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}

func randRead(b []byte) (int, error) {
	return rand.Read(b)
}

func jsonNewEncoder(w http.ResponseWriter) *json.Encoder {
	return json.NewEncoder(w)
}
