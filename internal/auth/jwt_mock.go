package auth

import (
	"fmt"
	"strings"
	"sync"
)

// tokenInfo represents stored information about an issued mock token.
// In a real implementation this state would not be required as JWT is self-contained.
type tokenInfo struct {
	PlayerID uint64
	IsAdmin  bool
}

var tokenStore sync.Map // map[string]tokenInfo

// GenerateMockJWT creates a deterministic token for the given user and remembers it in-memory.
// Token format: "token-<username>".
func GenerateMockJWT(user *User) string {
	token := fmt.Sprintf("token-%s", user.Username)
	tokenStore.Store(token, tokenInfo{PlayerID: user.ID, IsAdmin: user.IsAdmin})
	return token
}

// ValidateJWT checks token validity and returns associated user info.
// The contract mirrors the assignment specification.
func ValidateJWT(token string) (playerID uint64, isValid bool, isAdmin bool) {
	// First, check store.
	if v, ok := tokenStore.Load(token); ok {
		info := v.(tokenInfo)
		return info.PlayerID, true, info.IsAdmin
	}

	// Fallback: parse token in format "token-<username>" to derive player ID based on username hash.
	if strings.HasPrefix(token, "token-") {
		username := strings.TrimPrefix(token, "token-")
		// Simple hash: sum of bytes (not cryptographically secure, but deterministic and fast for mock).
		var sum uint64
		for i := 0; i < len(username); i++ {
			sum += uint64(username[i])
		}
		// Assume non-admin by default in fallback.
		return sum, true, false
	}

	return 0, false, false
}
