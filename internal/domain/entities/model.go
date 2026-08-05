package entities

// Filter represents a user-defined semantic filter.
type Filter string

// Message represents a notification message.
type Message struct {
	MatchedFilter string
	Confidence    float32
}
