package emit

import (
	"strings"
	"sync"
)

// Default sensitive field patterns (case-insensitive)
var defaultSensitiveFields = []string{
	"password", "pwd", "pass", "secret", "key", "token", "auth",
	"credential", "cred", "private", "confidential", "sensitive",
	"api_key", "apikey", "access_token", "refresh_token", "jwt",
	"session", "cookie", "authorization", "bearer", "oauth",
	"client_secret", "private_key", "passphrase", "pin", "code",
}

// Default PII field patterns (case-insensitive)
var defaultPIIFields = []string{
	"email", "mail", "e_mail", "email_address", "emailaddress",
	"phone", "mobile", "telephone", "phone_number", "phonenumber", "tel",
	"ssn", "social_security", "social_security_number", "tax_id",
	"credit_card", "creditcard", "card_number", "cardnumber", "ccn",
	"passport", "passport_number", "license", "driver_license", "dl",
	"name", "first_name", "last_name", "full_name", "firstname", "lastname", "fullname",
	"address", "street", "city", "zip", "zipcode", "postal", "postal_code",
	"ip", "ip_address", "ipaddress", "user_agent", "useragent",
	"dob", "date_of_birth", "birthdate", "birthday", "birth_date",
	"iban", "account_number", "bank_account", "routing_number",
	"username", "user_name", "login", "userid",
}

// Optimized security implementation with caching and pre-compilation

// Field pattern cache for faster lookup
type fieldPatternCache struct {
	mu             sync.RWMutex
	piiCache       map[string]bool
	sensitiveCache map[string]bool
}

var (
	// Global field cache for faster lookups
	fieldCache = &fieldPatternCache{
		piiCache:       make(map[string]bool, 100),
		sensitiveCache: make(map[string]bool, 100),
	}

	// Pre-built lookup maps for O(1) field checking
	piiFieldsMap       map[string]bool
	sensitiveFieldsMap map[string]bool
	onceInit           sync.Once
)

// initializeFieldMaps builds lookup maps for O(1) field pattern matching
func initializeFieldMaps() {
	onceInit.Do(func() {
		// Build PII fields map
		piiFieldsMap = make(map[string]bool, len(defaultPIIFields)*2)
		for _, pattern := range defaultPIIFields {
			piiFieldsMap[pattern] = true
			piiFieldsMap[strings.ToUpper(pattern)] = true // Add uppercase variant
		}

		// Build sensitive fields map
		sensitiveFieldsMap = make(map[string]bool, len(defaultSensitiveFields)*2)
		for _, pattern := range defaultSensitiveFields {
			sensitiveFieldsMap[pattern] = true
			sensitiveFieldsMap[strings.ToUpper(pattern)] = true // Add uppercase variant
		}
	})
}

// Fast PII field checking with caching
func (l *Logger) isPIIFieldFast(fieldName string) bool {
	if l.piiMode == SHOW_PII {
		return false
	}

	initializeFieldMaps()

	// Check cache first
	fieldCache.mu.RLock()
	if cached, exists := fieldCache.piiCache[fieldName]; exists {
		fieldCache.mu.RUnlock()
		return cached
	}
	fieldCache.mu.RUnlock()

	// Fast lookup in pre-built map
	lowerFieldName := strings.ToLower(fieldName)
	isPII := piiFieldsMap[lowerFieldName]

	if !isPII {
		// Fallback to substring search only if direct lookup fails
		// Check if field name contains the pattern as a word or suffix/prefix
		for pattern := range piiFieldsMap {
			if strings.Contains(lowerFieldName, pattern) {
				// Additional check to avoid false positives like "description" matching "ip"
				// Only match if the pattern is at word boundaries or is a significant portion
				if len(pattern) >= 3 || lowerFieldName == pattern ||
					strings.HasPrefix(lowerFieldName, pattern+"_") ||
					strings.HasSuffix(lowerFieldName, "_"+pattern) ||
					strings.Contains(lowerFieldName, "_"+pattern+"_") ||
					strings.HasPrefix(lowerFieldName, pattern) && len(pattern) >= len(lowerFieldName)/2 ||
					strings.HasSuffix(lowerFieldName, pattern) && len(pattern) >= len(lowerFieldName)/2 {
					isPII = true
					break
				}
			}
		}
	}

	// Cache the result
	fieldCache.mu.Lock()
	fieldCache.piiCache[fieldName] = isPII
	fieldCache.mu.Unlock()

	return isPII
}

// Fast sensitive field checking with caching
func (l *Logger) isSensitiveFieldFast(fieldName string) bool {
	if l.sensitiveMode == SHOW_SENSITIVE {
		return false
	}

	initializeFieldMaps()

	// Check cache first
	fieldCache.mu.RLock()
	if cached, exists := fieldCache.sensitiveCache[fieldName]; exists {
		fieldCache.mu.RUnlock()
		return cached
	}
	fieldCache.mu.RUnlock()

	// Fast lookup in pre-built map
	lowerFieldName := strings.ToLower(fieldName)
	isSensitive := sensitiveFieldsMap[lowerFieldName]

	if !isSensitive {
		// Fallback to substring search only if direct lookup fails
		for pattern := range sensitiveFieldsMap {
			if strings.Contains(lowerFieldName, pattern) {
				isSensitive = true
				break
			}
		}
	}

	// Cache the result
	fieldCache.mu.Lock()
	fieldCache.sensitiveCache[fieldName] = isSensitive
	fieldCache.mu.Unlock()

	return isSensitive
}

// Optimized field masking with pre-allocated map and minimal allocations
func (l *Logger) maskSensitiveFieldsFast(fields map[string]any) map[string]any {
	if (l.sensitiveMode == SHOW_SENSITIVE && l.piiMode == SHOW_PII) || len(fields) == 0 {
		return fields
	}

	// Pre-allocate with exact capacity to avoid map growth
	maskedFields := make(map[string]any, len(fields))

	for key, value := range fields {
		// Fast path: check PII first (more specific), then sensitive data
		if l.isPIIFieldFast(key) {
			maskedFields[key] = l.piiMaskString
		} else if l.isSensitiveFieldFast(key) {
			maskedFields[key] = l.maskString
		} else {
			// Handle nested maps recursively
			if nestedMap, ok := value.(map[string]any); ok {
				maskedFields[key] = l.maskSensitiveFieldsFast(nestedMap)
			} else {
				maskedFields[key] = value
			}
		}
	}

	return maskedFields
}

// ClearFieldCache clears the field pattern cache (for testing or dynamic field updates)
func ClearFieldCache() {
	fieldCache.mu.Lock()
	defer fieldCache.mu.Unlock()

	fieldCache.piiCache = make(map[string]bool, 100)
	fieldCache.sensitiveCache = make(map[string]bool, 100)
}
