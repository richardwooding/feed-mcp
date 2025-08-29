package model

// FeedResult represents the result of fetching a single feed
type FeedResult struct {
	Feed               *Feed  `json:"feed,omitempty"`
	ID                 string `json:"id"`
	PublicURL          string `json:"public_url"`
	Title              string `json:"title,omitempty"`
	FetchError         string `json:"fetch_error,omitempty"`
	CircuitBreakerOpen bool   `json:"circuit_breaker_open,omitempty"`
}
