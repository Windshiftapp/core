package llm

import "encoding/json"

// Predefined JSON Schemas for AI features.
// These are used for structured output constraints.

// SchemaPlanMyDay is the JSON Schema for the PlanMyDay response.
var SchemaPlanMyDay = json.RawMessage(`{
	"type": "object",
	"properties": {
		"activities": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"time": {
						"type": "string"
					},
					"duration_minutes": {
						"type": "integer"
					},
					"item_key": {
						"type": "string"
					},
					"title": {
						"type": "string"
					},
					"reason": {
						"type": "string"
					}
				},
				"required": ["time", "duration_minutes", "item_key", "title", "reason"],
				"additionalProperties": false
			}
		},
		"summary": {
			"type": "string"
		}
	},
	"required": ["activities", "summary"],
	"additionalProperties": false
}`)

// SchemaFindSimilar is the JSON Schema for the FindSimilarItems response.
var SchemaFindSimilar = json.RawMessage(`{
	"type": "object",
	"properties": {
		"similar_items": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"item_key": {
						"type": "string"
					},
					"similarity": {
						"type": "string",
						"enum": ["duplicate", "closely_related", "somewhat_related"]
					},
					"reason": {
						"type": "string"
					}
				},
				"required": ["item_key", "similarity", "reason"],
				"additionalProperties": false
			}
		},
		"summary": {
			"type": "string"
		}
	},
	"required": ["similar_items", "summary"],
	"additionalProperties": false
}`)

// SchemaDecompose is the JSON Schema for the DecomposeItem response.
var SchemaDecompose = json.RawMessage(`{
	"type": "object",
	"properties": {
		"sub_tasks": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"title": {
						"type": "string"
					},
					"description": {
						"type": "string"
					}
				},
				"required": ["title", "description"],
				"additionalProperties": false
			}
		},
		"reasoning": {
			"type": "string"
		}
	},
	"required": ["sub_tasks", "reasoning"],
	"additionalProperties": false
}`)
