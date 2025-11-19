package matcher

import (
	"encoding/json"
	"strings"
	"time"

	rootpb "subnet/proto/rootlayer"
)

// IntentMetadata contains parsed metadata from an intent
type IntentMetadata struct {
	Priority        string        // "critical", "high", "normal", "low"
	ResponseTimeout time.Duration // Override response timeout
	MaxRetries      int           // Override max retries
	RequireConfirm  bool          // Require explicit confirmation from agent
	Tags            []string      // Additional tags for routing
}

// ParseIntentMetadata extracts metadata from intent
func ParseIntentMetadata(intent *rootpb.Intent) *IntentMetadata {
	metadata := &IntentMetadata{
		Priority:       "normal",
		MaxRetries:     -1, // -1 means use default
		RequireConfirm: true,
	}

	// Parse from intent type prefix
	if strings.HasPrefix(intent.IntentType, "urgent:") {
		metadata.Priority = "high"
	} else if strings.HasPrefix(intent.IntentType, "critical:") {
		metadata.Priority = "critical"
	} else if strings.HasPrefix(intent.IntentType, "batch:") {
		metadata.Priority = "low"
	}

	// Check if deadline is very close
	if intent.Deadline > 0 {
		remaining := time.Until(time.Unix(intent.Deadline, 0))
		if remaining > 0 && remaining < 1*time.Minute {
			metadata.Priority = "critical"
		} else if remaining > 0 && remaining < 5*time.Minute {
			if metadata.Priority == "normal" {
				metadata.Priority = "high"
			}
		}
	}

	// Parse from intent params if they contain metadata
	if intent.Params != nil && intent.Params.IntentRaw != nil {
		// Try to parse as JSON to extract metadata
		var rawData map[string]interface{}
		if err := json.Unmarshal(intent.Params.IntentRaw, &rawData); err == nil {
			// Check for metadata field
			if metaField, ok := rawData["_metadata"]; ok {
				if metaMap, ok := metaField.(map[string]interface{}); ok {
					parseMetadataMap(metaMap, metadata)
				}
			}

			// Check for priority field
			if priority, ok := rawData["priority"].(string); ok {
				metadata.Priority = priority
			}

			// Check for timeout field
			if timeout, ok := rawData["timeout"].(float64); ok {
				metadata.ResponseTimeout = time.Duration(timeout) * time.Second
			}
		}
	}

	return metadata
}

// parseMetadataMap parses metadata from map
func parseMetadataMap(metaMap map[string]interface{}, metadata *IntentMetadata) {
	if priority, ok := metaMap["priority"].(string); ok {
		metadata.Priority = priority
	}

	if timeout, ok := metaMap["response_timeout"].(float64); ok {
		metadata.ResponseTimeout = time.Duration(timeout) * time.Second
	}

	if retries, ok := metaMap["max_retries"].(float64); ok {
		metadata.MaxRetries = int(retries)
	}

	if confirm, ok := metaMap["require_confirm"].(bool); ok {
		metadata.RequireConfirm = confirm
	}

	if tags, ok := metaMap["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				metadata.Tags = append(metadata.Tags, tagStr)
			}
		}
	}
}

// GetEffectiveTimeout returns effective timeout considering all factors
func GetEffectiveTimeout(intent *rootpb.Intent, config *TimeoutConfig) TimeoutSettings {
	metadata := ParseIntentMetadata(intent)

	// Calculate base timeouts
	settings := config.CalculateTimeouts(
		intent.IntentType,
		intent.Deadline,
		metadata.Priority,
	)

	// Override with explicit metadata if provided
	if metadata.ResponseTimeout > 0 {
		settings.ResponseTimeout = metadata.ResponseTimeout
	}

	if metadata.MaxRetries >= 0 {
		settings.MaxRetries = metadata.MaxRetries
	}

	return settings
}