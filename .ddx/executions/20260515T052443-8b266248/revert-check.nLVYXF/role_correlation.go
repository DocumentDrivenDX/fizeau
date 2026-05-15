package fizeau

import (
	"fmt"
	"strings"
)

// Reserved cross-tool metadata keys (CONTRACT-003 § ExecuteRequest.Metadata).
// When the caller also sets one of these top-level fields and the same key
// in Metadata, the top-level field wins and a MetadataKeyCollision warning
// is emitted on the final event.
const (
	MetadataKeyRole          = "role"
	MetadataKeyCorrelationID = "correlation_id"
)

// MetadataWarningCodeKeyCollision is the ServiceFinalWarning.Code stamped
// onto the final event when caller-supplied Metadata duplicates a reserved
// top-level field (Role or CorrelationID).
const MetadataWarningCodeKeyCollision = "metadata_key_collision"

// Role normalization bounds (CONTRACT-003).
const roleMaxLen = 64

// CorrelationID normalization bounds (CONTRACT-003).
const correlationIDMaxLen = 256

// RoleNormalizationError is returned pre-dispatch when ServiceExecuteRequest.Role
// or RouteRequest.Role fails normalization (lowercased, alphanumeric+hyphen
// only, max 64 chars).
type RoleNormalizationError struct {
	// Role is the value that failed normalization.
	Role string
	// Reason is a short human-readable reason (e.g. "too long",
	// "contains invalid character").
	Reason string
}

func (e *RoleNormalizationError) Error() string {
	return fmt.Sprintf("invalid Role %q: %s", e.Role, e.Reason)
}

// CorrelationIDNormalizationError is returned pre-dispatch when
// ServiceExecuteRequest.CorrelationID or RouteRequest.CorrelationID fails
// normalization (printable ASCII, no whitespace except hyphen/colon/underscore,
// max 256 chars).
type CorrelationIDNormalizationError struct {
	CorrelationID string
	Reason        string
}

func (e *CorrelationIDNormalizationError) Error() string {
	return fmt.Sprintf("invalid CorrelationID %q: %s", e.CorrelationID, e.Reason)
}

// ValidateRole returns nil when role is empty (unset) or already
// normalized, and a typed *RoleNormalizationError otherwise. The empty
// string is treated as "unset". Callers should use the value as-is when
// validation succeeds — normalization here is identity-or-reject (we do
// not silently rewrite caller input).
func ValidateRole(role string) error {
	if role == "" {
		return nil
	}
	if len(role) > roleMaxLen {
		return &RoleNormalizationError{
			Role:   role,
			Reason: fmt.Sprintf("length %d exceeds max %d", len(role), roleMaxLen),
		}
	}
	for i := 0; i < len(role); i++ {
		c := role[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return &RoleNormalizationError{
				Role:   role,
				Reason: fmt.Sprintf("character %q at offset %d is not lowercase alphanumeric or hyphen", string(c), i),
			}
		}
	}
	return nil
}

// ValidateCorrelationID returns nil when id is empty or normalized, and a
// typed *CorrelationIDNormalizationError otherwise.
func ValidateCorrelationID(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > correlationIDMaxLen {
		return &CorrelationIDNormalizationError{
			CorrelationID: id,
			Reason:        fmt.Sprintf("length %d exceeds max %d", len(id), correlationIDMaxLen),
		}
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 0x21 && c <= 0x7E && c != ' ':
			// printable ASCII; whitespace already excluded by 0x21 lower bound.
			// allow hyphen/colon/underscore implicitly along with the rest of
			// the printable range. Only reject if outside the printable range.
		default:
			return &CorrelationIDNormalizationError{
				CorrelationID: id,
				Reason:        fmt.Sprintf("byte 0x%02x at offset %d is not printable ASCII (no control chars or whitespace)", c, i),
			}
		}
	}
	return nil
}

// metaWithRoleAndCorrelation overlays the caller-supplied top-level Role
// and CorrelationID onto a copy of meta under the reserved metadata keys,
// per CONTRACT-003. Top-level wins on collision; existing meta entries
// for unrelated keys are preserved. Returns meta unchanged when both
// fields are empty (avoids unnecessary allocation).
func metaWithRoleAndCorrelation(meta map[string]string, role, correlationID string) map[string]string {
	if role == "" && correlationID == "" {
		return meta
	}
	out := make(map[string]string, len(meta)+2)
	for k, v := range meta {
		out[k] = v
	}
	if role != "" {
		out[MetadataKeyRole] = role
	}
	if correlationID != "" {
		out[MetadataKeyCorrelationID] = correlationID
	}
	return out
}

// metadataReservedKeyCollisions returns the reserved metadata keys whose
// caller-supplied Metadata value would be overridden by a non-empty
// top-level Role or CorrelationID. Used to emit the MetadataKeyCollision
// warning on the final event.
func metadataReservedKeyCollisions(meta map[string]string, role, correlationID string) []string {
	var out []string
	if role != "" {
		if v, ok := meta[MetadataKeyRole]; ok && v != "" && v != role {
			out = append(out, MetadataKeyRole)
		}
	}
	if correlationID != "" {
		if v, ok := meta[MetadataKeyCorrelationID]; ok && v != "" && v != correlationID {
			out = append(out, MetadataKeyCorrelationID)
		}
	}
	return out
}

// metadataKeyCollisionMessage formats a human-readable message for the
// MetadataKeyCollision warning surfaced on the final event.
func metadataKeyCollisionMessage(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("top-level field(s) %s overrode caller Metadata entries with the same reserved key", strings.Join(keys, ", "))
}
