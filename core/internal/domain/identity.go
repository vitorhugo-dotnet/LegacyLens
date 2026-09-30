package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

// SymbolID creates a deterministic identity scoped to a project and revision.
// Length prefixes keep field boundaries unambiguous even when values contain separators.
func SymbolID(projectID, revisionID ID, path, qualifiedName, descriptor string) ID {
	hash := sha256.New()
	for _, value := range []string{string(projectID), string(revisionID), path, qualifiedName, descriptor} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(value))
	}
	return ID("sym_" + hex.EncodeToString(hash.Sum(nil)))
}
