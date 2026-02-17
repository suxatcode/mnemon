package search

import (
	"regexp"
	"strings"

	"github.com/Grivn/mnemon/internal/model"
)

// Intent represents the detected query intent.
type Intent string

const (
	IntentWhy     Intent = "WHY"
	IntentWhen    Intent = "WHEN"
	IntentEntity  Intent = "ENTITY"
	IntentGeneral Intent = "GENERAL"
)

// IntentWeights maps intent to edge type weights for traversal.
type IntentWeights map[model.EdgeType]float64

var intentWeightsMap = map[Intent]IntentWeights{
	IntentWhy: {
		model.EdgeCausal:   0.7,
		model.EdgeTemporal: 0.2,
		model.EdgeEntity:   0.05,
		model.EdgeSemantic: 0.05,
	},
	IntentWhen: {
		model.EdgeTemporal: 0.7,
		model.EdgeCausal:   0.1,
		model.EdgeEntity:   0.1,
		model.EdgeSemantic: 0.1,
	},
	IntentEntity: {
		model.EdgeEntity:   0.6,
		model.EdgeSemantic: 0.3,
		model.EdgeTemporal: 0.05,
		model.EdgeCausal:   0.05,
	},
	IntentGeneral: {
		model.EdgeTemporal: 0.25,
		model.EdgeSemantic: 0.25,
		model.EdgeCausal:   0.25,
		model.EdgeEntity:   0.25,
	},
}

var whyPatterns = regexp.MustCompile(
	`(?i)\b(why|reason|because|cause|motivation|rationale)\b|` +
		`(为什么|原因|理由)`)

var whenPatterns = regexp.MustCompile(
	`(?i)\b(when|time|date|before|after|during|timeline|history|sequence)\b|` +
		`(什么时候|何时|时间|之前|之后)`)

var entityPatterns = regexp.MustCompile(
	`(?i)\b(what is|who is|tell me about|describe|about)\b|` +
		`(是什么|谁是|关于|介绍)`)

// DetectIntent analyzes a query string and returns the detected intent.
func DetectIntent(query string) Intent {
	q := strings.ToLower(query)
	whyScore := len(whyPatterns.FindAllString(q, -1))
	whenScore := len(whenPatterns.FindAllString(q, -1))
	entityScore := len(entityPatterns.FindAllString(q, -1))

	if whyScore > whenScore && whyScore > entityScore && whyScore > 0 {
		return IntentWhy
	}
	if whenScore > whyScore && whenScore > entityScore && whenScore > 0 {
		return IntentWhen
	}
	if entityScore > 0 {
		return IntentEntity
	}
	return IntentGeneral
}

// GetWeights returns the edge type weights for the given intent.
func GetWeights(intent Intent) IntentWeights {
	w, ok := intentWeightsMap[intent]
	if !ok {
		return intentWeightsMap[IntentGeneral]
	}
	return w
}
